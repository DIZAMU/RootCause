-- One row per finished session: its outcome, stars, hints, checks and
-- duration.
--
-- outcome: solved | gave_up | expired_fail (expired after at least one check)
--          | expired_skip (expired with no check) | voided
-- kind is the outcome as the level rule classifies it (internal/grading):
--          strong (solved, 2+ stars) | weak (solved, 1 star) | fail | skip
-- counted is false for skip: those sessions are left out of the stats and
-- the review queue.

with sessions as (
    select * from {{ ref('stg_sessions') }}
    where status <> 'active'
),

checks as (
    select session_id, count(*) as checks
    from {{ ref('stg_events') }}
    where event_type = 'check_run'
    group by session_id
),

outcomes as (
    select
        s.session_id,
        s.learner_id,
        s.exercise_id,
        x.topic,
        s.difficulty,
        s.seed,
        s.variant_values,
        s.stars,
        s.hints_revealed                       as hints,
        coalesce(c.checks, 0)                  as checks,
        s.started_at,
        s.ended_at,
        extract(epoch from s.ended_at - s.started_at)::int as duration_seconds,
        case
            when s.status = 'solved' then 'solved'
            when s.status = 'gave_up' then 'gave_up'
            when s.status = 'expired' and coalesce(c.checks, 0) > 0 then 'expired_fail'
            when s.status = 'expired' then 'expired_skip'
            else 'voided'
        end as outcome
    from sessions s
    join {{ ref('stg_exercises') }} x on x.exercise_id = s.exercise_id
    left join checks c on c.session_id = s.session_id
)

select
    *,
    case
        when outcome = 'solved' and stars >= 2 then 'strong'
        when outcome = 'solved' then 'weak'
        when outcome in ('gave_up', 'expired_fail') then 'fail'
        else 'skip'
    end as kind,
    outcome in ('solved', 'gave_up', 'expired_fail') as counted
from outcomes
