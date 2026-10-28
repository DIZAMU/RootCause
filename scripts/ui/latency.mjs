// Terminal typing latency and terminal stability, over CDP.
// node latency.mjs <cdp-port> <session-url> <n> [stability]
// Types n characters into the web terminal, one at a time, and times each
// from the key event to its echo in the terminal's rows. With "stability",
// it also marks the xterm element, reveals a hint and waits through polls,
// then checks the same element is still mounted.
const [port, url, nArg, mode] = process.argv.slice(2);
const n = Number(nArg || 50);
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
const send = (method, params = {}) => new Promise((r) => { const k = ++id; pending.set(k, r); ws.send(JSON.stringify({ id: k, method, params })); });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const ev = async (expression) => (await send("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true })).result?.result?.value;

await send("Page.enable");
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 800, deviceScaleFactor: 1, mobile: false });
await send("Page.navigate", { url });
for (let i = 0; i < 80 && !(await ev("!!document.querySelector('.xterm textarea')")); i++) await sleep(250);
await sleep(2000);
await ev("document.querySelector('.xterm textarea').focus()");
await send("Input.dispatchKeyEvent", { type: "keyDown", key: "u", code: "KeyU", windowsVirtualKeyCode: 85, modifiers: 2 });
await send("Input.dispatchKeyEvent", { type: "keyUp", key: "u", code: "KeyU", windowsVirtualKeyCode: 85, modifiers: 2 });
await sleep(500);

// The page measures in its own clock: from the key's dispatch to the first
// frame in which the rows show one more character.
await ev(`window.__lat = []; window.__count = () => (document.querySelector('.xterm-rows').innerText.match(/x/g) || []).length;`);
const times = [];
for (let i = 0; i < n; i++) {
  await ev("window.__t0 = performance.now(); window.__c0 = window.__count();");
  await send("Input.dispatchKeyEvent", { type: "keyDown", key: "x", code: "KeyX", windowsVirtualKeyCode: 88, text: "x" });
  await send("Input.dispatchKeyEvent", { type: "keyUp", key: "x", code: "KeyX", windowsVirtualKeyCode: 88 });
  const ms = await ev(`new Promise((done) => { const tick = () => { if (window.__count() > window.__c0) done(performance.now() - window.__t0); else if (performance.now() - window.__t0 > 2000) done(-1); else requestAnimationFrame(tick); }; tick(); })`);
  times.push(ms);
  await sleep(40);
}
await send("Input.dispatchKeyEvent", { type: "keyDown", key: "u", code: "KeyU", windowsVirtualKeyCode: 85, modifiers: 2 });
await send("Input.dispatchKeyEvent", { type: "keyUp", key: "u", code: "KeyU", windowsVirtualKeyCode: 85, modifiers: 2 });
const ok = times.filter((t) => t >= 0).sort((a, b) => a - b);
const q = (p) => ok[Math.min(ok.length - 1, Math.floor(p * ok.length))];
console.log(`latency: ${ok.length}/${n} echoed, median ${q(0.5).toFixed(1)} ms, p90 ${q(0.9).toFixed(1)} ms, max ${ok[ok.length - 1].toFixed(1)} ms`);

if (mode === "stability") {
  await ev("document.querySelector('.xterm').dataset.mark = 'kept'");
  const hint = await ev(`(() => { const b = [...document.querySelectorAll('.checkbar button')].find(b => b.querySelector('.btn-note')); if (b) { b.click(); return true; } return false; })()`);
  await sleep(6000); // a hint reveal, then three session polls
  const kept = await ev("document.querySelector('.xterm')?.dataset.mark === 'kept'");
  const focused = await ev("!!document.activeElement?.closest('.xterm') || document.activeElement?.tagName");
  console.log(`stability: hint revealed ${hint}, same terminal element after 6 s of polls and the reveal: ${kept}`);
  console.log(`stability: focus after the reveal: ${focused}`);
}
await fetch(`http://127.0.0.1:${port}/json/close/${target.id}`);
ws.close();
