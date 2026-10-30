-- Exercises that come back for review (ARCHITECTURE.md section 7.2, rule 1): per learner
-- and exercise, the latest counted session was a fail or a 1-star solve. It is
-- due 72 hours after that session ended; the selector compares due_at with
-- the current time, so a queue built earlier is still correct later. A later
-- strong solve replaces the old outcome, so the exercise leaves the queue.

with latest as (
    select
        *,
        row_number() over (
            partition by learner_id, exercise_id
            order by ended_at desc, session_id desc
        ) as recency
    from {{ ref('int_session_outcomes') }}
    where counted
)

select
    learner_id,
    exercise_id,
    topic,
    session_id     as last_session_id,
    seed           as last_seed,
    variant_values as last_variant_values,
    outcome        as last_outcome,
    ended_at       as last_ended_at,
    ended_at + interval '72 hours' as due_at
from latest
where recency = 1
  and kind in ('fail', 'weak')
