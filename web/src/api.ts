// Client for the rootcause serve API (internal/server). Every request carries
// the per-process token that the server wrote into index.html.

const token =
  document.querySelector<HTMLMetaElement>('meta[name="rootcause-token"]')?.content ?? "";

export type Lang = "en" | "no";
export type Strings = Record<string, string>;

export interface Overview {
  lang: Lang;
  active: { id: string; exercise: string } | null;
  next: {
    exercise: string;
    topic: string;
    difficulty: number;
    reason: "review" | "weakest_topic";
    level: number;
  } | null;
  source: "marts" | "fallback" | "";
  levels: { topic: string; level: number }[];
  review: { exercise: string; due_at: string; due_now: boolean }[] | null;
  stats: { fresh: boolean; updating: boolean; error: string; built_at: string | null };
}

export interface ExerciseRow {
  id: string;
  topic: string;
  difficulty: number;
  best_stars: number | null;
}

export interface HistoryRow {
  session: string;
  exercise: string;
  status: SessionStatus;
  stars: number | null;
  hints: number;
  checks: number;
  started_at: string;
  ended_at: string | null;
}

export type SessionStatus = "active" | "solved" | "gave_up" | "expired" | "voided";

export interface Session {
  id: string;
  exercise: string;
  topic: string;
  difficulty: number;
  status: SessionStatus;
  stars: number | null;
  started_at: string;
  ended_at: string | null;
  expires_at: string;
  ticket: { title: string; from: string; body: string };
  hints: { tier: number; text: string }[];
  hints_total: number;
  can_give_up: boolean;
  checks: { at: string; passed: boolean; feedback: string }[];
  events: { type: string; at: string; tier?: number }[];
  level_change: { topic: string; from: number; to: number } | null;
  walkthrough: string;
  shell_command: string;
  terminal: boolean;
  check?: { result: "pass" | "fail" | "infra"; feedback: string; stars: number };
}

// ApiError carries the server's message, already in the learner's language.
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}

// openTerminal opens the web terminal's WebSocket. A browser cannot set
// headers on a WebSocket, so the token travels as a subprotocol; the server
// answers with rootcause.terminal only and never echoes the token.
export function openTerminal(): WebSocket {
  const url = new URL("/api/terminal", window.location.href);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return new WebSocket(url, ["rootcause.terminal", "rootcause.token." + token]);
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { "X-RootCause-Token": token };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: "no-store",
  });
  if (res.status === 204) return undefined as T;
  const data: unknown = await res.json().catch(() => null);
  if (!res.ok) {
    const msg =
      data && typeof data === "object" && "error" in data ? String(data.error) : res.statusText;
    throw new ApiError(msg, res.status);
  }
  return data as T;
}

export const api = {
  overview: () => request<Overview>("GET", "/api/overview"),
  exercises: () => request<ExerciseRow[]>("GET", "/api/exercises"),
  history: () => request<HistoryRow[]>("GET", "/api/history"),
  active: () => request<Session | undefined>("GET", "/api/session"),
  session: (id: string) => request<Session>("GET", `/api/sessions/${encodeURIComponent(id)}`),
  strings: (lang: Lang) => request<Strings>("GET", `/api/i18n/${lang}`),
  setLang: (lang: Lang) => request<void>("PUT", "/api/lang", { lang }),
  start: (exercise: string) => request<Session>("POST", "/api/start", { exercise }),
  hint: (session: string) => request<Session>("POST", "/api/hint", { session }),
  check: (session: string) => request<Session>("POST", "/api/check", { session }),
  reset: (session: string) => request<Session>("POST", "/api/reset", { session }),
  giveUp: (session: string) => request<Session>("POST", "/api/giveup", { session }),
};
