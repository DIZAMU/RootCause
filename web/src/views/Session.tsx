import { useState } from "react";
import { api, type Session } from "../api";
import { CopyCommand, ErrorBanner, Stars, TopicTag } from "../components";
import { sessionHref, usePoll } from "../hooks";
import { useI18n } from "../i18n";
import { WebTerminal } from "../WebTerminal";

// sessionPollMs is how often an open session is re-read, so hints, checks and
// results from the CLI show up here (plan decision D2).
const sessionPollMs = 2000;

type Action = "hint" | "check" | "reset" | "giveup";

interface Props {
  id: string;
  activeId: string | null;
  activeExercise: string;
  reloadOverview: () => void;
}

export function SessionView({ id, activeId, activeExercise, reloadOverview }: Props) {
  const { t, lang } = useI18n();
  const [open, setOpen] = useState(true);
  const polled = usePoll(() => api.session(id), sessionPollMs, `${id}/${lang}`, open);
  const [busy, setBusy] = useState<Action | null>(null);
  const [error, setError] = useState("");
  const [lastCheck, setLastCheck] = useState<Session["check"]>();

  const s = polled.data;
  const isActive = s?.status === "active";
  if (s && !isActive && open) setOpen(false);
  if (s && isActive && !open) setOpen(true);

  if (polled.error && !s) return <ErrorBanner message={polled.error} />;
  if (!s) return <div className="spinner" aria-label={t("web.loading")} />;

  const run = (action: Action) => {
    if (action === "reset" && !window.confirm(t("web.session.reset_confirm"))) return;
    if (action === "giveup" && !window.confirm(t("web.session.giveup_confirm"))) return;
    setBusy(action);
    setError("");
    const call = { hint: api.hint, check: api.check, reset: api.reset, giveup: api.giveUp }[action];
    call(s.id).then(
      (next) => {
        polled.set(next);
        if (action === "check") setLastCheck(next.check);
        if (action === "reset") setLastCheck(undefined);
        setBusy(null);
        reloadOverview();
      },
      (e: unknown) => {
        setError(e instanceof Error ? e.message : String(e));
        setBusy(null);
        polled.reload();
      },
    );
  };

  const nextTier = s.hints.length + 1;
  const newer = activeId && activeId !== s.id;

  return (
    <div className="session">
      {newer && (
        <div className="banner info">
          {t("web.session.newer", { exercise: activeExercise })}{" "}
          <a href={sessionHref(activeId)}>{t("web.session.open_newer")}</a>
        </div>
      )}
      <ErrorBanner message={error} />

      <div className="grid session-grid">
        <div className="stack">
          <article className="card ticket">
            <header className="ticket-head">
              <span className="eyebrow">{t("web.session.ticket")}</span>
              <span className="mono muted small">{s.exercise}</span>
            </header>
            <h1>{s.ticket.title}</h1>
            <p className="ticket-from">
              <span className="muted">{t("web.session.from")}</span> {s.ticket.from}
            </p>
            <div className="ticket-body">{s.ticket.body}</div>
            <footer className="meta-row">
              <TopicTag topic={s.topic} />
              <span className="muted small">{t("web.difficulty", { difficulty: s.difficulty })}</span>
            </footer>
          </article>

          {isActive ? (
            <>
              {s.terminal ? (
                <section className="card">
                  <h2>{t("web.session.terminal_title")}</h2>
                  <WebTerminal sessionId={s.id} />
                </section>
              ) : (
                <section className="card">
                  <h2>{t("web.session.shell_title")}</h2>
                  <p className="muted">{t("web.session.shell_body")}</p>
                  <CopyCommand command={s.shell_command} />
                </section>
              )}

              <CheckResult check={lastCheck} />

              <section className="card actions">
                <button type="button" className="primary" onClick={() => run("check")} disabled={busy !== null}>
                  {busy === "check" && <span className="spinner small" aria-hidden="true" />}
                  {busy === "check" ? t("web.session.checking") : t("web.session.check")}
                </button>
                <button type="button" onClick={() => run("reset")} disabled={busy !== null}>
                  {busy === "reset" && <span className="spinner small" aria-hidden="true" />}
                  {busy === "reset" ? t("web.session.resetting") : t("web.session.reset")}
                </button>
                <button
                  type="button"
                  className="danger"
                  onClick={() => run("giveup")}
                  disabled={busy !== null || !s.can_give_up}
                  title={s.can_give_up ? undefined : t("giveup.locked")}
                >
                  {t("web.session.giveup")}
                </button>
                {!s.can_give_up && <span className="muted small">{t("giveup.locked")}</span>}
              </section>
            </>
          ) : (
            <Result s={s} />
          )}
        </div>

        <aside className="stack">
          <section className="card">
            <h2>{t("web.session.hints_title")}</h2>
            <p className="muted small">{t("web.session.hints_rule")}</p>
            <ol className="hints">
              {s.hints.map((h) => (
                <li key={h.tier}>
                  <span className="hint-tier">{t("web.session.hint_n", { tier: h.tier })}</span>
                  <p>{h.text}</p>
                </li>
              ))}
            </ol>
            {isActive && nextTier <= s.hints_total && (
              <button type="button" onClick={() => run("hint")} disabled={busy !== null}>
                {t("web.session.reveal", { tier: nextTier, total: s.hints_total })}
              </button>
            )}
          </section>

          <section className="card">
            <h2>{t("web.session.checks_title")}</h2>
            {s.checks.length === 0 ? (
              <p className="muted small">{t("web.session.checks_none")}</p>
            ) : (
              <ul className="checks">
                {s.checks.map((c, i) => (
                  <li key={i} className={c.passed ? "pass" : "fail"}>
                    <span className="dot" aria-hidden="true" />
                    <span>
                      <b>{c.passed ? t("web.session.check_passed") : t("web.session.check_failed")}</b>
                      {c.feedback && <span className="muted"> {c.feedback}</span>}
                    </span>
                    <Time iso={c.at} />
                  </li>
                ))}
              </ul>
            )}
          </section>

          <section className="card">
            <h2>{t("web.session.activity")}</h2>
            <ul className="timeline">
              {s.events.map((e, i) => (
                <li key={i}>
                  <span>{t(`web.event.${e.type}`, { tier: e.tier ?? "" })}</span>
                  <Time iso={e.at} />
                </li>
              ))}
            </ul>
            {isActive && <p className="muted small">{t("web.session.expires", { time: timeOf(s.expires_at, lang) })}</p>}
          </section>
        </aside>
      </div>
    </div>
  );
}

