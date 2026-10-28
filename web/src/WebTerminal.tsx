import { memo, useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { openTerminal } from "./api";
import { useI18n } from "./i18n";

// The terminal is this many rows high; its width follows the card.
const ROWS = 24;

type State = { kind: "connecting" } | { kind: "open" } | { kind: "closed"; reason: string };

// WebTerminal is a shell in the session's sandbox (v2): xterm.js on a
// WebSocket to /api/terminal, where the server runs bash in the sandbox pod.
// The socket closes with a reason when the shell exits or the session ends;
// the learner can open it again while the session is active. It is memoised
// on the session id, so nothing around it re-renders the terminal.
export const WebTerminal = memo(function WebTerminal({ sessionId }: { sessionId: string }) {
  const { t } = useI18n();
  const box = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<State>({ kind: "connecting" });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    setState({ kind: "connecting" });
    let disposed = false;
    let cleanup = () => {};
    // Measure cells in the terminal's own font: wait until it has loaded.
    void document.fonts.load('14px "JetBrains Mono"').finally(() => {
      if (!disposed) cleanup = attach(el);
    });
    return () => {
      disposed = true;
      cleanup();
    };
  }, [sessionId, attempt]);

  // attach opens the terminal in el and connects it; it returns the cleanup.
  const attach = (el: HTMLDivElement) => {
    const term = new Terminal({
      rows: ROWS,
      cursorBlink: true,
      fontFamily: '"JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
      fontSize: 14,
      scrollback: 2000,
    });
    term.open(el);
    // Ctrl+Enter (run the check) and Ctrl+K (the palette) are the app's: the
    // shell never receives them. The key event goes on to the app's own
    // listener, which acts on it.
    //
    // Tab completes in the shell, so the terminal would trap the keyboard
    // (WCAG 2.1.2). Esc, then Tab or Shift+Tab, leaves it instead: the Esc
    // still reaches the shell, the Tab moves focus on.
    let afterEscape = false;
    term.attachCustomKeyEventHandler((e) => {
      if ((e.ctrlKey || e.metaKey) && (e.key === "Enter" || e.key.toLowerCase() === "k")) return false;
      if (e.type !== "keydown") return true;
      if (e.key === "Tab" && afterEscape) {
        afterEscape = false;
        return false;
      }
      afterEscape = e.key === "Escape";
      return true;
    });

    const ws = openTerminal();
    ws.binaryType = "arraybuffer";
    const send = (msg: object) => {
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(msg));
    };

    // Fit the columns to the card's width, from the width of one character
    // cell in the terminal's font.
    const fit = () => {
      const cell = measureCell(el);
      if (cell <= 0) return;
      const cols = Math.max(20, Math.floor((el.clientWidth - 16) / cell));
      if (cols !== term.cols) term.resize(cols, ROWS);
      send({ cols: term.cols, rows: term.rows });
    };
    const observer = new ResizeObserver(fit);
    observer.observe(el);

    ws.onopen = () => {
      setState({ kind: "open" });
      fit();
      term.focus();
    };
    ws.onmessage = (e: MessageEvent<ArrayBuffer>) => term.write(new Uint8Array(e.data));
    ws.onclose = (e) => setState({ kind: "closed", reason: e.reason });
    const input = term.onData((data) => send({ input: data }));

    return () => {
      input.dispose();
      observer.disconnect();
      ws.close();
      term.dispose();
    };
  };

  return (
    <div className="web-terminal">
      <div ref={box} className="web-terminal-screen" />
      {/* One status line, always there, so nothing below it moves. */}
      <p className="terminal-status">{state.kind === "connecting" ? t("web.terminal.connecting") : t("web.terminal.leave")}</p>
      {state.kind === "closed" && (
        <div className="banner banner-warn" role="status">
          <p className="banner-text">{t(closeText(state.reason))}</p>
          {state.reason !== "session_ended" && state.reason !== "no_session" && (
            <button type="button" className="btn btn-secondary" onClick={() => setAttempt((n) => n + 1)}>
              <span className="btn-label">{t("web.terminal.reconnect")}</span>
            </button>
          )}
        </div>
      )}
    </div>
  );
});

// closeText maps the server's close reason to a message key.
function closeText(reason: string): string {
  switch (reason) {
    case "session_ended":
    case "no_session":
      return "web.terminal.session_ended";
    case "shell_exited":
      return "web.terminal.shell_exited";
    default:
      return "web.terminal.lost";
  }
}

// measureCell returns the width of one character in the terminal's font, in
// pixels, by measuring a line of them inside el.
function measureCell(el: HTMLElement): number {
  const probe = document.createElement("span");
  probe.className = "xterm-char-probe";
  probe.textContent = "W".repeat(50);
  el.appendChild(probe);
  const width = probe.getBoundingClientRect().width / 50;
  probe.remove();
  return width;
}
