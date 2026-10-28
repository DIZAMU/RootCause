import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { api, type Session } from "../api";
import { sessionHref, usePoll } from "../hooks";
import { useI18n } from "../i18n";
import type { Prefs } from "../prefs";
import { checkEvent } from "../shell/keyboard";
import { Icon } from "../ui/icons";
import { Banner, Button, ConfirmButton, Difficulty, IconSwap, Prose, Stars, Time, TopicLabel, timeOf, useAnnounce, useDelayed } from "../ui/kit";

// The terminal (xterm.js) is the largest part of the bundle; it loads only
// when a session page with a web terminal opens.
const WebTerminal = lazy(() => import("../WebTerminal").then((m) => ({ default: m.WebTerminal })));

// sessionPollMs is how often an open session is re-read, so hints, checks and
// results from the CLI show up here.
const sessionPollMs = 2000;

type Action = "hint" | "check" | "reset" | "giveup";

interface Props {
  id: string;
  activeId: string | null;
  activeExercise: string;
  offline: boolean;
  prefs: Prefs;
  setPrefs: (p: Partial<Prefs>) => void;
  reloadOverview: () => void;
}

export function SessionView({ id, activeId, activeExercise, offline, prefs, setPrefs, reloadOverview }: Props) {
  const { t, tn, lang } = useI18n();
  const announce = useAnnounce();
  const [open, setOpen] = useState(true);
  const polled = usePoll(() => api.session(id), sessionPollMs, `${id}/${lang}`, open);
  const [busy, setBusy] = useState<Action | null>(null);
  const [error, setError] = useState("");
  const [lastCheck, setLastCheck] = useState<Session["check"]>();
  const [solvedHere, setSolvedHere] = useState(false);

  const s = polled.data;
  const isActive = s?.status === "active";
  if (s && !isActive && open) setOpen(false);
  if (s && isActive && !open) setOpen(true);

  useEffect(() => {
    if (s) document.title = `${s.ticket.title} · ${t("web.app_name")}`;
  }, [s?.ticket.title, t]);

  const run = (action: Action) => {
    if (!s || busy !== null || offline) return;
    setBusy(action);
    setError("");
    const call = { hint: api.hint, check: api.check, reset: api.reset, giveup: api.giveUp }[action];
    call(s.id).then(
      (next) => {
        polled.set(next);
        if (action === "check") {
          setLastCheck(next.check);
          if (next.check?.result === "pass") {
            setSolvedHere(true);
            announce(t("web.solve.announce", { stars: next.check.stars }));
          } else if (next.check?.result === "fail") {
            announce(next.check.feedback ? t("check.fail", { feedback: next.check.feedback }) : t("check.fail_generic"));
          } else announce(t("check.infra"));
        }
        if (action === "hint") announce(t("web.session.hint_n", { tier: next.hints.length }));
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

  // Ctrl+Enter (from anywhere, the terminal included) runs the check.
  const runRef = useRef(run);
  runRef.current = run;
  useEffect(() => {
    const onCheck = () => runRef.current("check");
    window.addEventListener(checkEvent, onCheck);
    return () => window.removeEventListener(checkEvent, onCheck);
  }, []);

  if (polled.error && !s) return <Banner tone="fail">{polled.error}</Banner>;
  if (!s) return <SessionSkeleton />;

  const newer = activeId && activeId !== s.id;
  const nextTier = s.hints.length + 1;
  const hasNextHint = isActive && nextTier <= s.hints_total;
  const disabled = busy !== null || offline;

  return (
    <div className="page page-session">
      {newer && (
        <Banner tone="info" action={<a href={sessionHref(activeId)}>{t("web.session.open_newer")}</a>}>
          {t("web.session.newer", { exercise: activeExercise })}
        </Banner>
      )}
      {error && <Banner tone="fail">{error}</Banner>}

      <div className="session-grid">
        <article className="ticket" aria-labelledby="page-title">
          <p className="label ticket-label">
            {t("web.session.ticket")} <span className="mono">{s.exercise}</span>
          </p>
          <h1 id="page-title" tabIndex={-1}>
            {s.ticket.title}
          </h1>
          <p className="ticket-sender">
            <span className="muted">{t("web.session.from")}</span> {s.ticket.from}
          </p>
          <Prose text={s.ticket.body} />
          <p className="ticket-facts">
            <TopicLabel topic={s.topic} />
            <Difficulty n={s.difficulty} />
            {isActive && <span className="muted">{t("web.session.expires", { time: timeOf(s.expires_at, lang) })}</span>}
          </p>

          {s.hints.length > 0 && (
            <section className="hints" aria-labelledby="hints-title">
              <h2 id="hints-title" className="label">
                {t("web.session.hints_title")}
              </h2>
              <ol>
                {s.hints.map((h) => (
                  <li key={h.tier} className="hint">
                    <span className="hint-tier">{t("web.session.hint_n", { tier: h.tier })}</span>
                    <p>{h.text}</p>
                  </li>
                ))}
              </ol>
            </section>
          )}

          <details className="activity">
            <summary>
              <Icon name="chevronRight" size={14} className="why-chevron" />
              {t("web.session.activity")}
            </summary>
            <ul className="timeline">
              {s.events.map((e, i) => (
                <li key={i}>
                  <span>{t(`web.event.${e.type}`, { tier: e.tier ?? "" })}</span>
                  <Time iso={e.at} />
                </li>
              ))}
            </ul>
          </details>
        </article>

        <section className="workbench" aria-label={t("web.session.workbench")}>
          {isActive ? (
            <>
              <div className="checkbar" role="toolbar" aria-label={t("web.session.toolbar")}>
                <Button variant="primary" icon="check" busy={busy === "check"} disabled={disabled} kbd="Ctrl Enter" onClick={() => run("check")}>
                  {busy === "check" ? t("web.session.checking") : t("web.session.run_check")}
                </Button>
                {hasNextHint && (
                  <Button icon="hint" busy={busy === "hint"} disabled={disabled} onClick={() => run("hint")}>
                    {t("web.session.show_hint", { tier: nextTier })}
                    <span className="btn-note">{tn("web.session.hint_cost", starsAfter(nextTier))}</span>
                  </Button>
                )}
                <ConfirmButton icon="refresh" busy={busy === "reset"} disabled={disabled} confirmLabel={t("web.session.reset_confirm_short")} onConfirm={() => run("reset")}>
                  {t("web.session.reset")}
                </ConfirmButton>
                <span className="checkbar-spacer" />
                <Timer startedAt={s.started_at} on={prefs.timer} onToggle={() => setPrefs({ timer: !prefs.timer })} />
                <MoreMenu canGiveUp={s.can_give_up} disabled={disabled} onGiveUp={() => run("giveup")} />
              </div>
              <CheckLine check={lastCheck} running={busy === "check"} />
              {offline && <p className="note is-fail">{t("web.session.offline")}</p>}
              {s.terminal ? (
                <Suspense fallback={<TerminalPlaceholder />}>
                  <WebTerminal sessionId={s.id} />
                </Suspense>
              ) : (
                <ShellCommand command={s.shell_command} />
              )}
              <CheckLog checks={s.checks} />
            </>
          ) : (
            <Result s={s} animate={solvedHere} />
          )}
        </section>
      </div>
    </div>
  );
}

// TerminalPlaceholder holds the terminal's place, at its size and with its
// status line, while its code loads.
function TerminalPlaceholder() {
  const { t } = useI18n();
  return (
    <div className="web-terminal">
      <div className="terminal-placeholder" />
      <p className="terminal-status">{t("web.terminal.connecting")}</p>
    </div>
  );
}

// starsAfter is the most stars left after revealing hint tier (spec 6).
function starsAfter(tier: number): number {
  return tier >= 3 ? 1 : 2;
}

function SessionSkeleton() {
  const { t } = useI18n();
  return (
    <div className="page page-session" role="status" aria-label={t("web.loading")}>
      <div className="session-grid">
        <div className="ticket skeleton" aria-hidden="true">
          <span className="skeleton-bar w-40" />
          <span className="skeleton-bar h-title w-80" />
          <span className="skeleton-bar w-60" />
          <span className="skeleton-bar w-100" />
          <span className="skeleton-bar w-90" />
          <span className="skeleton-bar w-70" />
        </div>
        <div className="workbench skeleton" aria-hidden="true">
          <span className="skeleton-bar h-bar w-100" />
          <span className="skeleton-block" />
        </div>
      </div>
    </div>
  );
}

// Timer shows the time since the session started, when the learner turns it
// on. It is never scored.
function Timer({ startedAt, on, onToggle }: { startedAt: string; on: boolean; onToggle: () => void }) {
  const { t } = useI18n();
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    if (!on) return;
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [on]);
  const secs = Math.max(0, Math.floor((now - new Date(startedAt).getTime()) / 1000));
  const text = `${Math.floor(secs / 60)}:${String(secs % 60).padStart(2, "0")}`;
  return (
    <button type="button" className={on ? "timer is-on" : "timer"} aria-pressed={on} onClick={onToggle} title={on ? t("web.session.timer_hide") : t("web.session.timer_show")}>
      <Icon name="timer" size={16} />
      {on ? <span className="timer-value">{text}</span> : <span className="sr-only">{t("web.session.timer_show")}</span>}
    </button>
  );
}

// MoreMenu holds the rare actions, apart from the check: give up.
function MoreMenu({ canGiveUp, disabled, onGiveUp }: { canGiveUp: boolean; disabled: boolean; onGiveUp: () => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const away = (e: Event) => {
      if (!box.current?.contains(e.target as Node)) setOpen(false);
    };
    const esc = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("pointerdown", away);
    document.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("pointerdown", away);
      document.removeEventListener("keydown", esc);
    };
  }, [open]);
  return (
    <div className="menu" ref={box}>
      <button type="button" className="icon-btn" aria-haspopup="true" aria-expanded={open} aria-label={t("web.session.more")} title={t("web.session.more")} onClick={() => setOpen(!open)}>
        <Icon name="more" />
      </button>
      {open && (
        <div className="menu-pop">
          <ConfirmButton variant="quiet" icon="flag" disabled={disabled || !canGiveUp} confirmLabel={t("web.session.giveup_confirm_short")} onConfirm={onGiveUp}>
            {t("web.session.giveup")}
          </ConfirmButton>
          {!canGiveUp && <p className="note">{t("giveup.locked")}</p>}
        </div>
      )}
    </div>
  );
}