// paragraphs splits a YAML block text into paragraphs at blank lines, and
// joins the hard-wrapped lines inside each one.
function paragraphs(text: string): string[] {
  return text
    .split(/\n\s*\n/)
    .map((p) => p.replace(/\s*\n\s*/g, " ").trim())
    .filter((p) => p !== "");
}

function timeOf(iso: string, lang: string): string {
  return new Date(iso).toLocaleTimeString(lang === "no" ? "nb-NO" : "en-GB", { hour: "2-digit", minute: "2-digit" });
}

function Time({ iso }: { iso: string }) {
  const { lang } = useI18n();
  return (
    <time className="muted small" dateTime={iso}>
      {timeOf(iso, lang)}
    </time>
  );
}

// CheckResult announces the last check run from this page.
function CheckResult({ check }: { check: Session["check"] }) {
  const { t } = useI18n();
  if (!check || check.result === "pass") return null;
  if (check.result === "infra") return <div className="banner error" role="status">{t("check.infra")}</div>;
  return (
    <div className="banner warn" role="status">
      {check.feedback ? t("check.fail", { feedback: check.feedback }) : t("check.fail_generic")}
    </div>
  );
}

// Result is the end of a session: outcome, stars, level change and the
// walkthrough.
function Result({ s }: { s: Session }) {
  const { t } = useI18n();
  const lc = s.level_change;
  return (
    <section className={`card result result-${s.status}`}>
      <div className="result-head">
        <div>
          <p className="eyebrow">{t(`web.event.${s.status}`)}</p>
          <h2>{s.status === "solved" ? t("check.pass") : t(`web.result.${s.status}`)}</h2>
        </div>
        {s.status === "solved" && <Stars n={s.stars} />}
      </div>
      {lc && (
        <p className={lc.to > lc.from ? "level-change up" : "level-change down"}>
          {t(lc.to > lc.from ? "level.up" : "level.down", { topic: t(`web.topic.${lc.topic}`), from: lc.from, to: lc.to })}
        </p>
      )}
      {s.walkthrough && (
        <div className="walkthrough">
          <h3>{t("web.result.walkthrough")}</h3>
          <div className="prose">
            {paragraphs(s.walkthrough).map((para, i) => (
              <p key={i}>{para}</p>
            ))}
          </div>
        </div>
      )}
      <p>
        <a className="button primary" href="#/">
          {t("web.result.next")}
        </a>
      </p>
    </section>
  );
}
