// Keyboard gate checks (U1, U2, U3) on a live session page, over CDP.
// node keys.mjs <cdp-port> <session-url>
const [port, url] = process.argv.slice(2);
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
const ev = async (expression) => (await send("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true })).result?.result?.value;
const key = async (k, code, vk, modifiers = 0, text) => {
  await send("Input.dispatchKeyEvent", { type: "keyDown", key: k, code, windowsVirtualKeyCode: vk, modifiers, text });
  await send("Input.dispatchKeyEvent", { type: "keyUp", key: k, code, windowsVirtualKeyCode: vk, modifiers });
  await sleep(60);
};
const type = async (s) => {
  for (const ch of s) await key(ch, ch === " " ? "Space" : `Key${ch.toUpperCase()}`, ch.toUpperCase().charCodeAt(0), 0, ch);
};
const results = [];
const check = (name, ok, detail = "") => results.push(`${ok ? "PASS" : "FAIL"}  ${name}${detail ? `: ${detail}` : ""}`);

await send("Page.enable");
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 800, deviceScaleFactor: 1, mobile: false });
await send("Page.navigate", { url });
for (let i = 0; i < 60 && !(await ev("!!document.querySelector('.xterm textarea')")); i++) await sleep(250);
await sleep(1500);
const rows = () => ev("document.querySelector('.xterm-rows').innerText.replace(/\\u00a0/g, ' ')");
const checksText = () => ev("(document.querySelector('.checklog summary')?.innerText ?? document.querySelector('.workbench .note')?.innerText ?? '')");

// U2: Ctrl+Enter inside the terminal runs the check and the shell gets nothing.
await ev("document.querySelector('.xterm textarea').focus()");
await key("u", "KeyU", 85, 2); // Ctrl+U: clear the line
await type("echo marker");
await sleep(400);
const before = await rows();
const checksBefore = await checksText();
await key("Enter", "Enter", 13, 2); // Ctrl+Enter
await sleep(500);
const afterKey = await rows();
for (let i = 0; i < 40 && (await checksText()) === checksBefore; i++) await sleep(250);
const checksAfter = await checksText();
const line = (t) => t.split("\n").filter((l) => l.includes("echo marker")).join("|");
check("U2 Ctrl+Enter in the terminal ran a check", checksAfter !== checksBefore, `"${checksBefore}" -> "${checksAfter}"`);
check("U2 the shell received nothing (no newline, no characters)", line(before) === line(afterKey) && !afterKey.includes("marker\nmarker"), `line "${line(afterKey)}"`);
check("U2 focus stayed in the terminal", await ev("!!document.activeElement?.closest('.xterm')"));

// U1: "gi" typed in the terminal stays in the terminal.
await key("u", "KeyU", 85, 2);
const hashBefore = await ev("location.hash");
await type("gi");
await sleep(400);
check("U1 'gi' in the terminal did not navigate", (await ev("location.hash")) === hashBefore, await ev("location.hash"));
check("U1 'gi' reached the shell", (await rows()).includes("# gi"), "");
await key("u", "KeyU", 85, 2);

// U3: Ctrl+K from the terminal opens the palette; Esc closes it and returns focus.
await key("k", "KeyK", 75, 2);
await sleep(300);
check("U3 Ctrl+K in the terminal opened the palette", await ev("!!document.querySelector('dialog.dialog-palette[open]')"));
check("U3 the shell did not receive Ctrl+K", !(await rows()).includes("^K"));
check("U3 focus is in the palette input", await ev("document.activeElement?.id === 'palette-input'"));
check("U3 combobox semantics", await ev("(() => { const i = document.getElementById('palette-input'); return i.getAttribute('role') === 'combobox' && !!i.getAttribute('aria-activedescendant') && !!document.getElementById(i.getAttribute('aria-activedescendant')); })()"));
await key("ArrowDown", "ArrowDown", 40);
const active2 = await ev("document.getElementById('palette-input').getAttribute('aria-activedescendant')");
check("U3 arrow keys move the active option", active2 === "palette-go-exercises", active2);
await key("Escape", "Escape", 27);
await sleep(300);
check("U3 Esc closed the palette", await ev("!document.querySelector('dialog.dialog-palette[open]')"));
check("U3 focus went back to the terminal", await ev("!!document.activeElement?.closest('.xterm')"));

// U1 off switch and the palette's Enter: open the palette with Ctrl+K from the page, search, Enter.
await ev("document.activeElement?.blur()");
await key("k", "KeyK", 75, 2);
await sleep(300);
await type("hist");
await key("Enter", "Enter", 13);
await sleep(800);
check("U3 typing and Enter in the palette navigate", (await ev("location.hash")) === "#/history", await ev("location.hash"));

// U1: "gi" in a History filter does nothing; outside fields it goes to the inbox.
await ev("document.querySelector('.filters select').focus()");
await type("gi");
await sleep(300);
check("U1 'gi' in a History filter did not navigate", (await ev("location.hash")) === "#/history", await ev("location.hash"));
await ev("document.activeElement?.blur()");
await type("gi");
await sleep(500);
check("U1 'gi' outside fields goes to the inbox", (await ev("location.hash")) === "#/", await ev("location.hash"));

// U1 off switch: '?' opens the shortcuts; turning single keys off stops 'g h'.
await type("?");
await sleep(300);
check("'?' opens the shortcut list", await ev("!!document.querySelector('dialog.dialog-help[open]')"));
await ev("document.querySelector('dialog.dialog-help input[type=checkbox]').click()");
await key("Escape", "Escape", 27);
await sleep(300);
await ev("document.activeElement?.blur()");
await type("gh");
await sleep(400);
check("U1 with single keys off, 'g h' does nothing", (await ev("location.hash")) === "#/", await ev("location.hash"));
const pref = await ev("localStorage.getItem('rootcause.prefs')");
check("U1 the off switch is remembered", (pref ?? "").includes('"shortcuts":false'), pref);
await ev("localStorage.setItem('rootcause.prefs', JSON.stringify({ introDone: true }))");

console.log(results.join("\n"));
await fetch(`http://127.0.0.1:${port}/json/close/${target.id}`);
ws.close();
