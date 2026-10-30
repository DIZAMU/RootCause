import { useEffect, useRef, useState } from "react";
import { api, type StatsPage, type TopicLevel } from "../api";
import { usePoll } from "../hooks";
import { useI18n } from "../i18n";
import { Icon } from "../ui/icons";
import { Banner, Button, EmptyState, Ring, Skeleton, useAnnounce, useDuration, useLevelLabel, useLevelNext } from "../ui/kit";

// Stats is the statistics view (ARCHITECTURE.md section 7.3): topic levels, solves per
// day, and the per-exercise statistics with calibration flags, all from the
// dbt marts.
export function Stats({ levels }: { levels: TopicLevel[] | undefined }) {
  const { t, num, date } = useI18n();
  const duration = useDuration();
  const announce = useAnnounce();
  const page = usePoll(api.stats, 5000, "stats");
  const levelLabel = useLevelLabel();
  const levelNext = useLevelNext();
  const [rebuildError, setRebuildError] = useState("");

  const p = page.data;
  const updating = p?.stats.updating ?? false;
  const rebuild = () => {
    setRebuildError("");
    api.rebuild().then(
      () => {
        announce(t("web.stats.updating"));
        page.reload();
      },
      (e: unknown) => setRebuildError(e instanceof Error ? e.message : String(e)),
    );
  };

  return (
    <div className="page page-stats">
      <header className="page-head">
        <h1 id="page-title" tabIndex={-1}>
          {t("web.stats.title")}
        </h1>
        {p && (
          <div className="stats-actions">
            <p className="stats-line">
              {updating ? (
                <>
                  <span className="spinner" aria-hidden="true" /> {t("web.stats.updating")}
                </>
              ) : p.stats.built_at ? (
                t("web.stats.built_at", { time: date(p.stats.built_at) })
              ) : (
                t("web.stats.never_built")
              )}
            </p>
            {p.can_rebuild ? (
              <Button icon="refresh" busy={updating} onClick={rebuild} title={updating ? t("web.stats.rebuild_running") : undefined}>
                {t("web.stats.rebuild")}
              </Button>
            ) : (
              <p className="note">{t("web.stats.rebuild_unavailable")}</p>
            )}
          </div>
        )}
      </header>
      {rebuildError && <Banner tone="warn">{rebuildError}</Banner>}
      {p?.stats.error && <Banner tone="fail">{t("web.stats.failed")}</Banner>}
      {page.error && !p && <Banner tone="fail">{page.error}</Banner>}

      {levels && (
        <section className="stats-section" aria-labelledby="levels-title">
          <h2 id="levels-title">{t("web.levels.title")}</h2>
          <ul className="rings-large">
            {levels.map((l) => {
              const label = levelLabel(l.topic, l.level, l.streak, l.needed);
              return (
                <li key={l.topic}>
                  <Ring level={l.level} streak={l.streak} needed={l.needed} label={label} size={72} />
                  <span className="ring-name">{t(`web.topic.${l.topic}`)}</span>
                  <span className="ring-text">{levelNext(l.level, l.streak, l.needed)}</span>
                </li>
              );
            })}
          </ul>
        </section>
      )}

      {!p ? (
        <Skeleton rows={8} className="table-skeleton" />
      ) : !p.built ? (
        <EmptyState title={t("web.stats.empty_title")}>{t("web.stats.empty")}</EmptyState>
      ) : (
        <>
          <section className="stats-section" aria-labelledby="days-title">
            <h2 id="days-title">{t("web.stats.days_title")}</h2>
            <p className="note">{t("web.stats.days_sub")}</p>
            {p.days.length === 0 ? <p className="muted">{t("web.stats.days_empty")}</p> : <DaysChart days={p.days} />}
          </section>

          <section className="stats-section" aria-labelledby="ex-title">
            <h2 id="ex-title">{t("web.stats.exercises_title")}</h2>
            <div className="table-wrap is-scroll" role="region" aria-label={t("web.stats.exercises_title")} tabIndex={0}>
              <table className="table table-sticky">
                <thead>
                  <tr>
                    <th scope="col">{t("web.col.ticket")}</th>
                    <th scope="col" className="num">
                      {t("web.col.sessions")}
                    </th>
                    <th scope="col" className="num">
                      {t("web.col.solve_rate")}
                    </th>
                    <th scope="col" className="num">
                      {t("web.col.avg_hints")}
                    </th>
                    <th scope="col" className="num">
                      {t("web.col.avg_time")}
                    </th>
                    <th scope="col">{t("web.col.calibration")}</th>
                  </tr>
                </thead>
                <tbody>
                  {p.exercises.map((e) => (
                    <tr key={e.id}>
                      <td>
                        <span className="cell-title">{e.title}</span>
                        <span className="cell-sub mono">{e.id}</span>
                      </td>
                      <td className="num">{num(e.sessions)}</td>
                      <td className="num">{e.solve_rate === null ? "-" : `${num(e.solve_rate * 100)}%`}</td>
                      <td className="num">{e.avg_hints === null ? "-" : num(e.avg_hints, 1)}</td>
                      <td className="num nowrap">{e.avg_solve_seconds === null ? "-" : duration(e.avg_solve_seconds)}</td>
                      <td>
                        {e.calibration_flag ? (
                          <span className="flag">
                            <Icon name="flag" size={14} />
                            {t(`web.stats.flag.${e.calibration_flag}`)}
                          </span>
                        ) : e.sessions >= 10 ? (
                          <span className="muted">{t("web.stats.flag.none")}</span>
                        ) : (
                          <span className="muted" title={t("web.stats.flag.too_few")}>
                            -<span className="sr-only">{t("web.stats.flag.too_few")}</span>
                          </span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <p className="note">{t("web.stats.flag_note")}</p>
          </section>
        </>
      )}
    </div>
  );
}

// DaysChart draws solves per day as columns on one axis. Every column has a
// tooltip on hover and focus, and the same numbers are in a table below. The
// drawing is as wide as its box at one unit per pixel, so it fills a wide
// screen and its text stays at its size; the height is fixed.
function DaysChart({ days }: { days: StatsPage["days"] }) {
  const { t, num, lang } = useI18n();
  const [hover, setHover] = useState<number | null>(null);
  const box = useRef<HTMLDivElement>(null);
  const [W, setW] = useState(640);
  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => e && setW(Math.max(280, Math.round(e.contentRect.width))));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const shown = days.slice(-30);
  const max = Math.max(1, ...shown.map((d) => d.solves));
  const ticks = niceTicks(max);
  const top = ticks[ticks.length - 1] ?? 1;
  const H = 220,
    left = 32,
    bottom = 24,
    plotH = H - bottom - 8;
  const band = (W - left) / shown.length;
  const barW = Math.min(24, band - 2);
  const y = (v: number) => 8 + plotH - (v / top) * plotH;
  const dayLabel = (iso: string) => new Date(`${iso}T00:00:00Z`).toLocaleDateString(lang === "no" ? "nb-NO" : "en-GB", { day: "numeric", month: "short", timeZone: "UTC" });
  const tip = hover !== null ? shown[hover] : null;
  return (
    <div className="chart" ref={box}>
      <svg viewBox={`0 0 ${W} ${H}`} height={H} className="chart-svg" role="img" aria-label={t("web.stats.chart_label", { days: shown.length })}>
        {ticks.map((v) => (
          <g key={v}>
            <line x1={left} x2={W} y1={y(v)} y2={y(v)} className="chart-grid" />
            <text x={left - 6} y={y(v)} className="chart-tick" textAnchor="end" dominantBaseline="central">
              {num(v)}
            </text>
          </g>
        ))}
        {shown.map((d, i) => {
          const x = left + i * band + (band - barW) / 2;
          const h = Math.max(0, y(0) - y(d.solves));
          const r = Math.min(4, h);
          return (
            <g
              key={d.day}
              className={hover === i ? "chart-col is-hover" : "chart-col"}
              role="img"
              tabIndex={0}
              aria-label={t("web.stats.chart_point", { day: dayLabel(d.day), solves: num(d.solves), sessions: num(d.sessions) })}
              onPointerEnter={() => setHover(i)}
              onPointerLeave={() => setHover(null)}
              onFocus={() => setHover(i)}
              onBlur={() => setHover(null)}
            >
              <rect x={left + i * band} y={8} width={band} height={plotH} className="chart-hit" />
              {h > 0 && (
                <path
                  className="chart-bar"
                  d={`M${x},${y(0)} V${y(d.solves) + r} Q${x},${y(d.solves)} ${x + r},${y(d.solves)} H${x + barW - r} Q${x + barW},${y(d.solves)} ${x + barW},${y(d.solves) + r} V${y(0)} Z`}
                />
              )}
            </g>
          );
        })}
        <line x1={left} x2={W} y1={y(0)} y2={y(0)} className="chart-axis" />
        {shown.map((d, i) =>
          i === 0 || i === shown.length - 1 || (shown.length > 6 && i % Math.ceil(shown.length / 6) === 0) ? (
            <text key={d.day} x={left + i * band + band / 2} y={H - 6} className="chart-tick" textAnchor="middle">
              {dayLabel(d.day)}
            </text>
          ) : null,
        )}
      </svg>
      {tip && (
        <div className="chart-tip" style={{ left: `${((left + (hover! + 0.5) * band) / W) * 100}%` }} aria-hidden="true">
          <strong>{dayLabel(tip.day)}</strong>
          <span>{t("web.stats.tip_solves", { solves: num(tip.solves), sessions: num(tip.sessions) })}</span>
          {tip.avg_stars !== null && <span>{t("web.stats.tip_stars", { stars: num(tip.avg_stars, 1) })}</span>}
        </div>
      )}
      <details className="chart-table">
        <summary>{t("web.stats.show_table")}</summary>
        <table className="table">
          <thead>
            <tr>
              <th scope="col">{t("web.col.day")}</th>
              <th scope="col" className="num">
                {t("web.col.sessions")}
              </th>
              <th scope="col" className="num">
                {t("web.col.solves")}
              </th>
              <th scope="col" className="num">
                {t("web.col.avg_stars")}
              </th>
            </tr>
          </thead>
          <tbody>
            {shown.map((d) => (
              <tr key={d.day}>
                <td>{dayLabel(d.day)}</td>
                <td className="num">{num(d.sessions)}</td>
                <td className="num">{num(d.solves)}</td>
                <td className="num">{d.avg_stars === null ? "-" : num(d.avg_stars, 1)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </details>
    </div>
  );
}

// niceTicks returns 0 and up to four round steps covering max.
function niceTicks(max: number): number[] {
  const step = max <= 4 ? 1 : max <= 10 ? 2 : max <= 25 ? 5 : Math.ceil(max / 4 / 5) * 5;
  const out: number[] = [];
  let v = 0;
  for (; v < max && out.length < 5; v += step) out.push(v);
  out.push(v);
  return out;
}
