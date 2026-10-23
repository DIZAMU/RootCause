-- One row per exercise in the catalog, with its current difficulty.
select
    id as exercise_id,
    version,
    topic,
    difficulty
from {{ source('engine', 'exercise') }}
