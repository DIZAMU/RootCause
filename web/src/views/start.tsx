import { useState } from "react";
import { api, type Overview } from "../api";
import { sessionHref } from "../hooks";
import { useI18n } from "../i18n";
import { ProgressSteps, useAnnounce } from "../ui/kit";

// stepKeys name the engine's start steps (session.StartStep), in order.
const stepKeys = ["web.start.step.image", "web.start.step.machine", "web.start.step.setup", "web.start.step.fault"];

// useStart starts a session ("" = the recommended one) and opens it. While
// it runs, onStarting(true) makes the app poll the overview fast, so the
// start steps tick off.
export function useStart(reloadOverview: () => void, onStarting: (on: boolean) => void) {
  const announce = useAnnounce();
  const { t } = useI18n();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState("");
  const start = (exercise: string) => {
    setBusy(exercise);
    setError("");
    onStarting(true);
    announce(t("web.start.announce"));
    api.start(exercise).then(
      (s) => {
        onStarting(false);
        reloadOverview();
        window.location.hash = sessionHref(s.id);
      },
      (e: unknown) => {
        onStarting(false);
        setBusy(null);
        setError(e instanceof Error ? e.message : String(e));
        reloadOverview();
      },
    );
  };
  return { start, busy, error };
}

// StartProgress shows the real steps of a start while it runs. Before the
// first report it shows the first step as running.
export function StartProgress({ starting }: { starting: Overview["starting"] }) {
  const { t } = useI18n();
  return (
    <div className="start-progress">
      <p className="label">{t("web.start.title")}</p>
      <ProgressSteps steps={stepKeys.map((k) => t(k))} current={starting?.step ?? 0} />
    </div>
  );
}
