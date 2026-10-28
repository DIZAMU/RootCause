// Shared building blocks of the dashboard. Colours, sizes and timings come
// from tokens.css; no component sets its own.
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type ReactNode,
} from "react";
import { useI18n } from "../i18n";
import { Icon, type IconName } from "./icons";

// Mark is the RootCause logo: a trunk splitting into roots, the middle one
// highlighted as the cause.
export function Mark({ size = 24 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true" className="mark" fill="none">
      <path d="M16 4v9.5" className="mark-trunk" />
      <path d="M16 13.5c0 4.5-6.5 4.8-8.6 11.2" className="mark-root" />
      <path d="M16 13.5c0 4.5 6.5 4.8 8.6 11.2" className="mark-root" />
      <path d="M16 13.5v9" className="mark-cause" />
      <circle cx="16" cy="26" r="2.4" className="mark-cause-dot" />
    </svg>
  );
}

type Variant = "primary" | "secondary" | "quiet";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  icon?: IconName;
  busy?: boolean;
  kbd?: string; // a shortcut shown inside the button
}

// Button is every clickable action. busy keeps the width and shows a spinner,
// so the layout does not move.
export function Button({ variant = "secondary", icon, busy, kbd, children, className, disabled, ...rest }: ButtonProps) {
  return (
    <button
      type="button"
      {...rest}
      disabled={disabled || busy}
      aria-busy={busy || undefined}
      className={["btn", `btn-${variant}`, busy ? "is-busy" : "", className ?? ""].join(" ").trim()}
    >
      {busy ? <span className="spinner" aria-hidden="true" /> : icon && <Icon name={icon} />}
      <span className="btn-label">{children}</span>
      {kbd && <kbd>{kbd}</kbd>}
    </button>
  );
}

// ConfirmButton asks for a second click before it acts: the first click
// turns its label into confirmLabel for a few seconds. No dialog.
export function ConfirmButton({
  confirmLabel,
  onConfirm,
  children,
  ...rest
}: Omit<ButtonProps, "onClick"> & { confirmLabel: string; onConfirm: () => void }) {
  const [armed, setArmed] = useState(false);
  useEffect(() => {
    if (!armed) return;
    const id = window.setTimeout(() => setArmed(false), 4000);
    return () => window.clearTimeout(id);
  }, [armed]);
  return (
    <Button
      {...rest}
      aria-live="polite"
      className={armed ? "is-armed" : undefined}
      onClick={() => {
        if (armed) {
          setArmed(false);
          onConfirm();
        } else setArmed(true);
      }}
      onBlur={() => setArmed(false)}
    >
      {armed ? confirmLabel : children}
    </Button>
  );
}

export function Stars({ n, size = 16 }: { n: number | null; size?: number }) {
  const { t } = useI18n();
  if (n === null) return <span className="muted">-</span>;
  return (
    <span className="stars" role="img" aria-label={t("web.stars", { stars: n })}>
      {[1, 2, 3].map((i) => (
        <svg key={i} viewBox="0 0 20 20" width={size} height={size} aria-hidden="true" className={i <= n ? "star on" : "star"}>
          <path d="M10 2.2l2.4 5 5.4.7-3.9 3.8.9 5.4L10 14.6l-4.8 2.5.9-5.4-3.9-3.8 5.4-.7z" />
        </svg>
      ))}
    </span>
  );
}

// Ring shows a topic level: the number in the middle, and an arc filled by
// the strong solves made toward the next level. A full ring is the top
// level. The meaning is in the text label too, never in the colour alone.
export function Ring({ level, streak, needed, size = 36, label }: { level: number; streak: number; needed: number; size?: number; label: string }) {
  const r = 15;
  const c = 2 * Math.PI * r;
  const fraction = needed === 0 ? 1 : streak / needed;
  return (
    <svg width={size} height={size} viewBox="0 0 36 36" role="img" aria-label={label} className="ring">
      <circle cx="18" cy="18" r={r} className="ring-track" />
      {fraction > 0 && (
        <circle cx="18" cy="18" r={r} className="ring-fill" strokeDasharray={`${c * fraction} ${c}`} transform="rotate(-90 18 18)" />
      )}
      <text x="18" y="18" className="ring-level" dominantBaseline="central" textAnchor="middle">
        {level}
      </text>
    </svg>
  );
}

