-- One row per event, typed and renamed.
select
    id   as event_id,
    occurred_at,
    session_id,
    type as event_type,
    payload
from {{ source('engine', 'event') }}
