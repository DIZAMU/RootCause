// Per-browser preferences, kept in localStorage. Storage can be blocked or
// cleared; every preference then falls back to its default and the page works
// the same.
import { useCallback, useState } from "react";

export interface Prefs {
  sidebarCollapsed: boolean;
  shortcuts: boolean; // the single-key shortcuts (g i, ?), WCAG 2.1.4
  timer: boolean; // show the elapsed time in a session
  introDismissed: boolean;
}

const defaults: Prefs = { sidebarCollapsed: false, shortcuts: true, timer: false, introDismissed: false };
const key = "rootcause.prefs";

function read(): Prefs {
  try {
    const raw = window.localStorage.getItem(key);
    if (raw) return { ...defaults, ...(JSON.parse(raw) as Partial<Prefs>) };
  } catch {
    // blocked or invalid: the defaults apply
  }
  return defaults;
}

export function usePrefs(): [Prefs, (change: Partial<Prefs>) => void] {
  const [prefs, setPrefs] = useState<Prefs>(read);
  const update = useCallback((change: Partial<Prefs>) => {
    setPrefs((p) => {
      const next = { ...p, ...change };
      try {
        window.localStorage.setItem(key, JSON.stringify(next));
      } catch {
        // blocked: the change lasts for this page only
      }
      return next;
    });
  }, []);
  return [prefs, update];
}
