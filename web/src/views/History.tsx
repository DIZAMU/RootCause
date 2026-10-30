import { useMemo, useState } from "react";
import { api, type SessionStatus } from "../api";
import { sessionHref, usePoll } from "../hooks";
import { useI18n } from "../i18n";
import { Banner, EmptyState, Skeleton, Stars, TopicLabel, useDuration } from "../ui/kit";

const topics = ["permissions", "services", "logs", "networking", "disk"];
const outcomes: SessionStatus[] = ["solved", "gave_up", "expired", "voided"];

// History lists the finished sessions (ARCHITECTURE.md section 11), newest first, with
// filters by topic and outcome.
export function History() {
  const { t, date, num } = useI18n();
  const duration = useDuration();
  const list = usePoll(api.history, 15000, "history");
  const catalog = usePoll(api.exercises, 60000, "history-catalog");
  const [topic, setTopic] = useState("");
  const [outcome, setOutcome] = useState("");

  const byId = useMemo(() => new Map((catalog.data ?? []).map((e) => [e.id, e])), [catalog.data]);
  const rows = (list.data ?? []).filter((h) => (!topic || byId.get(h.exercise)?.topic === topic) && (!outcome || h.status === outcome));

  return (
    <div className="page page-history">
      <header className="page-head">
        <h1 id="page-title" tabIndex={-1}>
          {t("web.history.title")}
        </h1>
      </header>
      {list.error && !list.data && <Banner tone="fail">{list.error}</Banner>}
      {!list.data ? (
        <Skeleton rows={8} className="table-skeleton" />
      ) : list.data.length === 0 ? (
        <EmptyState
          title={t("web.history.empty_title")}
          action={
            <a className="btn btn-primary" href="#/">
              <span className="btn-label">{t("web.history.empty_action")}</span>
            </a>
          }
        >
          {t("web.history.empty")}
        </EmptyState>
      ) : (
        <>
          <div className="filters">
            <label>
              <span className="label">{t("web.col.topic")}</span>
              <select value={topic} onChange={(e) => setTopic(e.target.value)}>
                <option value="">{t("web.filter.all")}</option>
                {topics.map((tp) => (
                  <option key={tp} value={tp}>
                    {t(`web.topic.${tp}`)}
                  </option>
                ))}
              </select>
            </label>
            <label>
              <span className="label">{t("web.col.outcome")}</span>
              <select value={outcome} onChange={(e) => setOutcome(e.target.value)}>
                <option value="">{t("web.filter.all")}</option>
                {outcomes.map((o) => (
                  <option key={o} value={o}>
                    {t(`web.outcome.${o}`)}
                  </option>
                ))}
              </select>
            </label>
            <p className="filters-count" aria-live="polite">
              {t("web.filter.count", { shown: num(rows.length), total: num(list.data.length) })}
            </p>
          </div>
          <div className="table-wrap is-scroll" role="region" aria-label={t("web.history.title")} tabIndex={0}>
            <table className="table table-sticky">
              <thead>
                <tr>
                  <th scope="col">{t("web.col.ended")}</th>
                  <th scope="col">{t("web.col.ticket")}</th>
                  <th scope="col">{t("web.col.outcome")}</th>
                  <th scope="col">{t("web.col.stars")}</th>
                  <th scope="col" className="num">
                    {t("web.col.hints")}
                  </th>
                  <th scope="col" className="num">
                    {t("web.col.checks")}
                  </th>
                  <th scope="col" className="num">
                    {t("web.col.duration")}
                  </th>
                </tr>
              </thead>
              <tbody>
                {rows.map((h) => {
                  const ex = byId.get(h.exercise);
                  return (
                    <tr key={h.session}>
                      <td className="num nowrap">{h.ended_at ? date(h.ended_at) : "-"}</td>
                      <td>
                        <a href={sessionHref(h.session)} className="cell-title">
                          {ex?.title ?? h.exercise}
                        </a>
                        <span className="cell-sub">
                          {ex && <TopicLabel topic={ex.topic} />} <span className="mono">{h.exercise}</span>
                        </span>
                      </td>
                      <td>
                        <span className={`tag tag-outcome-${h.status}`}>{t(`web.outcome.${h.status}`)}</span>
                      </td>
                      <td>{h.status === "solved" ? <Stars n={h.stars} size={14} /> : <span className="muted">-</span>}</td>
                      <td className="num">{num(h.hints)}</td>
                      <td className="num">{num(h.checks)}</td>
                      <td className="num nowrap">{h.ended_at ? duration((new Date(h.ended_at).getTime() - new Date(h.started_at).getTime()) / 1000) : "-"}</td>
                    </tr>
                  );
                })}
                {rows.length === 0 && (
                  <tr>
                    <td colSpan={7} className="muted">
                      {t("web.filter.none")}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}
