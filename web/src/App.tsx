import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api, type Lang, type Strings } from "./api";
import { usePoll, useRoute, useTheme } from "./hooks";
import { I18nContext, makeI18n } from "./i18n";
import { usePrefs } from "./prefs";
import { Intro } from "./shell/Intro";
import { HelpDialog, Palette, type PaletteItem } from "./shell/Overlays";
import { paletteEvent, useShortcuts } from "./shell/keyboard";
import { Sidebar } from "./shell/Sidebar";
import { LiveRegionProvider } from "./ui/kit";
import { Exercises } from "./views/Exercises";
import { History } from "./views/History";
import { Inbox } from "./views/Inbox";
import { SessionView } from "./views/Session";
import { Stats } from "./views/Stats";

// overviewPollMs is how often the app looks for a session the CLI started or
// finished, and for statistics that finished rebuilding.
const overviewPollMs = 4000;
// startingPollMs is how often it looks while a session is starting, to tick
// off its steps.
const startingPollMs = 500;

export function App() {
  const route = useRoute();
  const [theme, setTheme] = useTheme();
  const [prefs, setPrefs] = usePrefs();
  const [lang, setLangState] = useState<Lang>();
  const [strings, setStrings] = useState<Strings>();
  const [bootError, setBootError] = useState("");
  const [startingHere, setStartingHere] = useState(false);
  const [palette, setPalette] = useState(false);
  const [help, setHelp] = useState(false);
  // The sidebar animates only once the learner toggles it, never on load.
  const [sidebarMotion, setSidebarMotion] = useState(false);
  const setCollapsed = useCallback(
    (c: boolean) => {
      setSidebarMotion(true);
      setPrefs({ sidebarCollapsed: c });
    },
    [setPrefs],
  );

  const overview = usePoll(api.overview, startingHere ? startingPollMs : overviewPollMs, lang ?? "");

  // The learner's language lives in the profile, shared with the CLI. Follow
  // it when the CLI changes it.
  const serverLang = overview.data?.lang;
  useEffect(() => {
    if (serverLang && serverLang !== lang) setLangState(serverLang);
  }, [serverLang, lang]);

  useEffect(() => {
    if (!lang) return;
    api.strings(lang).then(
      (s) => {
        setStrings(s);
        setBootError("");
      },
      (e: unknown) => setBootError(e instanceof Error ? e.message : String(e)),
    );
  }, [lang]);

  const setLang = useCallback((l: Lang) => {
    api.setLang(l).then(
      () => setLangState(l),
      (e: unknown) => setBootError(e instanceof Error ? e.message : String(e)),
    );
  }, []);

  const i18n = useMemo(() => (lang && strings ? makeI18n(lang, strings, setLang) : null), [lang, strings, setLang]);

  useShortcuts({
    singleKeys: prefs.shortcuts,
    go: useCallback((hash: string) => {
      window.location.hash = hash;
    }, []),
    help: useCallback(() => setHelp(true), []),
    toggleSidebar: useCallback(() => setCollapsed(!prefs.sidebarCollapsed), [setCollapsed, prefs.sidebarCollapsed]),
  });
  useEffect(() => {
    const open = () => setPalette(true);
    window.addEventListener(paletteEvent, open);
    return () => window.removeEventListener(paletteEvent, open);
  }, []);

  // Each page has its own title, and after a navigation focus moves to the
  // page's heading, so a screen reader hears where it is.
  const first = useRef(true);
  const routeKey = route.name === "session" ? `session/${route.id}` : route.name;
  useEffect(() => {
    if (!i18n) return;
    document.documentElement.lang = i18n.lang === "no" ? "nb" : "en";
    if (route.name !== "session") document.title = `${i18n.t(`web.title.${route.name}`)} · ${i18n.t("web.app_name")}`;
  }, [i18n, route.name]);
  useEffect(() => {
    if (first.current) {
      first.current = false;
      return;
    }
    const id = window.setTimeout(() => document.getElementById("page-title")?.focus({ preventScroll: false }), 0);
    return () => window.clearTimeout(id);
  }, [routeKey]);

  const paletteActions = useMemo<PaletteItem[]>(() => {
    if (!i18n) return [];
    const { t } = i18n;
    return [
      { id: "act-lang", icon: "flag", label: t("web.palette.switch_lang"), hint: lang === "no" ? "EN" : "NO", run: () => setLang(lang === "no" ? "en" : "no") },
      { id: "act-theme", icon: theme === "dark" ? "sun" : "moon", label: t("web.palette.switch_theme"), run: () => setTheme(theme === "dark" ? "light" : "dark") },
      { id: "act-sidebar", icon: "panel", label: t("web.shortcuts.sidebar"), hint: "[", run: () => setCollapsed(!prefs.sidebarCollapsed) },
      { id: "act-help", icon: "help", label: t("web.help.title"), hint: "?", run: () => setHelp(true) },
    ];
  }, [i18n, lang, theme, setLang, setTheme, setCollapsed, prefs.sidebarCollapsed]);

  if (!i18n) {
    // No strings yet, so nothing translated can be shown. A failure here
    // means the server is gone; show the server's own error text if any.
    return (
      <div className="boot">
        {bootError || overview.error ? <p role="alert">{bootError || overview.error}</p> : <span className="spinner spinner-lg" aria-hidden="true" />}
      </div>
    );
  }

  const activeId = overview.data?.active?.id ?? null;
  const offline = overview.error !== "";
  return (
    <I18nContext.Provider value={i18n}>
      <LiveRegionProvider>
        <a className="skip-link" href="#main">
          {i18n.t("web.skip")}
        </a>
        <div className={["app", prefs.sidebarCollapsed ? "is-collapsed" : "", sidebarMotion ? "is-animating" : ""].join(" ").trim()}>
          <Sidebar
            route={route}
            overview={overview.data}
            theme={theme}
            onTheme={setTheme}
            collapsed={prefs.sidebarCollapsed}
            onCollapse={setCollapsed}
            onHelp={() => setHelp(true)}
          />
          <main id="main" className="main" tabIndex={-1}>
            {offline && (
              <div className="offline" role="alert">
                {i18n.t("web.error.load")}
              </div>
            )}
            {bootError && (
              <div className="offline" role="alert">
                {bootError}
              </div>
            )}
            {route.name === "home" && (
              <Inbox overview={overview.data} offline={offline} reloadOverview={overview.reload} onStarting={setStartingHere} />
            )}
            {route.name === "exercises" && (
              <Exercises focus={route.focus} levels={overview.data?.levels} activeId={activeId} offline={offline} reloadOverview={overview.reload} onStarting={setStartingHere} starting={overview.data?.starting ?? null} />
            )}
            {route.name === "history" && <History />}
            {route.name === "stats" && <Stats levels={overview.data?.levels} />}
            {route.name === "session" && (
              <SessionView
                id={route.id}
                activeId={activeId}
                activeExercise={overview.data?.active?.exercise ?? ""}
                offline={offline}
                prefs={prefs}
                setPrefs={setPrefs}
                reloadOverview={overview.reload}
              />
            )}
          </main>
        </div>
        <HelpDialog
          open={help}
          onClose={() => setHelp(false)}
          singleKeys={prefs.shortcuts}
          onSingleKeys={(on) => setPrefs({ shortcuts: on })}
          onIntro={() => {
            setHelp(false);
            setPrefs({ introDone: false });
          }}
        />
        <Intro open={!prefs.introDone && !help} terminal={overview.data?.terminal ?? true} onDone={() => setPrefs({ introDone: true })} />
        <Palette open={palette} onClose={() => setPalette(false)} overview={overview.data} actions={paletteActions} />
      </LiveRegionProvider>
    </I18nContext.Provider>
  );
}
