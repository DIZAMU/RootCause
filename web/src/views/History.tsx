import { api } from "../api";
import { ErrorBanner, Stars } from "../components";
import { sessionHref, usePoll } from "../hooks";
import { useI18n } from "../i18n";

export function History() {
  const { t, date, lang } = useI18n();
  const list = usePoll(api.history, 10000, lang);

  const duration = (start: string, end: string | null): string => {
    if (!end) return "-";
    const minutes = Math.max(0, Math.round((Date.parse(end) - Date.parse(start)) / 60000));
    if (minutes < 60) return t("web.duration.minutes", { minutes });
    return t("web.duration.hours", { hours: Math.floor(minutes / 60), minutes: minutes % 60 });
  };

  return (
    <section className="card">
      <h1>{t("web.history.title")}</h1>
      <ErrorBanner message={list.error} />
      {!list.data ? (
        <div className="spinner" aria-label={t("web.loading")} />
      ) : list.data.length === 0 ? (
        <p className="muted">{t("web.history.empty")}</p>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>{t("web.col.ended")}</th>
              <th>{t("web.col.exercise")}</th>
              <th>{t("web.col.outcome")}</th>
              <th>{t("web.col.stars")}</th>
              <th>{t("web.col.hints")}</th>
              <th>{t("web.col.checks")}</th>
              <th>{t("web.col.duration")}</th>
            </tr>
          </thead>
          <tbody>
            {list.data.map((h) => (
              <tr key={h.session}>
                <td className="muted small">{h.ended_at ? date(h.ended_at) : "-"}</td>
                <td>
                  <a className="mono" href={sessionHref(h.session)}>
                    {h.exercise}
                  </a>
                </td>
                <td>
                  <span className={`tag outcome-${h.status}`}>{t(`web.outcome.${h.status}`)}</span>
                </td>
                <td>{h.status === "solved" ? <Stars n={h.stars} /> : <span className="muted">-</span>}</td>
                <td>{h.hints}</td>
                <td>{h.checks}</td>
                <td className="muted small">{duration(h.started_at, h.ended_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
