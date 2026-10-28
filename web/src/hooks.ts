import { useCallback, useEffect, useRef, useState } from "react";
import { flushSync } from "react-dom";

export type Route =
  | { name: "home" }
  | { name: "exercises"; focus?: string }
  | { name: "history" }
  | { name: "stats" }
  | { name: "session"; id: string };

// parseRoute reads the hash: #/, #/exercises (or #/exercises/<id> to show
// one), #/history, #/stats, #/session/<id>.
export function parseRoute(hash: string): Route {
  const parts = hash.replace(/^#\/?/, "").split("/");
  switch (parts[0]) {
    case "exercises":
      return parts[1] ? { name: "exercises", focus: decodeURIComponent(parts[1]) } : { name: "exercises" };
    case "history":
      return { name: "history" };
    case "stats":
      return { name: "stats" };
    case "session":
      if (parts[1]) return { name: "session", id: decodeURIComponent(parts[1]) };
      return { name: "home" };
    default:
      return { name: "home" };
  }
}

export function sessionHref(id: string): string {
  return `#/session/${encodeURIComponent(id)}`;
}

export function useRoute(): Route {
  const [route, setRoute] = useState(() => parseRoute(window.location.hash));
  useEffect(() => {
    // A route change cross-fades with the View Transitions API where the
    // browser has it and the learner has not asked for reduced motion;
    // otherwise it switches at once.
    const onHash = () => {
      const next = parseRoute(window.location.hash);
      const doc = document as Document & { startViewTransition?: (cb: () => void) => unknown };
      if (doc.startViewTransition && window.matchMedia("(prefers-reduced-motion: no-preference)").matches) {
        doc.startViewTransition(() => flushSync(() => setRoute(next)));
      } else {
        setRoute(next);
      }
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);
  return route;
}

export interface Polled<T> {
  data: T | undefined;
  error: string;
  reload: () => void;
  set: (v: T) => void;
}

// usePoll loads with fn now, then every intervalMs while the page is visible
// and enabled is true, and again whenever the window gets focus. key changes
// (a new id, a new language) start over.
export function usePoll<T>(
  fn: () => Promise<T>,
  intervalMs: number,
  key: string,
  enabled = true,
): Polled<T> {
  const [data, setData] = useState<T>();
  const [error, setError] = useState("");
  const fnRef = useRef(fn);
  fnRef.current = fn;
  const seq = useRef(0);

  const reload = useCallback(() => {
    const mine = ++seq.current;
    fnRef.current().then(
      (v) => {
        if (mine !== seq.current) return; // a newer load started
        setData(v);
        setError("");
      },
      (e: unknown) => {
        if (mine !== seq.current) return;
        setError(e instanceof Error ? e.message : String(e));
      },
    );
  }, []);

  useEffect(() => {
    setData(undefined);
    reload();
  }, [key, reload]);

  useEffect(() => {
    window.addEventListener("focus", reload);
    return () => window.removeEventListener("focus", reload);
  }, [reload]);

  useEffect(() => {
    if (!enabled) return;
    const id = window.setInterval(() => {
      if (document.visibilityState === "visible") reload();
    }, intervalMs);
    return () => window.clearInterval(id);
  }, [enabled, intervalMs, reload]);

  const set = useCallback((v: T) => {
    seq.current++; // drop any load still in flight
    setData(v);
    setError("");
  }, []);

  return { data, error, reload, set };
}

export type Theme = "system" | "light" | "dark";

const themeKey = "rootcause.theme";

function readTheme(): Theme {
  try {
    const v = window.localStorage.getItem(themeKey);
    if (v === "light" || v === "dark") return v;
  } catch {
    // storage blocked: use the system setting
  }
  return "system";
}

// useTheme keeps the choice in localStorage and on <html data-theme>.
export function useTheme(): [Theme, (t: Theme) => void] {
  const [theme, setTheme] = useState<Theme>(readTheme);
  useEffect(() => {
    const root = document.documentElement;
    // Switch without transitions for a frame, so the whole page does not
    // fade from one theme to the other.
    root.classList.add("no-transitions");
    void root.offsetHeight;
    requestAnimationFrame(() => requestAnimationFrame(() => root.classList.remove("no-transitions")));
    if (theme === "system") root.removeAttribute("data-theme");
    else root.setAttribute("data-theme", theme);
    try {
      if (theme === "system") window.localStorage.removeItem(themeKey);
      else window.localStorage.setItem(themeKey, theme);
    } catch {
      // storage blocked: the choice lasts for this page only
    }
  }, [theme]);
  return [theme, setTheme];
}
