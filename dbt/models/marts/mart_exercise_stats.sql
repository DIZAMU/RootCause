-- Per exercise: solve rate, hint use and duration over all counted sessions,
-- and the calibration flag (ARCHITECTURE.md section 7.3). The flag is set only once an
-- exercise has at least 10 counted sessions: too_hard when difficulty 1 is
-- solved under 50% of the time, too_easy when difficulty 3 is solved over
-- 90% of the time.

with counted as (
    select * from {{ ref('int_session_outcomes') }}
    where counted
),

stats as (
    select
        exercise_id,
        count(*)                                            as sessions,
        count(*) filter (where outcome = 'solved')          as solves,
        avg(hints)                                          as avg_hints,
        avg(duration_seconds) filter (where outcome = 'solved') as avg_solve_seconds
    from counted
    group by exercise_id
)

select
    x.exercise_id,
    x.topic,
    x.difficulty,
    coalesce(s.sessions, 0)                              as sessions,
    coalesce(s.solves, 0)                                as solves,
    s.solves::numeric / nullif(s.sessions, 0)            as solve_rate,
    s.avg_hints,
    s.avg_solve_seconds,
    case
        when coalesce(s.sessions, 0) < 10 then null
        when x.difficulty = 1 and s.solves::numeric / s.sessions < 0.5 then 'too_hard'
        when x.difficulty = 3 and s.solves::numeric / s.sessions > 0.9 then 'too_easy'
    end as calibration_flag
from {{ ref('stg_exercises') }} x
left join stats s on s.exercise_id = x.exercise_id
