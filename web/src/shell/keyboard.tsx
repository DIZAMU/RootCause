// Keyboard shortcuts (ARCHITECTURE.md section 11). Two kinds:
//   - with a modifier: Ctrl+K (or Cmd+K) opens the palette, Ctrl+Enter runs
//     the check. They work everywhere; inside the terminal, WebTerminal
//     catches them before the shell sees them and sends the same events.
//   - single keys (g then i/e/h/s, ?, [): only when the learner has them on
//     (WCAG 2.1.4), and never while typing in a field, the terminal or a
//     dialog.
import { useEffect, useRef } from "react";

// Events the shortcuts send. The session view listens for the check; the app
// for the palette.
export const checkEvent = "rootcause:check";
export const paletteEvent = "rootcause:palette";

// isTyping reports whether keys pressed at el belong to what is being typed.
export function isTyping(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  if (el.closest(".xterm, dialog[open]")) return true;
  const tag = el.tagName;
  return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || el.isContentEditable;
}

interface Handlers {
  singleKeys: boolean;
  go: (hash: string) => void;
  help: () => void;
  toggleSidebar: () => void;
}

const goTargets: Record<string, string> = { i: "#/", e: "#/exercises", h: "#/history", s: "#/stats" };

export function useShortcuts({ singleKeys, go, help, toggleSidebar }: Handlers) {
  const pendingG = useRef(0);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const mod = e.ctrlKey || e.metaKey;
      if (mod && !e.altKey && !e.shiftKey && e.key.toLowerCase() === "k") {
        e.preventDefault();
        window.dispatchEvent(new Event(paletteEvent));
        return;
      }
      if (mod && e.key === "Enter") {
        if (document.querySelector("dialog[open]")) return;
        e.preventDefault();
        window.dispatchEvent(new Event(checkEvent));
        return;
      }
      if (!singleKeys || mod || e.altKey || isTyping(e.target)) return;
      const target = goTargets[e.key];
      if (pendingG.current && Date.now() - pendingG.current < 1500 && target) {
        e.preventDefault();
        pendingG.current = 0;
        go(target);
        return;
      }
      pendingG.current = 0;
      if (e.key === "g") {
        pendingG.current = Date.now();
      } else if (e.key === "?") {
        e.preventDefault();
        help();
      } else if (e.key === "[") {
        e.preventDefault();
        toggleSidebar();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [singleKeys, go, help, toggleSidebar]);
}