// CheckLine is the result of the last check run here, in one line under the
// check bar. It changes in place, with an icon that swaps.
function CheckLine({ check, running }: { check: Session["check"]; running: boolean }) {
  const { t } = useI18n();
  const shown = useDelayed(running);
  if (running && shown) {
    return (
      <p className="checkline">
        <span className="spinner" aria-hidden="true" /> {t("web.session.checking")}
      </p>
    );
  }
  if (!check || check.result === "pass") return <p className="checkline is-empty" aria-hidden="true" />;
  const infra = check.result === "infra";
  return (
    <p className={infra ? "checkline is-infra" : "checkline is-fail"}>
      <IconSwap on={infra} a={<Icon name="close" size={16} />} b={<Icon name="flag" size={16} />} />
      {infra ? t("check.infra") : check.feedback ? t("check.fail", { feedback: check.feedback }) : t("check.fail_generic")}
    </p>
  );
}

function CheckLog({ checks }: { checks: Session["checks"] }) {
  const { t, tn } = useI18n();
  if (checks.length === 0) return <p className="note">{t("web.session.checks_none")}</p>;
  return (
    <details className="checklog">
      <summary>
        <Icon name="chevronRight" size={14} className="why-chevron" />
        {tn("web.session.check_log", checks.length)}
      </summary>
      <ul>
        {checks.map((c, i) => (
          <li key={i} className={c.passed ? "is-pass" : "is-fail"}>
            <Icon name={c.passed ? "check" : "close"} size={14} />
            <span>
              <b>{c.passed ? t("web.session.check_passed") : t("web.session.check_failed")}</b>
              {c.feedback && <span className="muted"> {c.feedback}</span>}
            </span>
            <Time iso={c.at} />
          </li>
        ))}
      </ul>
    </details>
  );
}

