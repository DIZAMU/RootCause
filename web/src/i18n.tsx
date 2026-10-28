// UI strings come from the Go bundle (locales/*.json) through /api/i18n, so
// the dashboard and the CLI share one copy of every string. The app itself
// holds no user-facing text.

import { createContext, useContext } from "react";
import type { Lang, Strings } from "./api";

export interface I18n {
  lang: Lang;
  t: (key: string, vars?: Record<string, string | number>) => string;
  // tn picks key_one or key_other by count (English and Norwegian both have
  // these two forms) and fills {count} and vars.
  tn: (key: string, count: number, vars?: Record<string, string | number>) => string;
  // num formats a number in the learner's locale.
  num: (n: number, digits?: number) => string;
  date: (iso: string) => string;
  day: (iso: string) => string; // the date without the time
  setLang: (lang: Lang) => void;
}

// fill replaces {name} tokens, like i18n.Fill in Go. Unknown tokens stay.
export function fill(template: string, vars?: Record<string, string | number>): string {
  if (!vars) return template;
  return template.replace(/\{([A-Za-z0-9_]+)\}/g, (m, name: string) =>
    name in vars ? String(vars[name]) : m,
  );
}

export function makeI18n(lang: Lang, strings: Strings, setLang: (l: Lang) => void): I18n {
  const locale = lang === "no" ? "nb-NO" : "en-GB";
  const fmt = new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" });
  const dayFmt = new Intl.DateTimeFormat(locale, { day: "numeric", month: "short" });
  return {
    lang,
    // A missing key shows the key itself, as the Go bundle does.
    t: (key, vars) => fill(strings[key] ?? key, vars),
    tn: (key, count, vars) => {
      const k = `${key}_${count === 1 ? "one" : "other"}`;
      return fill(strings[k] ?? k, { count, ...vars });
    },
    num: (n, digits = 0) => n.toLocaleString(locale, { minimumFractionDigits: digits, maximumFractionDigits: digits }),
    date: (iso) => fmt.format(new Date(iso)),
    day: (iso) => dayFmt.format(new Date(iso)),
    setLang,
  };
}

export const I18nContext = createContext<I18n | null>(null);

export function useI18n(): I18n {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error("useI18n outside I18nContext");
  return ctx;
}
