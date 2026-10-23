-- The current level per learner and topic (spec section 7.1): the "to" value
-- of the topic's latest level_changed event, or 1 if it never changed. Also
-- when the topic was last practiced (any finished session that was not
-- voided), for the selector's tie-break.

with learners as (
    select distinct learner_id from {{ ref('stg_sessions') }}
),

topics as (
    select distinct topic from {{ ref('stg_exercises') }}
),

changes as (
    select
        s.learner_id,
        e.payload ->> 'topic'        as topic,
        (e.payload ->> 'to')::int    as level,
        row_number() over (
            partition by s.learner_id, e.payload ->> 'topic'
            order by e.occurred_at desc, e.event_id desc
        ) as recency
    from {{ ref('stg_events') }} e
    join {{ ref('stg_sessions') }} s on s.session_id = e.session_id
    where e.event_type = 'level_changed'
),

practiced as (
    select learner_id, topic, max(ended_at) as last_practiced_at
    from {{ ref('int_session_outcomes') }}
    where outcome <> 'voided'
    group by learner_id, topic
)

select
    l.learner_id,
    t.topic,
    coalesce(c.level, 1) as level,
    p.last_practiced_at
from learners l
cross join topics t
left join changes c
    on c.learner_id = l.learner_id and c.topic = t.topic and c.recency = 1
left join practiced p
    on p.learner_id = l.learner_id and p.topic = t.topic