function ShellCommand({ command }: { command: string }) {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);
  const copy = () => {
    navigator.clipboard?.writeText(command).then(
      () => {
        setCopied(true);
        window.setTimeout(() => setCopied(false), 1500);
      },
      () => setCopied(false),
    );
  };
  return (
    <div className="shell-panel">
      <p>{t("web.session.shell_body")}</p>
      <div className="command">
        <code>
          <span className="prompt" aria-hidden="true">
            $
          </span>{" "}
          {command}
        </code>
        <button type="button" className="icon-btn command-copy" onClick={copy} aria-label={t("web.session.copy")}>
          <IconSwap on={copied} a={<Icon name="catalog" size={16} />} b={<Icon name="check" size={16} />} />
          <span className="command-copy-text" aria-live="polite">
            {copied ? t("web.session.copied") : t("web.session.copy")}
          </span>
        </button>
      </div>
    </div>
  );
}

// Result ends a session: the outcome, the stars, any level change, the
// walkthrough, and one next action. A solve made on this page plays the one
// expressive animation of the app; any key or click skips it.
function Result({ s, animate }: { s: Session; animate: boolean }) {
  const { t } = useI18n();
  const [playing, setPlaying] = useState(animate);
  useEffect(() => {
    if (!playing) return;
    const stop = () => setPlaying(false);
    const id = window.setTimeout(stop, 1300);
    window.addEventListener("keydown", stop, { once: true });
    window.addEventListener("pointerdown", stop, { once: true });
    return () => {
      window.clearTimeout(id);
      window.removeEventListener("keydown", stop);
      window.removeEventListener("pointerdown", stop);
    };
  }, [playing]);
  const lc = s.level_change;
  const solved = s.status === "solved";
  return (
    <div className={`result result-${s.status}${playing ? " is-playing" : ""}`}>
      <header className="result-head">
        {solved && (
          <svg className="solve-mark" viewBox="0 0 48 48" aria-hidden="true">
            <circle cx="24" cy="24" r="21" className="solve-ring" />
            <path d="M14 25l7 7 13-15" className="solve-check" />
          </svg>
        )}
        <div>
          <p className="label">{t(`web.event.${s.status}`)}</p>
          <h2>{solved ? t(praiseKey(s)) : t(`web.result.${s.status}`)}</h2>
        </div>
        {solved && (
          <span className="result-stars">
            <Stars n={s.stars} size={22} />
          </span>
        )}
      </header>
      {lc && (
        <p className={lc.to > lc.from ? "level-change is-up" : "level-change is-down"}>
          {t(lc.to > lc.from ? "level.up" : "level.down", { topic: t(`web.topic.${lc.topic}`), from: lc.from, to: lc.to })}
        </p>
      )}
      {s.walkthrough && (
        <section className="walkthrough" aria-labelledby="walkthrough-title">
          <h3 id="walkthrough-title">{t("web.result.walkthrough")}</h3>
          <Prose text={s.walkthrough} />
        </section>
      )}
      <a className="btn btn-primary" href="#/">
        <span className="btn-label">{t("web.result.next")}</span>
        <Icon name="arrowRight" />
      </a>
    </div>
  );
}

// praiseKey picks the summary line by how the session was solved: praise
// only for what was done.
function praiseKey(s: Session): string {
  if (s.stars === 3) return "web.solve.no_hints";
  if (s.stars === 2) return "web.solve.some_hints";
  return "web.solve.with_hints";
}
