// Walks the Tab order of a page and lists each stop, with whether a focus
// outline is visible. node tabwalk.mjs <port> <url> <stops>
const [port, url, stopsArg] = process.argv.slice(2);
const t = await (await fetch(`http://127.0.0.1:${port}/json/new?about:blank`, { method: "PUT" })).json();
const ws = new WebSocket(t.webSocketDebuggerUrl);
await new Promise((r) => (ws.onopen = r));
let id = 0; const p = new Map();
ws.onmessage = (e) => { const m = JSON.parse(e.data); if (m.id && p.has(m.id)) { p.get(m.id)(m); p.delete(m.id); } };
const send = (method, params = {}) => new Promise((r) => { const n = ++id; p.set(n, r); ws.send(JSON.stringify({ id: n, method, params })); });
const ev = async (expression) => (await send("Runtime.evaluate", { expression, returnByValue: true })).result?.result?.value;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
await send("Page.enable");
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 800, deviceScaleFactor: 1, mobile: false });
await send("Page.navigate", { url });
await sleep(4000);
const stops = [];
for (let i = 0; i < Number(stopsArg); i++) {
  // In the terminal, Esc first: that is the way out of it.
  if (await ev("!!document.activeElement?.closest('.xterm')")) {
    await send("Input.dispatchKeyEvent", { type: "keyDown", key: "Escape", code: "Escape", windowsVirtualKeyCode: 27 });
    await send("Input.dispatchKeyEvent", { type: "keyUp", key: "Escape", code: "Escape", windowsVirtualKeyCode: 27 });
  }
  await send("Input.dispatchKeyEvent", { type: "keyDown", key: "Tab", code: "Tab", windowsVirtualKeyCode: 9 });
  await send("Input.dispatchKeyEvent", { type: "keyUp", key: "Tab", code: "Tab", windowsVirtualKeyCode: 9 });
  await sleep(120);
  stops.push(await ev(`(() => { const a = document.activeElement; if (!a) return "none"; const s = getComputedStyle(a); const outline = s.outlineStyle !== "none" && parseFloat(s.outlineWidth) > 0; const name = (a.getAttribute("aria-label") || a.innerText || a.value || a.className || a.tagName).trim().replace(/\\s+/g, " ").slice(0, 40); return (outline || a.closest(".xterm") ? "ok " : "NO-OUTLINE ") + a.tagName.toLowerCase() + " " + name; })()`));
}
console.log(stops.map((s, i) => `${String(i + 1).padStart(2)} ${s}`).join("\n"));
await fetch(`http://127.0.0.1:${port}/json/close/${t.id}`); ws.close();
