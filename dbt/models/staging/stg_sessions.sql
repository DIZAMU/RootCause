-- One row per session, typed and renamed.
select
    id                  as session_id,
    learner_id,
    exercise_id,
    exercise_version,
    exercise_difficulty as difficulty,
    seed,
    variant_values,
    status,
    hints_revealed,
    stars,
    started_at,
    ended_at,
    expires_at
from {{ source('engine', 'session') }}
