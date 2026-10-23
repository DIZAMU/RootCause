import { useState } from "react";
import { api } from "../api";
import { ErrorBanner, Stars, TopicTag } from "../components";
import { sessionHref, usePoll } from "../hooks";
import { useI18n } from "../i18n";

interface Props {
  activeId: string | null;
  reloadOverview: () => void;
}

export function Exercises({ activeId, reloadOverview }: Props) {
  const { t, lang } = useI18n();
  const list = usePoll(api.exercises, 15000, lang);
  const [starting, setStarting] = useState("");
  const [error, setError] = useState("");

  const start = (id: string) => {
    setStarting(id);
    setError("");
    api.start(id).then(
      (s) => {
        reloadOverview();
        window.location.hash = sessionHref(s.id);
      },
      (e: unknown) => {
        setStarting("");
        setError(e instanceof Error ? e.message : String(e));
        reloadOverview();
      },
    );
  };

  return (
    <section className="card">
      <h1>{t("web.exercises.title")}</h1>
      <ErrorBanner message={error || list.error} />
      {starting && <p className="note"><span className="spinner small" aria-hidden="true" />{t("web.home.starting")}</p>}
      {!list.data ? (
        <div className="spinner" aria-label={t("web.loading")} />
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>{t("web.col.exercise")}</th>
              <th>{t("web.col.topic")}</th>
              <th>{t("web.col.difficulty")}</th>
              <th>{t("web.col.best")}</th>
              <th aria-hidden="true" />
            </tr>
          </thead>
          <tbody>
            {list.data.map((ex) => (
              <tr key={ex.id}>
                <td className="mono">{ex.id}</td>
                <td>
                  <TopicTag topic={ex.topic} />
                </td>
                <td>{ex.difficulty}</td>
                <td>{ex.best_stars === null ? <span className="muted small">{t("web.exercises.never")}</span> : <Stars n={ex.best_stars} />}</td>
                <td className="right">
                  <button type="button" className="small" onClick={() => start(ex.id)} disabled={activeId !== null || starting !== ""}>
                    {t("web.home.start")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
