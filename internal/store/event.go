package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Event is one logged action in a session (ARCHITECTURE.md section 9.1).
type Event struct {
	ID         int64
	OccurredAt time.Time
	SessionID  string
	Type       string
	Payload    map[string]any
}

// AddEvent appends an event. payload may be nil.
func (s *Store) AddEvent(ctx context.Context, sessionID, eventType string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	j, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO event (session_id, type, payload) VALUES ($1, $2, $3)`,
		sessionID, eventType, j)
	if err != nil {
		return fmt.Errorf("insert event %s: %w", eventType, err)
	}
	return nil
}

// Events returns a session's events in order.
func (s *Store) Events(ctx context.Context, sessionID string) ([]Event, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, occurred_at, session_id, type, payload
		 FROM event WHERE session_id = $1 ORDER BY id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var e Event
		var payload []byte
		if err := rows.Scan(&e.ID, &e.OccurredAt, &e.SessionID, &e.Type, &payload); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(payload, &e.Payload)
		out = append(out, e)
	}
	return out, rows.Err()
}
