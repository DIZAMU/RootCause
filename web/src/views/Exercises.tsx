import { useEffect } from "react";
import { api, type Overview, type TopicLevel } from "../api";
import { usePoll } from "../hooks";
import { useI18n } from "../i18n";
import { Banner, Button, Difficulty, Ring, Skeleton, Stars, useLevelLabel } from "../ui/kit";
import { StartProgress, useStart } from "./start";

const topics = ["permissions", "services", "logs", "networking", "disk"];

interface Props {
  focus?: string;
  levels: TopicLevel[] | undefined;
  activeId: string | null;
  offline: boolean;
  starting: Overview["starting"];
  reloadOverview: () => void;
  onStarting: (on: boolean) => void;
}

// Exercises is the catalog (spec section 11): every exercise by topic, with
// the ticket, difficulty, best stars and when it was last played.
export function Exercises({ focus, levels, activeId, offline, starting, reloadOverview, onStarting }: Props) {
  const { t, date } = useI18n();
  const list = usePoll(api.exercises, 15000, "exercises");
  const levelLabel = useLevelLabel();
  const { start, busy, error } = useStart(reloadOverview, onStarting);
  const locked = activeId !== null || starting !== null || busy !== null || offline;

  useEffect(() => {
    if (!focus || !list.data) return;
    const row = document.getElementById(`ex-${focus}`);
    row?.scrollIntoView({ block: "center" });
    row?.querySelector<HTMLElement>("button")?.focus({ preventScroll: true });
  }, [focus, list.data]);

  return (
    <div className="page page-exercises">
      <header className="page-head">
        <h1 id="page-title" tabIndex={-1}>
          {t("web.exercises.title")}
        </h1>
        <p className="page-sub">{t("web.exercises.sub")}</p>
      </header>
      {error && <Banner tone="fail">{error}</Banner>}
      {list.error && !list.data && <Banner tone="fail">{list.error}</Banner>}
      {!list.data ? (
        <Skeleton rows={9} className="table-skeleton" />
      ) : (
        topics.map((topic) => {
          const rows = list.data!.filter((e) => e.topic === topic).sort((a, b) => a.difficulty - b.difficulty || a.id.localeCompare(b.id));
          if (rows.length === 0) return null;
          const lv = levels?.find((l) => l.topic === topic);
          return (
            <section key={topic} className="catalog-group" aria-labelledby={`topic-${topic}`}>
              <header className="group-head">
                {lv && <Ring level={lv.level} streak={lv.streak} needed={lv.needed} label={levelLabel(topic, lv.level, lv.streak, lv.needed)} size={28} />}
                <h2 id={`topic-${topic}`}>{t(`web.topic.${topic}`)}</h2>
              </header>
              <div className="table-wrap" role="region" aria-labelledby={`topic-${topic}`} tabIndex={0}>
                <table className="table">
                  <thead>
                    <tr>
                      <th scope="col">{t("web.col.ticket")}</th>
                      <th scope="col">{t("web.col.difficulty")}</th>
                      <th scope="col">{t("web.col.best")}</th>
                      <th scope="col">{t("web.col.last_played")}</th>
                      <th scope="col">
                        <span className="sr-only">{t("web.col.actions")}</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((ex) => (
                      <tr key={ex.id} id={`ex-${ex.id}`} className={focus === ex.id ? "is-focus" : undefined}>
                        <td>
                          <span className="cell-title">{ex.title}</span>
                          <span className="cell-sub mono">{ex.id}</span>
                          {(starting?.exercise === ex.id || busy === ex.id) && <StartProgress starting={starting} />}
                        </td>
                        <td>
                          <Difficulty n={ex.difficulty} />
                        </td>
                        <td>{(ex.best_stars ?? 0) === 0 ? <span className="muted">{t("web.exercises.never")}</span> : <Stars n={ex.best_stars} size={14} />}</td>
                        <td className="num">{ex.last?.ended_at ? date(ex.last.ended_at) : <span className="muted">-</span>}</td>
                        <td className="cell-action">
                          <Button busy={busy === ex.id} disabled={locked && busy !== ex.id} title={locked && activeId ? t("web.inbox.locked_active") : undefined} onClick={() => start(ex.id)}>
                            {t("web.inbox.start")}
                          </Button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
          );
        })
      )}
    </div>
  );
}
