import type { Lang, Overview } from "../api";
import { sessionHref, type Route, type Theme } from "../hooks";
import { useI18n } from "../i18n";
import { Icon, type IconName } from "../ui/icons";
import { Mark, Ring, useLevelLabel } from "../ui/kit";

interface Props {
  route: Route;
  overview: Overview | undefined;
  theme: Theme;
  onTheme: (t: Theme) => void;
  collapsed: boolean;
  onCollapse: (collapsed: boolean) => void;
  onHelp: () => void;
}

const pages: { name: Route["name"]; href: string; icon: IconName; label: string }[] = [
  { name: "home", href: "#/", icon: "inbox", label: "web.nav.inbox" },
  { name: "exercises", href: "#/exercises", icon: "catalog", label: "web.nav.exercises" },
  { name: "history", href: "#/history", icon: "history", label: "web.nav.history" },
  { name: "stats", href: "#/stats", icon: "stats", label: "web.nav.stats" },
];

const themes: Theme[] = ["system", "light", "dark"];
const themeIcons: Record<Theme, IconName> = { system: "monitor", light: "sun", dark: "moon" };

// Sidebar is the app's navigation (ARCHITECTURE.md section 11): the pages, the active
// session, the topic level rings, the language and theme switches, and the
// help button. On a desktop it collapses to a rail of icons: the rings go,
// and the two switches become one button each that moves to the next value.
// On a phone it becomes a top bar.
export function Sidebar({ route, overview, theme, onTheme, collapsed, onCollapse, onHelp }: Props) {
  const { t, lang, setLang } = useI18n();
  const levelLabel = useLevelLabel();
  const active = overview?.active;
  const otherLang: Lang = lang === "no" ? "en" : "no";
  const nextTheme = themes[(themes.indexOf(theme) + 1) % themes.length] ?? "system";
  return (
    <nav className={collapsed ? "sidebar is-collapsed" : "sidebar"} aria-label={t("web.nav.label")}>
      <div className="sidebar-head">
        <a href="#/" className="brand" aria-label={t("web.app_name")} title={t("web.app_name")}>
          <Mark />
          <span className="brand-name">{t("web.app_name")}</span>
        </a>
        <button
          type="button"
          className="icon-btn sidebar-toggle"
          onClick={() => onCollapse(!collapsed)}
          aria-label={collapsed ? t("web.nav.expand") : t("web.nav.collapse")}
          aria-expanded={!collapsed}
          title={collapsed ? t("web.nav.expand") : t("web.nav.collapse")}
        >
          <Icon name="panel" />
        </button>
      </div>

      <ul className="nav-list">
        {pages.map((p) => {
          const current = route.name === p.name;
          return (
            <li key={p.name}>
              <a href={p.href} className={current ? "nav-item is-current" : "nav-item"} aria-current={current ? "page" : undefined} title={t(p.label)}>
                <Icon name={p.icon} />
                <span className="nav-label">{t(p.label)}</span>
              </a>
            </li>
          );
        })}
        {active && (
          <li>
            <a
              href={sessionHref(active.id)}
              className={route.name === "session" ? "nav-item nav-session is-current" : "nav-item nav-session"}
              aria-current={route.name === "session" ? "page" : undefined}
              title={t("web.nav.session")}
            >
              <span className="live-dot" aria-hidden="true" />
              <span className="nav-label">
                {t("web.nav.session")}
                <span className="nav-sub mono">{active.exercise}</span>
              </span>
            </a>
          </li>
        )}
      </ul>

      {overview && (
        <section className="sidebar-levels" aria-label={t("web.levels.title")} inert={collapsed}>
          <h2 className="label">{t("web.levels.title")}</h2>
          <ul>
            {overview.levels.map((l) => {
              const label = levelLabel(l.topic, l.level, l.streak, l.needed);
              return (
                <li key={l.topic} title={label}>
                  <Ring level={l.level} streak={l.streak} needed={l.needed} label={label} size={32} />
                  <span className="ring-topic">{t(`web.topic.${l.topic}`)}</span>
                </li>
              );
            })}
          </ul>
        </section>
      )}

      <div className="sidebar-foot">
        {collapsed ? (
          <>
            <button
              type="button"
              className="icon-btn rail-lang"
              onClick={() => setLang(otherLang)}
              title={t("web.lang.switch", { current: t(`web.lang.name.${lang}`), other: t(`web.lang.name.${otherLang}`) })}
            >
              {lang.toUpperCase()}
              <span className="sr-only">, {t("web.lang.switch", { current: t(`web.lang.name.${lang}`), other: t(`web.lang.name.${otherLang}`) })}</span>
            </button>
            <button
              type="button"
              className="icon-btn"
              onClick={() => onTheme(nextTheme)}
              aria-label={t("web.theme.switch", { current: t(`web.theme.${theme}`), next: t(`web.theme.${nextTheme}`) })}
              title={t("web.theme.switch", { current: t(`web.theme.${theme}`), next: t(`web.theme.${nextTheme}`) })}
            >
              <Icon name={themeIcons[theme]} />
            </button>
          </>
        ) : (
          <>
            <div className="seg" role="group" aria-label={t("web.lang.label")}>
              {(["en", "no"] as Lang[]).map((l) => (
                <button key={l} type="button" className={lang === l ? "seg-btn is-on" : "seg-btn"} aria-pressed={lang === l} onClick={() => setLang(l)}>
                  {l.toUpperCase()}
                </button>
              ))}
            </div>
            <div className="seg" role="group" aria-label={t("web.theme.label")}>
              {themes.map((value) => (
                <button
                  key={value}
                  type="button"
                  className={theme === value ? "seg-btn is-on" : "seg-btn"}
                  aria-pressed={theme === value}
                  aria-label={t(`web.theme.${value}`)}
                  title={t(`web.theme.${value}`)}
                  onClick={() => onTheme(value)}
                >
                  <Icon name={themeIcons[value]} size={16} />
                </button>
              ))}
            </div>
          </>
        )}
        <button type="button" className="icon-btn sidebar-help" onClick={onHelp} aria-label={t("web.help.title")} title={t("web.help.title")}>
          <Icon name="help" />
        </button>
      </div>
    </nav>
  );
}
