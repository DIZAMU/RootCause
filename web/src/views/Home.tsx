import { useState } from "react";
import { api, type Overview } from "../api";
import { ErrorBanner, LevelBar, TopicTag } from "../components";
import { sessionHref } from "../hooks";
import { useI18n } from "../i18n";

interface Props {
  overview: Overview | undefined;
  reloadOverview: () => void;
}

export function Home({ overview, reloadOverview }: Props) {
  const { t, date } = useI18n();
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState("");

  if (!overview) return <div className="spinner" aria-label={t("web.loading")} />;

  const start = () => {
    setStarting(true);
    setError("");
    api.start("").then(
      (s) => {
        reloadOverview();
        window.location.hash = sessionHref(s.id);
      },
      (e: unknown) => {
        setStarting(false);
        setError(e instanceof Error ? e.message : String(e));
        reloadOverview();
      },
    );
  };

  const { active, next } = overview;
  return (
    <div className="home">
      <ErrorBanner message={error} />

      {active ? (
        <section className="card hero active-hero">
          <div>
            <p className="eyebrow live-text">
              <span className="pulse" aria-hidden="true" />
              {t("web.home.active_title")}
            </p>
            <h1 className="mono">{active.exercise}</h1>
            <p className="muted">{t("web.home.active_body", { exercise: active.exercise })}</p>
          </div>
          <a className="button primary" href={sessionHref(active.id)}>
            {t("web.home.open")}
          </a>
        </section>
      ) : (
        <section className="card hero">
          <div>
            <p className="eyebrow">{t("web.home.next_title")}</p>
            {next ? (
              <>
                <h1 className="mono">{next.exercise}</h1>
                <p className="meta-row">
                  <TopicTag topic={next.topic} />
                  <span className="muted">{t("web.difficulty", { difficulty: next.difficulty })}</span>
                </p>
                <p className="muted">
                  {next.reason === "review"
                    ? t("web.home.reason.review")
                    : t("web.home.reason.weakest_topic", { topic: t(`web.topic.${next.topic}`), level: next.level })}
                </p>
              </>
            ) : (
              <p className="muted">{t("web.next.none")}</p>
            )}
            {starting && <p className="muted">{t("web.home.starting")}</p>}
          </div>
          {next && (
            <button type="button" className="primary" onClick={start} disabled={starting}>
              {starting ? <span className="spinner small" aria-hidden="true" /> : null}
              {t("web.home.start")}
            </button>
          )}
        </section>
      )}

      <StatsNote overview={overview} />

      <div className="grid two">
        <section className="card">
          <h2>{t("web.home.levels_title")}</h2>
          <ul className="levels">
            {overview.levels.map((l) => (
              <li key={l.topic}>
                <TopicTag topic={l.topic} />
                <LevelBar level={l.level} />
                <span className="muted small">{t("web.home.level", { level: l.level })}</span>
              </li>
            ))}
          </ul>
        </section>

        <section className="card">
          <h2>{t("web.home.review_title")}</h2>
          {overview.review === null ? (
            <p className="muted">{t("web.home.review_unavailable")}</p>
          ) : overview.review.length === 0 ? (
            <p className="muted">{t("web.home.review_empty")}</p>
          ) : (
            <ul className="review">
              {overview.review.map((r) => (
                <li key={r.exercise}>
                  <span className="mono">{r.exercise}</span>
                  <span className={r.due_now ? "tag due" : "muted small"}>
                    {r.due_now ? t("web.home.review_due_now") : t("web.home.review_due", { due: date(r.due_at) })}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </div>
  );
}

// StatsNote says when the statistics are rebuilding, stale or failed, and
// when they were last built.
function StatsNote({ overview }: { overview: Overview }) {
  return (
    <>
      <StatsState overview={overview} />
      <BuiltAt builtAt={overview.stats.built_at} />
    </>
  );
}

// BuiltAt shows when dbt last built the statistics, so statistics that lag
// behind the latest session are visibly older.
function BuiltAt({ builtAt }: { builtAt: string | null }) {
  const { t, date } = useI18n();
  if (!builtAt) return <p className="note">{t("web.stats.never_built")}</p>;
  return <p className="note">{t("web.stats.built_at", { time: date(builtAt) })}</p>;
}

function StatsState({ overview }: { overview: Overview }) {
  const { t } = useI18n();
  const { stats, source } = overview;
  if (stats.updating) {
    return (
      <p className="note">
        <span className="spinner small" aria-hidden="true" />
        {t("web.stats.updating")}
      </p>
    );
  }
  if (stats.error) {
    return (
      <div className="banner warn" role="status">
        <p>{t("web.stats.failed")}</p>
        <pre className="detail">{stats.error}</pre>
      </div>
    );
  }
  if (source === "fallback") return <p className="note">{t("web.stats.fallback")}</p>;
  return null;
}
