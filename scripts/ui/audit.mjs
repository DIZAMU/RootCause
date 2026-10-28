// Accessibility (axe-core) and layout shift (CLS) per page, over CDP.
// node audit.mjs <cdp-port> <axe.min.js> <plan.json>
// plan.json: [{name, url, width, height, scheme, wait}]
import { readFileSync } from "node:fs";

const [port, axePath, planFile] = process.argv.slice(2);
const axe = readFileSync(axePath, "utf8");
const plan = JSON.parse(readFileSync(planFile, "utf8"));
const target = await (await fetch(`http://127.0.0.1:${port}/json/new?about:blank`, { method: "PUT" })).json();
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => (ws.onopen = r));
let id = 0;
const pending = new Map();
ws.onmessage = (e) => {
  const m = JSON.parse(e.data);
  if (m.id && pending.has(m.id)) {
    pending.get(m.id)(m);
    pending.delete(m.id);
  }
};
const send = (method, params = {}) => new Promise((r) => { const n = ++id; pending.set(n, r); ws.send(JSON.stringify({ id: n, method, params })); });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const ev = async (expression) => {
  const m = await send("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true });
  if (m.result?.exceptionDetails) throw new Error(JSON.stringify(m.result.exceptionDetails).slice(0, 400));
  return m.result?.result?.value;
};

await send("Page.enable");
// Sum every layout shift from the first paint on, as CLS does.
await send("Page.addScriptToEvaluateOnNewDocument", {
  source: `window.__cls = 0; window.__shifts = []; new PerformanceObserver((l) => { for (const e of l.getEntries()) if (!e.hadRecentInput) { window.__cls += e.value; window.__shifts.push(e.value.toFixed(4) + " " + (e.sources || []).map((s) => s.node ? (s.node.className || s.node.nodeName) : "?").join(", ")); } }).observe({ type: "layout-shift", buffered: true });`,
});
let failures = 0;
for (const s of plan) {
  await send("Emulation.setDeviceMetricsOverride", { width: s.width, height: s.height, deviceScaleFactor: 1, mobile: s.width < 600 });
  await send("Emulation.setEmulatedMedia", { features: [{ name: "prefers-color-scheme", value: s.scheme ?? "light" }, { name: "prefers-reduced-motion", value: s.motion ?? "no-preference" }] });
  // A fresh document for every page: a hash change alone keeps the old one,
  // and its layout shift count with it.
  await send("Page.navigate", { url: "about:blank" });
  await sleep(200);
  await send("Page.navigate", { url: s.url });
  await sleep(s.wait ?? 4000);
  await ev(axe);
  const res = await ev(`axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"] } }).then(r => r.violations.map(v => ({ id: v.id, impact: v.impact, nodes: v.nodes.length, help: v.help, target: v.nodes[0]?.target?.join(" ") })))`);
  const cls = await ev("window.__cls");
  const bad = res.length > 0 || cls > 0;
  failures += bad ? 1 : 0;
  console.log(`${bad ? "FAIL" : "PASS"}  ${s.name}: axe violations ${res.length}, CLS ${cls.toFixed(4)}`);
  for (const v of res) console.log(`      ${v.impact} ${v.id} (${v.nodes}): ${v.help} @ ${v.target}`);
  if (cls > 0) for (const sh of await ev("window.__shifts")) console.log(`      shift ${sh}`);
}
await fetch(`http://127.0.0.1:${port}/json/close/${target.id}`);
ws.close();
console.log(failures ? `${failures} page(s) failed` : "every page passed");
