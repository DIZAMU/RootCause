// App shell checks over CDP: the first-run intro, the collapsed sidebar rail,
// the layout on wide screens, and the "Reset all data" dialog.
// node shell.mjs <cdp-port> <base-url> <axe.min.js> [wipe]
// Run it first in a fresh browser profile: the intro check needs a browser
// with no stored preferences. It leaves the intro finished. With "wipe" it
// only resets all data through the dialog, for real, and checks the fresh
// state after.
import { readFileSync } from "node:fs";

const [port, base, axePath, wipe] = process.argv.slice(2);
const axe = readFileSync(axePath, "utf8");
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
const key = async (k, code, vk, modifiers = 0, text) => {
  await send("Input.dispatchKeyEvent", { type: "keyDown", key: k, code, windowsVirtualKeyCode: vk, modifiers, text });
  await send("Input.dispatchKeyEvent", { type: "keyUp", key: k, code, windowsVirtualKeyCode: vk, modifiers });
  await sleep(60);
};
const esc = () => key("Escape", "Escape", 27);
const enter = () => key("Enter", "Enter", 13, 0, "\r");
const tab = () => key("Tab", "Tab", 9);
const type = async (s) => {
  for (const ch of s) await key(ch, `Key${ch.toUpperCase()}`, ch.toUpperCase().charCodeAt(0), 0, ch);
};
const results = [];
const check = (name, ok, detail = "") => results.push(`${ok ? "PASS" : "FAIL"}  ${name}${detail !== "" ? `: ${detail}` : ""}`);
const size = (width, height, scale = 1, motion = "no-preference") =>
  Promise.all([
    send("Emulation.setDeviceMetricsOverride", { width, height, deviceScaleFactor: scale, mobile: false }),
    send("Emulation.setEmulatedMedia", { features: [{ name: "prefers-reduced-motion", value: motion }] }),
  ]);
const load = async (hash) => {
  await send("Page.navigate", { url: "about:blank" });
  await sleep(150);
  await send("Page.navigate", { url: `${base}/${hash}` });
  for (let i = 0; i < 60 && !(await ev("!!document.querySelector('main .page-head')")); i++) await sleep(200);
  await sleep(1200);
};
const axeRun = async () => {
  if (!(await ev("typeof axe !== 'undefined'"))) await ev(axe);
  return ev(`axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"] } }).then(r => r.violations.map(v => v.id + " @ " + (v.nodes[0]?.target?.join(" ") ?? "")))`);
};
const introOpen = () => ev("!!document.querySelector('dialog.dialog-intro[open]')");
const step = () => ev("document.querySelector('dialog.dialog-intro[open] .intro-text .label')?.innerText ?? ''");

await send("Page.enable");
await size(1280, 800);