// useLevelLabel words a topic level for the ring's label and its tooltip.
export function useLevelLabel(): (topic: string, level: number, streak: number, needed: number) => string {
  const { t, tn } = useI18n();
  return (topic, level, streak, needed) =>
    needed === 0
      ? t("web.level.top", { topic: t(`web.topic.${topic}`), level })
      : tn("web.level.progress", needed - streak, { topic: t(`web.topic.${topic}`), level, next: level + 1, streak, needed });
}

// useLevelNext words only the way to the next level, for a caption under a
// ring that already names its topic.
export function useLevelNext(): (level: number, streak: number, needed: number) => string {
  const { t, tn } = useI18n();
  return (level, streak, needed) =>
    needed === 0 ? t("web.level.next_top") : tn("web.level.next", needed - streak, { next: level + 1 });
}

// useDelayed is true once active has lasted the spinner delay, and then
// stays true for at least the minimum time, so a spinner never flashes.
export function useDelayed(active: boolean, delayMs = 300, minMs = 400): boolean {
  const [shown, setShown] = useState(false);
  const since = useRef(0);
  useEffect(() => {
    if (active && !shown) {
      const id = window.setTimeout(() => {
        since.current = Date.now();
        setShown(true);
      }, delayMs);
      return () => window.clearTimeout(id);
    }
    if (!active && shown) {
      const left = Math.max(0, minMs - (Date.now() - since.current));
      const id = window.setTimeout(() => setShown(false), left);
      return () => window.clearTimeout(id);
    }
  }, [active, shown, delayMs, minMs]);
  return shown;
}

// Skeleton holds the place of rows that are loading, at their real height,
// so nothing jumps when the data arrives.
export function Skeleton({ rows, className }: { rows: number; className?: string }) {
  const { t } = useI18n();
  return (
    <div className={`skeleton ${className ?? ""}`} role="status" aria-label={t("web.loading")}>
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="skeleton-row" aria-hidden="true">
          <span className="skeleton-bar" />
        </div>
      ))}
    </div>
  );
}

export function Banner({ tone, children, action }: { tone: "info" | "warn" | "fail" | "pass"; children: ReactNode; action?: ReactNode }) {
  return (
    <div className={`banner banner-${tone}`} role={tone === "fail" ? "alert" : "status"}>
      <div className="banner-text">{children}</div>
      {action}
    </div>
  );
}

export function EmptyState({ title, children, action }: { title: string; children: ReactNode; action?: ReactNode }) {
  return (
    <div className="empty">
      <h2 className="empty-title">{title}</h2>
      <p className="empty-body">{children}</p>
      {action}
    </div>
  );
}

// IconSwap shows one of two states in the same cell and cross-fades between
// them, for a result that changes in place.
export function IconSwap({ on, a, b, label }: { on: boolean; a: ReactNode; b: ReactNode; label?: string }) {
  return (
    <span className="iconswap" data-on={on ? "b" : "a"} aria-label={label} role={label ? "img" : undefined}>
      <span className="iconswap-a" aria-hidden="true">
        {a}
      </span>
      <span className="iconswap-b" aria-hidden="true">
        {b}
      </span>
    </span>
  );
}

// ProgressSteps lists the real steps of a longer task and ticks each one off
// as it is reported done. It never shows a percentage.
export function ProgressSteps({ steps, current }: { steps: string[]; current: number }) {
  return (
    <ol className="steps">
      {steps.map((label, i) => {
        const state = i < current ? "done" : i === current ? "running" : "waiting";
        return (
          <li key={i} className={`step step-${state}`} aria-current={state === "running" ? "step" : undefined}>
            <span className="step-mark" aria-hidden="true">
              {state === "done" ? <Icon name="check" size={14} /> : state === "running" ? <span className="spinner" /> : null}
            </span>
            <span>{label}</span>
          </li>
        );
      })}
    </ol>
  );
}

