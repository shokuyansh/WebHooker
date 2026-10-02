package data

import (
	"context"
	"database/sql"
	"time"

	"github.com/shokuyansh/Webhooker/internal/validator"
)

type Event struct {
	ID         int64          `json:"id"`
	Product_ID int64          `json:"product_id"`
	Type       string         `json:"type"`
	Payload    map[string]any `json:"payload"`
	Created_AT time.Time      `json:"created_at"`
}

type EventModel struct {
	DB *sql.DB
}

func ValidateEvent(v *validator.Validator, event *Event) {
	v.Check(len(event.Type) != 0, "type", "must be provided")
	v.Check(len(event.Type) < 100, "type", "max 100 characters")

	v.Check(len(event.Payload) != 0, "payload", "must be provided")
}

func (m EventModel) INSERT(event *Event) error {
	query := `insert into events(product_id,type,payload)
	values($1,$2,$3)
	returning id,created_at`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	args := []any{event.Product_ID, event.Type, event.Payload}
	err := m.DB.QueryRowContext(ctx, query, args...).Scan(
		event.ID,
		event.Created_AT,
	)
	if err != nil {
		return err
	}
	return nil
}
