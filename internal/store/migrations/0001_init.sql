-- 0001_init: source tables written by the engine (spec section 9.1).

CREATE TABLE learner (
    id         INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT NOT NULL,
    lang       TEXT NOT NULL DEFAULT 'en' CHECK (lang IN ('en', 'no')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE exercise (
    id         TEXT PRIMARY KEY,
    version    INT  NOT NULL,
    topic      TEXT NOT NULL CHECK (topic IN ('permissions', 'services', 'logs', 'networking', 'disk')),
    difficulty INT  NOT NULL CHECK (difficulty BETWEEN 1 AND 3),
    synced_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE session (
    id               TEXT PRIMARY KEY,
    learner_id       INT  NOT NULL REFERENCES learner (id),
    exercise_id      TEXT NOT NULL REFERENCES exercise (id),
    exercise_version INT  NOT NULL,
    seed             BIGINT NOT NULL,
    variant_values   JSONB NOT NULL DEFAULT '{}',
    derived_values   JSONB NOT NULL DEFAULT '{}',
    status           TEXT NOT NULL CHECK (status IN ('active', 'solved', 'gave_up', 'expired', 'voided')),
    sandbox_id       TEXT,
    hints_revealed   INT NOT NULL DEFAULT 0,
    stars            INT CHECK (stars BETWEEN 0 AND 3),
    started_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at         TIMESTAMPTZ,
    expires_at       TIMESTAMPTZ NOT NULL
);

CREATE TABLE event (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    session_id  TEXT NOT NULL REFERENCES session (id),
    type        TEXT NOT NULL CHECK (type IN (
        'session_started', 'hint_revealed', 'check_run', 'reset',
        'gave_up', 'solved', 'expired', 'voided', 'level_changed'
    )),
    payload     JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_event_session   ON event (session_id);
CREATE INDEX idx_event_type      ON event (type);
CREATE INDEX idx_session_status  ON session (status);
CREATE INDEX idx_session_exercise ON session (exercise_id);
