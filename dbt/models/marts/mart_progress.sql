-- Per learner, topic and day (UTC): finished sessions that were not voided,
-- solves, average stars of the solves, and the topic level at the end of that
-- day. For the dashboard's history view.

with days as (
    select
        learner_id,
        topic,
        (ended_at at time zone 'UTC')::date               as day,
        count(*)                                          as sessions,
        count(*) filter (where outcome = 'solved')        as solves,
        avg(stars) filter (where outcome = 'solved')      as avg_stars
    from {{ ref('int_session_outcomes') }}
    where outcome <> 'voided'
    group by learner_id, topic, (ended_at at time zone 'UTC')::date
),

changes as (
    select
        s.learner_id,
        e.payload ->> 'topic'     as topic,
        (e.payload ->> 'to')::int as level,
        e.occurred_at,
        e.event_id
    from {{ ref('stg_events') }} e
    join {{ ref('stg_sessions') }} s on s.session_id = e.session_id
    where e.event_type = 'level_changed'
)

select
    d.*,
    coalesce((
        select c.level
        from changes c
        where c.learner_id = d.learner_id
          and c.topic = d.topic
          and c.occurred_at < (d.day + 1)::timestamp at time zone 'UTC'
        order by c.occurred_at desc, c.event_id desc
        limit 1
    ), 1) as level_end_of_day
from days d
