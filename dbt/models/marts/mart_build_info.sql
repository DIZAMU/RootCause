-- One row: when the latest dbt build made the marts. The dashboard shows it,
-- so statistics that lag behind (the v2 CronJob builds every 10 minutes) are
-- visibly older than the latest session.
-- depends_on: {{ ref('mart_topic_levels') }}
-- depends_on: {{ ref('mart_review_queue') }}
-- depends_on: {{ ref('mart_exercise_stats') }}
-- depends_on: {{ ref('mart_progress') }}

select now() as built_at
