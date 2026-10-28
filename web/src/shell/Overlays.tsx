import { useEffect, useMemo, useRef, useState } from "react";
import { api, type ExerciseRow, type Overview } from "../api";
import { sessionHref } from "../hooks";
import { useI18n } from "../i18n";
import { Icon, type IconName } from "../ui/icons";
import { Dialog } from "./Dialog";

// ShortcutsDialog lists the keyboard shortcuts and switches the single-key
// ones on or off (WCAG 2.1.4).
export function ShortcutsDialog({ open, onClose, singleKeys, onSingleKeys }: { open: boolean; onClose: () => void; singleKeys: boolean; onSingleKeys: (on: boolean) => void }) {
  const { t } = useI18n();
  const rows: [string, string][] = [
    ["Ctrl K", "web.shortcuts.palette"],
    ["Ctrl Enter", "web.shortcuts.check"],
    ["g i", "web.shortcuts.go_inbox"],
    ["g e", "web.shortcuts.go_exercises"],
    ["g h", "web.shortcuts.go_history"],
    ["g s", "web.shortcuts.go_stats"],
    ["[", "web.shortcuts.sidebar"],
    ["?", "web.shortcuts.help"],
  ];
  return (
    <Dialog open={open} onClose={onClose} labelledBy="shortcuts-title" className="dialog-shortcuts">
      <div className="dialog-body">
        <header className="dialog-head">
          <h2 id="shortcuts-title">{t("web.shortcuts.title")}</h2>
          <form method="dialog">
            <button className="icon-btn" aria-label={t("web.close")}>
              <Icon name="close" />
            </button>
          </form>
        </header>
        <table className="kbd-table">
          <tbody>
            {rows.map(([keys, label]) => (
              <tr key={keys} className={keys.length <= 3 && !singleKeys ? "is-off" : undefined}>
                <td>
                  {keys.split(" ").map((k) => (
                    <kbd key={k}>{k}</kbd>
                  ))}
                </td>
                <td>{t(label)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <label className="switch">
          <input type="checkbox" checked={singleKeys} onChange={(e) => onSingleKeys(e.target.checked)} />
          <span>{t("web.shortcuts.single_keys")}</span>
        </label>
        <p className="note">{t("web.shortcuts.note")}</p>
      </div>
    </Dialog>
  );
}

interface Item {
  id: string;
  icon: IconName;
  label: string;
  hint?: string;
  run: () => void;
}

// Palette is the Ctrl+K command palette: a combobox over a list of pages,
// exercises and actions, used with the arrow keys and Enter.
export function Palette({
  open,
  onClose,
  overview,
  actions,
}: {
  open: boolean;
  onClose: () => void;
  overview: Overview | undefined;
  actions: Item[];
}) {
  const { t } = useI18n();
  const [query, setQuery] = useState("");
  const [index, setIndex] = useState(0);
  const [exercises, setExercises] = useState<ExerciseRow[]>([]);
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!open) return;
    setQuery("");
    setIndex(0);
    api.exercises().then(setExercises, () => setExercises([]));
    input.current?.focus();
  }, [open]);

  const items = useMemo<Item[]>(() => {
    const go = (hash: string) => () => {
      window.location.hash = hash;
    };
    const all: Item[] = [
      { id: "go-inbox", icon: "inbox", label: t("web.nav.inbox"), hint: "g i", run: go("#/") },
      { id: "go-exercises", icon: "catalog", label: t("web.nav.exercises"), hint: "g e", run: go("#/exercises") },
      { id: "go-history", icon: "history", label: t("web.nav.history"), hint: "g h", run: go("#/history") },
      { id: "go-stats", icon: "stats", label: t("web.nav.stats"), hint: "g s", run: go("#/stats") },
    ];
    if (overview?.active) {
      all.push({ id: "go-session", icon: "terminal", label: t("web.palette.open_session"), hint: overview.active.exercise, run: go(sessionHref(overview.active.id)) });
    }
    all.push(...actions);
    for (const ex of exercises) {
      all.push({ id: `ex-${ex.id}`, icon: "flag", label: ex.title, hint: ex.id, run: go(`#/exercises/${encodeURIComponent(ex.id)}`) });
    }
    const q = query.trim().toLowerCase();
    return q ? all.filter((i) => `${i.label} ${i.hint ?? ""}`.toLowerCase().includes(q)) : all;
  }, [t, overview, actions, exercises, query]);

  const current = items[Math.min(index, items.length - 1)];
  const choose = (item: Item | undefined) => {
    if (!item) return;
    onClose();
    item.run();
  };

  return (
    <Dialog open={open} onClose={onClose} labelledBy="palette-label" className="dialog-palette">
      <div className="dialog-body">
        <label id="palette-label" htmlFor="palette-input" className="sr-only">
          {t("web.palette.label")}
        </label>
        <div className="palette-search">
          <Icon name="search" />
          <input
            ref={input}
            id="palette-input"
            role="combobox"
            aria-expanded="true"
            aria-controls="palette-list"
            aria-activedescendant={current ? `palette-${current.id}` : undefined}
            aria-autocomplete="list"
            autoComplete="off"
            spellCheck={false}
            placeholder={t("web.palette.placeholder")}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setIndex(0);
            }}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown") {
                e.preventDefault();
                setIndex((i) => Math.min(i + 1, items.length - 1));
              } else if (e.key === "ArrowUp") {
                e.preventDefault();
                setIndex((i) => Math.max(i - 1, 0));
              } else if (e.key === "Enter") {
                e.preventDefault();
                choose(current);
              }
            }}
          />
        </div>
        <ul id="palette-list" role="listbox" className="palette-list" aria-label={t("web.palette.label")}>
          {items.map((item, i) => (
            <li
              key={item.id}
              id={`palette-${item.id}`}
              role="option"
              aria-selected={item === current}
              className={item === current ? "palette-item is-active" : "palette-item"}
              onMouseMove={() => setIndex(i)}
              onClick={() => choose(item)}
            >
              <Icon name={item.icon} />
              <span className="palette-label">{item.label}</span>
              {item.hint && <span className="palette-hint mono">{item.hint}</span>}
            </li>
          ))}
          {items.length === 0 && <li className="palette-empty">{t("web.palette.none")}</li>}
        </ul>
      </div>
    </Dialog>
  );
}

export type PaletteItem = Item;
