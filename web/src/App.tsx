import { useCallback, useEffect, useMemo, useState } from "react";
import { api, type Lang, type Strings } from "./api";
import { ErrorBanner, Header } from "./components";
import { usePoll, useRoute, useTheme } from "./hooks";
import { I18nContext, makeI18n } from "./i18n";
import { Exercises } from "./views/Exercises";
import { History } from "./views/History";
import { Home } from "./views/Home";
import { SessionView } from "./views/Session";

// overviewPollMs is how often the app looks for a session the CLI started or
// finished, and for statistics that finished rebuilding.
const overviewPollMs = 4000;

export function App() {
  const route = useRoute();
  const [theme, setTheme] = useTheme();
  const [lang, setLangState] = useState<Lang>();
  const [strings, setStrings] = useState<Strings>();
  const [bootError, setBootError] = useState("");

  const overview = usePoll(api.overview, overviewPollMs, lang ?? "");

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

  const i18n = useMemo(
    () => (lang && strings ? makeI18n(lang, strings, setLang) : null),
    [lang, strings, setLang],
  );

  useEffect(() => {
    if (!i18n) return;
    document.documentElement.lang = i18n.lang === "no" ? "nb" : "en";
    document.title = i18n.t("web.app_name");
  }, [i18n]);

  if (!i18n) {
    // No strings yet, so nothing translated can be shown. A failure here
    // means the server is gone; show the server's own error text if any.
    return (
      <div className="boot">
        {bootError || overview.error ? <p role="alert">{bootError || overview.error}</p> : <div className="spinner" aria-hidden="true" />}
      </div>
    );
  }

  const activeId = overview.data?.active?.id ?? null;
  return (
    <I18nContext.Provider value={i18n}>
      <Header route={route.name} theme={theme} onTheme={setTheme} activeId={activeId} />
      <main className="page">
        {overview.error && <ErrorBanner message={i18n.t("web.error.load")} />}
        <ErrorBanner message={bootError} />
        {route.name === "home" && <Home overview={overview.data} reloadOverview={overview.reload} />}
        {route.name === "exercises" && <Exercises activeId={activeId} reloadOverview={overview.reload} />}
        {route.name === "history" && <History />}
        {route.name === "session" && (
          <SessionView id={route.id} activeId={activeId} activeExercise={overview.data?.active?.exercise ?? ""} reloadOverview={overview.reload} />
        )}
      </main>
    </I18nContext.Provider>
  );
}
