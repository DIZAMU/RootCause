// Screenshot driver for the RootCause dashboard. Runs with Node's built-ins
// only (fetch, WebSocket) against a headless Chrome started with
// --remote-debugging-port on 127.0.0.1. No npm packages.
//
// node shots.mjs <cdp-port> <out-dir> <plan.json>
// plan.json: [{name, url, width, height, scheme, motion, waitFor, wait, fullPage}]
import { writeFileSync, readFileSync } from "node:fs";
import { join } from "node:path";

const [port, outDir, planFile] = process.argv.slice(2);
const plan = JSON.parse(readFileSync(planFile, "utf8"));

const target = await (await fetch(`http://127.0.0.1:${port}/json/new?about:blank`, { method: "PUT" })).json();
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r, j) => { ws.onopen = r; ws.onerror = j; });
let id = 0;
const pending = new Map();
const consoleErrors = [];
ws.onmessage = (ev) => {
  const m = JSON.parse(ev.data);
  if (m.id && pending.has(m.id)) {
    const { resolve, reject } = pending.get(m.id);
    pending.delete(m.id);
    m.error ? reject(new Error(JSON.stringify(m.error))) : resolve(m.result);
  } else if (m.method === "Runtime.exceptionThrown") {
    consoleErrors.push(["exception", m.params.exceptionDetails.text, m.params.exceptionDetails.exception?.description]);
  } else if (m.method === "Runtime.consoleAPICalled" && (m.params.type === "error" || m.params.type === "warning")) {
    consoleErrors.push([m.params.type, m.params.args.map((a) => a.value ?? a.description).join(" ")]);
  } else if (m.method === "Log.entryAdded" && (m.params.entry.level === "error" || m.params.entry.level === "warning")) {
    consoleErrors.push(["log-" + m.params.entry.level, m.params.entry.text, m.params.entry.url]);
  }
};
const send = (method, params = {}) => new Promise((resolve, reject) => {
  const n = ++id;
  pending.set(n, { resolve, reject });
  ws.send(JSON.stringify({ id: n, method, params }));
});
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const evaluate = async (expression) => (await send("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true })).result.value;

await send("Page.enable");
await send("Network.enable");
await send("Runtime.enable");
await send("Log.enable");

const results = [];
for (const s of plan) {
  consoleErrors.length = 0;
  await send("Emulation.setDeviceMetricsOverride", { width: s.width, height: s.height, deviceScaleFactor: 1, mobile: s.width < 600 });
  await send("Emulation.setEmulatedMedia", { features: [
    { name: "prefers-color-scheme", value: s.scheme ?? "light" },
    { name: "prefers-reduced-motion", value: s.motion ?? "no-preference" },
  ] });
  if (s.theme && s.url !== "keep") {
    // The app's own theme switch is stored in localStorage; set it before load.
    await send("Page.navigate", { url: new URL("/", s.url).href });
    await sleep(500);
    await evaluate(`localStorage.setItem("rootcause.theme", ${JSON.stringify(s.theme)})`);
  }
  await send("Network.emulateNetworkConditions", { offline: false, latency: s.latency ?? 0, downloadThroughput: -1, uploadThroughput: -1 });
  // "keep" stays on the current page (the app may have navigated itself).
  if (s.url !== "keep") await send("Page.navigate", { url: s.url });
  const deadline = Date.now() + 15000;
  if (s.waitFor) {
    while (Date.now() < deadline && !(await evaluate(`!!document.querySelector(${JSON.stringify(s.waitFor)})`))) await sleep(200);
  }
  if (s.click) {
    await evaluate(`document.querySelector(${JSON.stringify(s.click)})?.click()`);
    await sleep(s.afterClick ?? 1000);
  }
  if (s.keys) {
    for (const k of s.keys) {
      await send("Input.dispatchKeyEvent", { type: "keyDown", key: k.key, code: k.code ?? k.key, modifiers: k.modifiers ?? 0, windowsVirtualKeyCode: k.vk ?? 0, text: k.text });
      await send("Input.dispatchKeyEvent", { type: "keyUp", key: k.key, code: k.code ?? k.key, modifiers: k.modifiers ?? 0, windowsVirtualKeyCode: k.vk ?? 0 });
      await sleep(80);
    }
  }
  await sleep(s.wait ?? 800);
  let clip;
  if (s.fullPage) {
    const h = await evaluate("Math.ceil(document.documentElement.scrollHeight)");
    clip = { x: 0, y: 0, width: s.width, height: Math.max(h, s.height), scale: 1 };
  }
  const shot = await send("Page.captureScreenshot", { format: "png", captureBeyondViewport: !!s.fullPage, ...(clip ? { clip } : {}) });
  const file = join(outDir, `${s.name}.png`);
  writeFileSync(file, Buffer.from(shot.data, "base64"));
  const overflow = await evaluate("document.documentElement.scrollWidth > window.innerWidth");
  results.push({ name: s.name, file, consoleErrors: [...consoleErrors], horizontalOverflow: overflow, title: await evaluate("document.title") });
}
writeFileSync(join(outDir, "results.json"), JSON.stringify(results, null, 2));
await fetch(`http://127.0.0.1:${port}/json/close/${target.id}`);
ws.close();
console.log(`${results.length} screenshots in ${outDir}`);
for (const r of results) if (r.consoleErrors.length || r.horizontalOverflow) console.log(r.name, JSON.stringify({ consoleErrors: r.consoleErrors, horizontalOverflow: r.horizontalOverflow }));