let word = "";
try {
if (wipe !== "wipe") {
// --- the intro: a browser with no stored preferences, on a database with
// history (the fixture has an active and a finished session).
await load("#/");
check("intro opens on the first visit", await introOpen());
check("intro: focus starts on Next", await ev("document.activeElement?.classList.contains('intro-next')"), await ev("document.activeElement?.className"));
const introAxe = await axeRun();
check("intro: axe violations with the intro open", introAxe.length === 0, introAxe.join("; ") || "0");
const s1 = await step();
await enter();
await sleep(400);
const s2 = await step();
check("intro: Enter on Next moves to step 2", s1 !== s2 && s2.includes("2"), `${s1} -> ${s2}`);
check("intro: focus stays on Next after a step", await ev("document.activeElement?.classList.contains('intro-next')"));
await ev("document.querySelector('dialog.dialog-intro .intro-back').click()");
await sleep(400);
check("intro: Back returns to step 1, focus on Next", (await step()) === s1 && (await ev("document.activeElement?.classList.contains('intro-next')")), await step());
let inside = true;
for (let i = 0; i < 6; i++) {
  await tab();
  // Past the last button Chrome may hand focus to its own UI (body here),
  // never to the page behind the dialog, which is inert.
  inside &&= await ev("!!document.activeElement?.closest('dialog.dialog-intro') || document.activeElement === document.body || !document.activeElement");
}
check("intro: Tab never reaches the page behind", inside);
for (let i = 0; i < 3; i++) await ev("document.querySelector('dialog.dialog-intro .intro-next')?.click()");
await sleep(400);
check("intro: the last step offers Get started", await ev("!!document.querySelector('dialog.dialog-intro form .intro-next')"));
await esc();
await sleep(500);
check("intro: Esc skips it", !(await introOpen()));
check("intro: finishing is remembered", ((await ev("localStorage.getItem('rootcause.prefs')")) ?? "").includes('"introDone":true'));
await load("#/");
check("intro: does not open again after a reload", !(await introOpen()));
await ev("document.querySelector('.sidebar-help').focus()");
await enter();
await sleep(400);
check("help: the ? button opens the help dialog", await ev("!!document.querySelector('dialog.dialog-help[open]')"));
const helpAxe = await axeRun();
check("help: axe violations with help open", helpAxe.length === 0, helpAxe.join("; ") || "0");
await ev("document.querySelector('dialog.dialog-help .help-intro').focus()");
await enter();
await sleep(500);
check("help: Show the intro again opens it at step 1", (await introOpen()) && (await step()) === s1, await step());
await esc();
await sleep(500);
check("help: after the intro, focus is back on the ? button", await ev("document.activeElement?.classList.contains('sidebar-help')"), await ev("document.activeElement?.className"));

// --- the sidebar. Indicator: in the gutter at the sidebar's left edge, with
// clear space before the item's icon.
const railFacts = `(() => {
  const side = document.querySelector('.sidebar').getBoundingClientRect();
  const sb = document.querySelector('.sidebar');
  const cur = document.querySelector('.nav-item.is-current');
  const bar = getComputedStyle(cur, '::before');
  const item = cur.getBoundingClientRect();
  const barLeft = item.left + parseFloat(bar.left);
  const barRight = barLeft + parseFloat(bar.width);
  const icon = cur.querySelector('.icon').getBoundingClientRect();
  const levels = document.querySelector('.sidebar-levels');
  const ringsShown = [...document.querySelectorAll('.sidebar .ring')].filter((r) => r.getBoundingClientRect().width > 0).length;
  const foot = [...document.querySelectorAll('.sidebar-foot > *')].map((el) => { const r = el.getBoundingClientRect(); return r.left >= side.left - 0.5 && r.right <= side.right + 0.5 && r.width > 0; });
  const wide = [...sb.querySelectorAll('*')].filter((el) => { const r = el.getBoundingClientRect(); return r.width > 0 && r.right > side.right + 0.5; }).map((el) => el.tagName.toLowerCase() + '.' + String(el.className.baseVal ?? el.className).split(' ')[0] + ':' + Math.round(el.getBoundingClientRect().right - side.right)).slice(0, 6);
  return { width: Math.round(side.width), overflowX: sb.scrollWidth > sb.clientWidth, wide, overflowY: sb.scrollHeight > sb.clientHeight,
    barInside: barLeft >= side.left - 0.5 && barRight <= side.left + 12, gap: Math.round(icon.left - barRight), ringsShown,
    levelsShown: !!levels && getComputedStyle(levels).display !== 'none', footInside: foot.every(Boolean), foot: foot.length,
    transition: getComputedStyle(document.querySelector('.app')).transitionProperty, duration: getComputedStyle(document.querySelector('.app')).transitionDuration };
})()`;
await load("#/");
const open = await ev(railFacts);
check("sidebar: no transition on the first paint", !open.transition.includes("grid-template-columns"), open.transition);
check("sidebar open: indicator at the left edge with a gap before the icon", open.barInside && open.gap >= 8, `gap ${open.gap}px`);
await ev("document.querySelector('.sidebar-toggle').click()");
await sleep(600);
const rail = await ev(railFacts);
check("rail: 64 px wide", rail.width === 64, `${rail.width}px`);
check("rail: no overflow, no scrollbars", !rail.overflowX && !rail.overflowY, JSON.stringify({ x: rail.overflowX, y: rail.overflowY, past: rail.wide }));
check("rail: no level rings", !rail.levelsShown && rail.ringsShown === 0, `${rail.ringsShown} shown`);
check("rail: language, theme and help fit inside", rail.footInside && rail.foot === 3, `${rail.foot} controls`);
check("rail: indicator at the left edge with a gap before the icon", rail.barInside && rail.gap >= 8, `gap ${rail.gap}px`);
check("rail: the width animates after a toggle", rail.transition.includes("grid-template-columns") && rail.duration !== "0s", `${rail.transition} ${rail.duration}`);
const names = await ev("[...document.querySelectorAll('.sidebar a, .sidebar button')].filter((e) => e.getBoundingClientRect().width > 0).map((e) => (e.getAttribute('aria-label') || e.innerText).trim() && e.title ? 'ok' : (e.getAttribute('aria-label') || e.innerText).trim() ? 'notitle' : 'noname')");
check("rail: every control has a name and a title", names.every((n) => n === "ok"), names.join(","));
const railAxe = await axeRun();
check("rail: axe violations", railAxe.length === 0, railAxe.join("; ") || "0");
await size(1280, 800, 1, "reduce");
await ev("document.querySelector('.sidebar-toggle').click()");
await sleep(100);
const reduced = await ev(railFacts);
check("sidebar: instant under reduced motion", reduced.duration.split(",").every((d) => parseFloat(d) === 0), reduced.duration);
await size(1280, 800);
check("sidebar: expanded again shows the rings", reduced.levelsShown && reduced.ringsShown > 0, `${reduced.ringsShown}`);

// --- wide screens: the page column is centred in main, with no empty band
// on one side; the Stats rings sit in one row.
for (const [w, h, label, scale = 1] of [[1280, 800, "1280"], [1440, 900, "1440"], [1920, 1080, "1920"], [2560, 1440, "2560"], [2560, 1440, "1920 at 75%", 0.75]]) {
  await size(w, h, scale);
  for (const page of ["#/", "#/exercises", "#/history", "#/stats"]) {
    await load(page);
    const f = await ev(`(() => { const m = document.querySelector('main').getBoundingClientRect(); const p = document.querySelector('main .page').getBoundingClientRect(); const tops = [...document.querySelectorAll('.rings-large li')].map((li) => Math.round(li.getBoundingClientRect().top)); return { left: Math.round(p.left - m.left), right: Math.round(m.right - p.right), width: Math.round(p.width), rows: new Set(tops).size, overflow: document.documentElement.scrollWidth > innerWidth }; })()`);
    const ok = Math.abs(f.left - f.right) <= 2 && !f.overflow && (page !== "#/stats" || f.rows <= 1);
    check(`wide ${label} ${page}: centred`, ok, `band left ${f.left} right ${f.right}, page ${f.width}px${page === "#/stats" ? `, ring rows ${f.rows}` : ""}`);
  }
}
await size(1280, 800);

// --- the reset dialog, by keyboard. It is not confirmed here unless "wipe".
await load("#/");
await ev("document.querySelector('.sidebar-help').click()");
await sleep(400);
await ev("document.querySelector('dialog.dialog-help .help-wipe').focus()");
await enter();
await sleep(400);
check("reset: Enter on Reset all data opens the dialog", await ev("!!document.querySelector('dialog.dialog-wipe[open]')"));
check("reset: focus is in the confirmation field", await ev("document.activeElement?.id === 'wipe-word'"), await ev("document.activeElement?.id"));
const disabled = () => ev("document.querySelector('dialog.dialog-wipe .wipe-confirm').disabled");
check("reset: the button is disabled while the field is empty", await disabled());
word = await ev("document.querySelector('dialog.dialog-wipe label').innerText.match(/[A-ZÆØÅ]{4,}/)?.[0] ?? ''");
await type(word.slice(0, -1));
check("reset: still disabled one letter short", await disabled(), word.slice(0, -1));
await enter();
await sleep(300);
check("reset: Enter with the wrong word does nothing", await ev("!!document.querySelector('dialog.dialog-wipe[open]')"));
await type(word.slice(-1));
check(`reset: enabled once ${word} is typed`, !(await disabled()));
const wipeAxe = await axeRun();
check("reset: axe violations with the dialog open", wipeAxe.length === 0, wipeAxe.join("; ") || "0");
await esc();
await sleep(400);
check("reset: Esc closes it, help stays open", (await ev("!document.querySelector('dialog.dialog-wipe[open]')")) && (await ev("!!document.querySelector('dialog.dialog-help[open]')")));
check("reset: focus is back on Reset all data", await ev("document.activeElement?.classList.contains('help-wipe')"), await ev("document.activeElement?.className"));
await enter();
await sleep(300);
await ev("document.querySelector('dialog.dialog-wipe .wipe-cancel').click()");
await sleep(400);
check("reset: Cancel closes it and returns focus", (await ev("!document.querySelector('dialog.dialog-wipe[open]')")) && (await ev("document.activeElement?.classList.contains('help-wipe')")));
await esc();
await sleep(300);

}

if (wipe === "wipe") {
  await load("#/");
  await ev("document.querySelector('.sidebar-help').click()");
  await sleep(300);
  await ev("document.querySelector('dialog.dialog-help .help-wipe').click()");
  await sleep(300);
  word = await ev("document.querySelector('dialog.dialog-wipe label').innerText.match(/[A-ZÆØÅ]{4,}/)?.[0] ?? ''");
  await ev("document.getElementById('wipe-word').focus()");
  await type(word);
  const wipedAt = Date.now();
  await enter();
  for (let i = 0; i < 60 && !(await introOpen()); i++) await sleep(250);
  check("reset: after the reset the app reloads fresh and the intro opens", await introOpen());
  const get = (path) => ev(`fetch('${path}', { headers: { 'X-RootCause-Token': document.querySelector('meta[name=rootcause-token]').content } }).then((r) => r.json())`);
  let ov;
  for (let i = 0; i < 20 && !ov?.stats; i++) {
    ov = await get("/api/overview").catch(() => undefined);
    if (!ov?.stats) await sleep(250);
  }
  // Where the server builds the statistics they are rebuilt at once, so
  // built_at is either never (the cluster) or after the reset.
  const builtAt = ov?.stats?.built_at;
  check("reset: no session, statistics never built or rebuilt since", ov?.active === null && (builtAt === null || Date.parse(builtAt) >= wipedAt - 1000), JSON.stringify({ active: ov?.active, built_at: builtAt }));
  const history = await get("/api/history");
  check("reset: the history is empty", Array.isArray(history) && history.length === 0, `${history?.length} rows`);
  check("reset: stored preferences were cleared", (await ev("localStorage.getItem('rootcause.prefs')")) === null);
}

} catch (e) {
  check("shell checks ran to the end", false, String(e).slice(0, 300));
}

// Leave the intro finished for the checks that run after this one.
await ev("localStorage.setItem('rootcause.prefs', JSON.stringify({ introDone: true }))");
console.log(results.join("\n"));
await fetch(`http://127.0.0.1:${port}/json/close/${target.id}`);
ws.close();
