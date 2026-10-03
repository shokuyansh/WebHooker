package data

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/lib/pq"
	"github.com/shokuyansh/Webhooker/internal/validator"
)

type Event struct {
	ID        int64           `json:"id"`
	ProjectID int64           `json:"project_id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAT time.Time       `json:"created_at"`
}

type EventModel struct {
	db Querier
}

func ValidateEvent(v *validator.Validator, event *Event) {
	v.Check(len(event.Type) != 0, "type", "must be provided")
	v.Check(len(event.Type) <= 100, "type", "max 100 characters")

	v.Check(len(event.Payload) != 0, "payload", "must be provided")
	var object map[string]json.RawMessage
	if err := json.Unmarshal(event.Payload, &object); err != nil || object == nil {
		v.AddError("payload", "payload must be a json object ; null is not allowed")
	}
	if object != nil && len(object) == 0 {
		v.AddError("payload", "must contain atleast one field")
	}
}

func (m EventModel) Insert(event *Event) error {
	query := `insert into events(project_id,type,payload)
	values($1,$2,$3)
	returning id,created_at`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	args := []any{event.ProjectID, event.Type, string(event.Payload)}
	err := m.db.QueryRowContext(ctx, query, args...).Scan(
		&event.ID,
		&event.CreatedAT,
	)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) &&
			pqErr.Code == "23503" &&
			pqErr.Constraint == "webhooks_project_id_fkey" {
			return ErrNonExistentProject
		}
		return err
	}
	return nil
}
