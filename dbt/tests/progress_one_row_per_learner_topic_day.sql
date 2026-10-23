-- mart_progress has one row per learner, topic and day.
select learner_id, topic, day, count(*) as n
from {{ ref('mart_progress') }}
group by learner_id, topic, day
having count(*) > 1
