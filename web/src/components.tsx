import { useState } from "react";
import type { Theme } from "./hooks";
import type { Lang } from "./api";
import { useI18n } from "./i18n";

// Mark is the RootCause logo: a trunk splitting into roots, one of them
// highlighted as the cause.
export function Mark({ size = 28 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      aria-hidden="true"
      className="mark"
      fill="none"
      strokeWidth="2.4"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M16 3v11" stroke="currentColor" />
      <path d="M16 14c0 5-7 5-9 11" stroke="currentColor" />
      <path d="M16 14c0 5 7 5 9 11" stroke="currentColor" />
      <path d="M16 14v10" className="mark-cause" />
      <circle cx="16" cy="26.5" r="2.6" className="mark-cause-dot" />
    </svg>
  );
}

export function Stars({ n }: { n: number | null }) {
  const { t } = useI18n();
  if (n === null) return <span className="muted">-</span>;
  return (
    <span className="stars" role="img" aria-label={t("web.stars", { stars: n })} title={t("web.stars", { stars: n })}>
      {[1, 2, 3].map((i) => (
        <svg key={i} viewBox="0 0 20 20" width="16" height="16" aria-hidden="true" className={i <= n ? "star on" : "star"}>
          <path d="M10 1.8l2.5 5.2 5.7.8-4.1 4 1 5.6L10 14.7l-5.1 2.7 1-5.6-4.1-4 5.7-.8z" />
        </svg>
      ))}
    </span>
  );
}

export function LevelBar({ level }: { level: number }) {
  return (
    <span className="levelbar" aria-hidden="true">
      {[1, 2, 3].map((i) => (
        <span key={i} className={i <= level ? "seg on" : "seg"} />
      ))}
    </span>
  );
}

export function TopicTag({ topic }: { topic: string }) {
  const { t } = useI18n();
  return <span className={`tag topic-${topic}`}>{t(`web.topic.${topic}`)}</span>;
}

export function ErrorBanner({ message }: { message: string }) {
  if (!message) return null;
  return (
    <div className="banner error" role="alert">
      {message}
    </div>
  );
}

export function CopyCommand({ command }: { command: string }) {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);
  const copy = () => {
    navigator.clipboard?.writeText(command).then(
      () => {
        setCopied(true);
        window.setTimeout(() => setCopied(false), 1500);
      },
      () => setCopied(false),
    );
  };
  return (
    <div className="command">
      <code>
        <span className="prompt" aria-hidden="true">
          $
        </span>{" "}
        {command}
      </code>
      <button type="button" className="ghost small" onClick={copy}>
        {copied ? t("web.session.copied") : t("web.session.copy")}
      </button>
    </div>
  );
}

interface HeaderProps {
  route: string;
  theme: Theme;
  onTheme: (t: Theme) => void;
  activeId: string | null;
}

export function Header({ route, theme, onTheme, activeId }: HeaderProps) {
  const { t, lang, setLang } = useI18n();
  const links: [string, string, string][] = [
    ["home", "#/", t("web.nav.home")],
    ["exercises", "#/exercises", t("web.nav.exercises")],
    ["history", "#/history", t("web.nav.history")],
  ];
  return (
    <header className="topbar">
      <a href="#/" className="brand">
        <Mark />
        <span className="wordmark">{t("web.app_name")}</span>
        <span className="tagline">{t("web.tagline")}</span>
      </a>
      <nav className="nav">
        {links.map(([name, href, label]) => (
          <a key={name} href={href} className={route === name ? "current" : ""} aria-current={route === name ? "page" : undefined}>
            {label}
          </a>
        ))}
        {activeId && (
          <a
            href={`#/session/${encodeURIComponent(activeId)}`}
            className={route === "session" ? "current live" : "live"}
            aria-current={route === "session" ? "page" : undefined}
          >
            <span className="pulse" aria-hidden="true" />
            {t("web.nav.session")}
          </a>
        )}
      </nav>
      <div className="toggles">
        <div className="segmented" role="group" aria-label={t("web.lang.label")}>
          {(["en", "no"] as Lang[]).map((l) => (
            <button key={l} type="button" className={lang === l ? "on" : ""} aria-pressed={lang === l} onClick={() => setLang(l)}>
              {l.toUpperCase()}
            </button>
          ))}
        </div>
        <label className="theme-select">
          <span className="sr-only">{t("web.theme.label")}</span>
          <select value={theme} onChange={(e) => onTheme(e.target.value as Theme)} aria-label={t("web.theme.label")}>
            <option value="system">{t("web.theme.system")}</option>
            <option value="light">{t("web.theme.light")}</option>
            <option value="dark">{t("web.theme.dark")}</option>
          </select>
        </label>
      </div>
    </header>
  );
}
