import { useMemo } from "react";
import { api, type ExerciseRow, type Overview } from "../api";
import { sessionHref, usePoll } from "../hooks";
import { useI18n } from "../i18n";
import { Icon } from "../ui/icons";
import { Banner, Button, Difficulty, Skeleton, Stars, TopicLabel } from "../ui/kit";
import { StartProgress, useStart } from "./start";

interface Props {
  overview: Overview | undefined;
  offline: boolean;
  reloadOverview: () => void;
  onStarting: (on: boolean) => void;
}

type Tag = "next" | "due" | "new" | "solved" | "tried";

interface Row {
  ex: ExerciseRow;
  tag: Tag;
  dueAt?: string;
}

// Inbox is the home page (spec section 11): the tickets as a helpdesk inbox.
// The recommended one is pinned first with why it was picked; a session in
// progress is pinned above everything.
export function Inbox({ overview, offline, reloadOverview, onStarting }: Props) {
  const { t } = useI18n();
  const list = usePoll(api.exercises, 15000, "inbox");
  const { start, busy, error } = useStart(reloadOverview, onStarting);

  const rows = useMemo(() => (overview && list.data ? inboxRows(overview, list.data) : null), [overview, list.data]);
  const active = overview?.active ?? null;
  const starting = overview?.starting ?? null;
  const locked = active !== null || starting !== null || busy !== null || offline;

  return (
    <div className="page page-inbox">
      <header className="page-head">
        <h1 id="page-title" tabIndex={-1}>
          {t("web.inbox.title")}
        </h1>
        {overview && <StatsLine overview={overview} />}
      </header>

      {error && <Banner tone="fail">{error}</Banner>}

      {active && (
        <a className="pinned pinned-active" href={sessionHref(active.id)}>
          <span className="live-dot" aria-hidden="true" />
          <span className="pinned-text">
            <span className="label">{t("web.inbox.in_progress")}</span>
            <span className="mono">{active.exercise}</span>
          </span>
          <span className="btn btn-primary">
            <span className="btn-label">{t("web.inbox.resume")}</span>
            <Icon name="arrowRight" />
          </span>
        </a>
      )}
      {active && <p className="note">{t("web.inbox.locked_active")}</p>}

      {!rows ? (
        <Skeleton rows={6} className="ticket-skeleton" />
      ) : (
        <ul className="tickets" aria-label={t("web.inbox.title")}>
          {rows.map((r) => (
            <TicketRow
              key={r.ex.id}
              row={r}
              overview={overview!}
              locked={locked}
              lockedReason={offline ? t("web.error.load") : active ? t("web.inbox.locked_active") : undefined}
              busy={busy === r.ex.id || (busy === "" && r.tag === "next")}
              onStart={() => start(r.tag === "next" ? "" : r.ex.id)}
              starting={starting && starting.exercise === r.ex.id ? starting : busy === r.ex.id || (busy === "" && r.tag === "next") ? (starting ?? { exercise: r.ex.id, step: 0, steps: 4 }) : null}
            />
          ))}
        </ul>
      )}
      {overview?.source === "fallback" && <p className="note">{t("web.stats.fallback")}</p>}
    </div>
  );
}

// inboxRows orders the catalog for the inbox: the recommended ticket, then
// the ones due for review, then the rest by id.
function inboxRows(o: Overview, exercises: ExerciseRow[]): Row[] {
  const now = Date.now();
  const due = new Map((o.review ?? []).map((r) => [r.exercise, r]));
  const rows: Row[] = exercises.map((ex) => {
    const review = due.get(ex.id);
    // A give-up stores 0 stars; a solve always has 1 or more.
    let tag: Tag = ex.last === null ? "new" : (ex.best_stars ?? 0) > 0 ? "solved" : "tried";
    if (review && new Date(review.due_at).getTime() <= now) tag = "due";
    if (o.next?.exercise === ex.id) tag = "next";
    return { ex, tag, dueAt: review?.due_at };
  });
  const rank: Record<Tag, number> = { next: 0, due: 1, new: 2, tried: 3, solved: 4 };
  return rows.sort((a, b) => rank[a.tag] - rank[b.tag] || a.ex.id.localeCompare(b.ex.id));
}