export function TopicLabel({ topic }: { topic: string }) {
  const { t } = useI18n();
  return <span className={`topic topic-${topic}`}>{t(`web.topic.${topic}`)}</span>;
}

// Difficulty is a row of three dots, the number spelled out for screen
// readers.
export function Difficulty({ n }: { n: number }) {
  const { t } = useI18n();
  return (
    <span className="difficulty" role="img" aria-label={t("web.difficulty", { difficulty: n })}>
      {[1, 2, 3].map((i) => (
        <span key={i} className={i <= n ? "dot on" : "dot"} aria-hidden="true" />
      ))}
    </span>
  );
}

// --- live region -------------------------------------------------------------

const AnnounceContext = createContext<(message: string) => void>(() => {});

// LiveRegionProvider keeps one polite live region for the whole app, so
// results that change in place (a check, a start) are read out once.
export function LiveRegionProvider({ children }: { children: ReactNode }) {
  const [message, setMessage] = useState("");
  const announce = useCallback((m: string) => {
    // Clear first, so the same message twice is read twice.
    setMessage("");
    window.setTimeout(() => setMessage(m), 50);
  }, []);
  return (
    <AnnounceContext.Provider value={announce}>
      {children}
      <div className="sr-only" role="status" aria-live="polite" aria-atomic="true">
        {message}
      </div>
    </AnnounceContext.Provider>
  );
}

export function useAnnounce(): (message: string) => void {
  return useContext(AnnounceContext);
}

// --- time ------------------------------------------------------------------

export function timeOf(iso: string, lang: string): string {
  return new Date(iso).toLocaleTimeString(lang === "no" ? "nb-NO" : "en-GB", { hour: "2-digit", minute: "2-digit" });
}

export function Time({ iso }: { iso: string }) {
  const { lang } = useI18n();
  return (
    <time className="time" dateTime={iso}>
      {timeOf(iso, lang)}
    </time>
  );
}

// useDuration formats a duration in seconds for tables and summaries.
export function useDuration(): (seconds: number) => string {
  const { t } = useI18n();
  return (seconds) => {
    const minutes = Math.max(0, Math.round(seconds / 60));
    if (minutes < 60) return t("web.duration.minutes", { minutes });
    return t("web.duration.hours", { hours: Math.floor(minutes / 60), minutes: minutes % 60 });
  };
}

// paragraphs splits a YAML block text into paragraphs at blank lines, and
// joins the hard-wrapped lines inside each one.
export function paragraphs(text: string): string[] {
  return text
    .split(/\n\s*\n/)
    .map((p) => p.replace(/\s*\n\s*/g, " ").trim())
    .filter((p) => p !== "");
}

export type TextBlock = { kind: "p"; text: string } | { kind: "code"; lines: string[] };

// textBlocks reads an exercise's block text: lines indented further than the
// text around them are commands (a code block); other lines are prose,
// reflowed into paragraphs at blank lines.
export function textBlocks(text: string): TextBlock[] {
  const out: TextBlock[] = [];
  let prose: string[] = [];
  let code: string[] = [];
  const flushProse = () => {
    const p = prose.join(" ").replace(/\s+/g, " ").trim();
    if (p) out.push({ kind: "p", text: p });
    prose = [];
  };
  const flushCode = () => {
    if (code.length) out.push({ kind: "code", lines: code });
    code = [];
  };
  for (const line of text.split("\n")) {
    if (/^\s{2,}\S/.test(line)) {
      flushProse();
      code.push(line.trim());
    } else if (line.trim() === "") {
      flushProse();
      flushCode();
    } else {
      flushCode();
      prose.push(line.trim());
    }
  }
  flushProse();
  flushCode();
  return out;
}

export function Prose({ text }: { text: string }) {
  return (
    <div className="prose">
      {textBlocks(text).map((b, i) =>
        b.kind === "p" ? (
          <p key={i}>{b.text}</p>
        ) : (
          // Focusable, so a command too wide for the screen can be scrolled
          // with the keyboard.
          <pre key={i} className="code" tabIndex={0}>
            <code>{b.lines.join("\n")}</code>
          </pre>
        ),
      )}
    </div>
  );
}
