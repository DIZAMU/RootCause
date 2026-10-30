import { useEffect, useRef, useState } from "react";
import { useI18n } from "../i18n";
import { Icon } from "../ui/icons";
import { Dialog } from "./Dialog";

const steps = ["inbox", "session", "hints", "levels"] as const;
type Step = (typeof steps)[number];

// Intro is the guided first-run tour: four steps in a modal dialog, with
// Back, Next and Skip. It opens on a browser's first visit (no stored
// preferences) and again from the help dialog. Esc, Skip and the last step's
// button all end it, and end calls onDone. Without a web terminal (v1) the
// session step points to rootcause shell instead.
export function Intro({ open, terminal, onDone }: { open: boolean; terminal: boolean; onDone: () => void }) {
  const { t } = useI18n();
  const [index, setIndex] = useState(0);
  const next = useRef<HTMLButtonElement>(null);
  // Dialog (a child, so its effect runs first) has opened the dialog by now;
  // start on Next rather than on Skip, the first button.
  useEffect(() => {
    if (!open) return;
    setIndex(0);
    next.current?.focus();
  }, [open]);
  // After Back or Next, focus is on the primary button of the new step (the
  // Back button is gone on the first step).
  const moved = useRef(false);
  useEffect(() => {
    if (moved.current) next.current?.focus();
    moved.current = false;
  }, [index]);
  const step = steps[index] ?? "inbox";
  const text = step === "session" && !terminal ? "session_shell" : step;
  const last = index === steps.length - 1;
  const go = (i: number) => {
    moved.current = true;
    setIndex(i);
  };
  return (
    <Dialog open={open} onClose={onDone} labelledBy="intro-title" className="dialog-intro">
      <div className="dialog-body">
        <div className="intro-art" key={step} aria-hidden="true">
          <Art step={step} />
        </div>
        <div className="intro-text" key={`text-${step}`}>
          <p className="label" aria-live="polite">
            {t("web.intro.step", { n: index + 1, total: steps.length })}
          </p>
          <h2 id="intro-title">{t(`web.intro.${text}.title`)}</h2>
          <p>{t(`web.intro.${text}.body`)}</p>
        </div>
        <div className="intro-dots" aria-hidden="true">
          {steps.map((s, i) => (
            <span key={s} className={i === index ? "intro-dot is-on" : "intro-dot"} />
          ))}
        </div>
        <footer className="dialog-actions">
          <form method="dialog">
            <button className="btn btn-quiet intro-skip">{t("web.intro.skip")}</button>
          </form>
          <span className="dialog-actions-spacer" />
          {index > 0 && (
            <button type="button" className="btn btn-secondary intro-back" onClick={() => go(index - 1)}>
              <Icon name="arrowLeft" />
              <span className="btn-label">{t("web.intro.back")}</span>
            </button>
          )}
          {last ? (
            <form method="dialog">
              <button ref={next} className="btn btn-primary intro-next">
                <span className="btn-label">{t("web.intro.done")}</span>
              </button>
            </form>
          ) : (
            <button ref={next} type="button" className="btn btn-primary intro-next" onClick={() => go(index + 1)}>
              <span className="btn-label">{t("web.intro.next")}</span>
              <Icon name="arrowRight" />
            </button>
          )}
        </footer>
      </div>
    </Dialog>
  );
}

// Art is each step's small drawing of the screen it is about, in the theme's
// colours.
function Art({ step }: { step: Step }) {
  switch (step) {
    case "inbox":
      return (
        <svg viewBox="0 0 240 120" className="art">
          <rect x="20" y="14" width="200" height="28" rx="3" className="art-hi" />
          <circle cx="34" cy="28" r="4" className="art-accent" />
          <rect x="46" y="22" width="96" height="6" rx="2" className="art-ink" />
          <rect x="46" y="32" width="60" height="4" rx="2" className="art-soft" />
          <rect x="172" y="21" width="38" height="14" rx="3" className="art-accent" />
          {[52, 82].map((y) => (
            <g key={y}>
              <rect x="20" y={y} width="200" height="24" rx="3" className="art-panel" />
              <circle cx="34" cy={y + 12} r="4" className="art-line" />
              <rect x="46" y={y + 7} width="80" height="5" rx="2" className="art-soft" />
              <rect x="46" y={y + 15} width="48" height="3" rx="1.5" className="art-soft" />
            </g>
          ))}
        </svg>
      );
    case "session":
      return (
        <svg viewBox="0 0 240 120" className="art">
          <rect x="16" y="12" width="84" height="96" rx="3" className="art-panel" />
          <rect x="26" y="24" width="56" height="6" rx="2" className="art-ink" />
          {[38, 46, 54, 62].map((y) => (
            <rect key={y} x="26" y={y} width={y === 62 ? 36 : 62} height="3" rx="1.5" className="art-soft" />
          ))}
          <rect x="108" y="12" width="116" height="96" rx="3" className="art-term" />
          <rect x="118" y="24" width="40" height="3" rx="1.5" className="art-accent" />
          <rect x="118" y="32" width="70" height="3" rx="1.5" className="art-term-ink" />
          <rect x="118" y="40" width="54" height="3" rx="1.5" className="art-term-ink" />
          <rect x="164" y="88" width="50" height="12" rx="3" className="art-accent" />
        </svg>
      );
    case "hints":
      return (
        <svg viewBox="0 0 240 120" className="art">
          {[0, 1, 2].map((i) => (
            <path
              key={i}
              d="M0 -14l4.1 8.4 9.2 1.3-6.6 6.5 1.6 9.2L0 7.1l-8.3 4.3 1.6-9.2-6.6-6.5 9.2-1.3z"
              transform={`translate(${84 + i * 36} 40)`}
              className={i < 2 ? "art-star" : "art-line"}
            />
          ))}
          {[0, 1, 2].map((i) => (
            <rect key={i} x={54 + i * 46} y="76" width="40" height="24" rx="3" className={i === 0 ? "art-hi" : "art-panel"} />
          ))}
          <rect x="66" y="86" width="16" height="4" rx="2" className="art-ink" />
        </svg>
      );
    default:
      return (
        <svg viewBox="0 0 240 120" className="art">
          {[66, 33, 100].map((fill, i) => (
            <g key={i} transform={`translate(${60 + i * 60} 46)`}>
              <circle r="20" className="art-ring" />
              <circle r="20" className="art-ring-fill" pathLength="100" strokeDasharray={`${fill} 100`} transform="rotate(-90)" />
            </g>
          ))}
          {[8, 14, 10, 20, 16, 24].map((h, i) => (
            <rect key={i} x={70 + i * 18} y={108 - h} width="10" height={h} rx="2" className="art-accent" />
          ))}
        </svg>
      );
  }
}
