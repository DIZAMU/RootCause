-- Every learner with a session has exactly one level row per topic. Returns
-- the offending learner and topic pairs.
with expected as (
    select l.learner_id, t.topic
    from (select distinct learner_id from {{ ref('stg_sessions') }}) l
    cross join (select distinct topic from {{ ref('stg_exercises') }}) t
),

actual as (
    select learner_id, topic, count(*) as n
    from {{ ref('mart_topic_levels') }}
    group by learner_id, topic
)

select e.learner_id, e.topic, coalesce(a.n, 0) as n
from expected e
left join actual a on a.learner_id = e.learner_id and a.topic = e.topic
where coalesce(a.n, 0) <> 1