function TicketRow({
  row,
  overview,
  locked,
  lockedReason,
  busy,
  onStart,
  starting,
}: {
  row: Row;
  overview: Overview;
  locked: boolean;
  lockedReason?: string;
  busy: boolean;
  onStart: () => void;
  starting: Overview["starting"];
}) {
  const { t } = useI18n();
  const { ex, tag } = row;
  const next = tag === "next";
  return (
    <li className={next ? "ticket-row is-next" : "ticket-row"}>
      <div className="ticket-main">
        <span className={`status-dot status-${tag}`} aria-hidden="true" />
        <div className="ticket-text">
          <p className="ticket-title">{ex.title}</p>
          <p className="ticket-meta">
            <span className="ticket-from">{ex.from}</span>
            <TopicLabel topic={ex.topic} />
            <Difficulty n={ex.difficulty} />
            <span className="mono ticket-id">{ex.id}</span>
          </p>
        </div>
        <div className="ticket-side">
          <StatusTag row={row} />
          <Button
            variant={next ? "primary" : "secondary"}
            icon={next ? "play" : undefined}
            busy={busy}
            disabled={locked && !busy}
            title={locked ? lockedReason : undefined}
            onClick={onStart}
          >
            {next ? t("web.inbox.open_ticket") : t("web.inbox.start")}
          </Button>
        </div>
      </div>
      {next && <WhyPicked overview={overview} ex={ex} />}
      {starting && <StartProgress starting={starting} />}
    </li>
  );
}

function StatusTag({ row }: { row: Row }) {
  const { t, day } = useI18n();
  switch (row.tag) {
    case "next":
      return <span className="tag tag-next">{t("web.tag.next")}</span>;
    case "due":
      return <span className="tag tag-due">{t("web.tag.due")}</span>;
    case "new":
      return <span className="tag tag-new">{t("web.tag.new")}</span>;
    case "solved":
      return (
        <span className="tag tag-solved">
          <Stars n={row.ex.best_stars} size={13} />
        </span>
      );
    default:
      return row.dueAt ? <span className="tag tag-date">{t("web.tag.due_on", { date: day(row.dueAt) })}</span> : <span className="tag">{t("web.tag.tried")}</span>;
  }
}

// WhyPicked explains the recommendation: the rule that picked it, the last
// result on it, and what would change that.
function WhyPicked({ overview, ex }: { overview: Overview; ex: ExerciseRow }) {
  const { t, tn, date } = useI18n();
  const next = overview.next;
  if (!next) return null;
  const level = overview.levels.find((l) => l.topic === next.topic);
  const topic = t(`web.topic.${next.topic}`);
  const rule = next.reason === "review" ? t("web.why.rule.review") : t("web.why.rule.weakest", { topic, level: next.level });
  const last = ex.last
    ? t("web.why.last", { outcome: t(`web.outcome.${ex.last.status}`), date: ex.last.ended_at ? date(ex.last.ended_at) : "-" })
    : t("web.why.never");
  let after: string;
  if (next.reason === "review") after = t("web.why.after.review");
  else if (level && level.needed > 0) after = tn("web.why.after.level", level.needed - level.streak, { topic, next: level.level + 1 });
  else after = t("web.why.after.top", { topic });
  return (
    <details className="why">
      <summary>
        <Icon name="chevronRight" size={14} className="why-chevron" />
        {t("web.why.summary")}
      </summary>
      <dl>
        <dt>{t("web.why.rule_label")}</dt>
        <dd>{rule}</dd>
        <dt>{t("web.why.last_label")}</dt>
        <dd>{last}</dd>
        <dt>{t("web.why.after_label")}</dt>
        <dd>{after}</dd>
      </dl>
    </details>
  );
}

// StatsLine is the one line about the statistics: building, failed, built at,
// or never built.
export function StatsLine({ overview }: { overview: Overview }) {
  const { t, date } = useI18n();
  const s = overview.stats;
  if (s.updating)
    return (
      <p className="stats-line">
        <span className="spinner" aria-hidden="true" /> {t("web.stats.updating")}
      </p>
    );
  if (s.error) return <p className="stats-line is-fail">{t("web.stats.failed")}</p>;
  if (!s.built_at) return <p className="stats-line">{t("web.stats.never_built")}</p>;
  return <p className="stats-line">{t("web.stats.built_at", { time: date(s.built_at) })}</p>;
}
