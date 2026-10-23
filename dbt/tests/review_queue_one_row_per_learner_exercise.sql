-- An exercise is in a learner's review queue at most once.
select learner_id, exercise_id, count(*) as n
from {{ ref('mart_review_queue') }}
group by learner_id, exercise_id
having count(*) > 1
