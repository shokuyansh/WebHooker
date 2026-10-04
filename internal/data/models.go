package data

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	ErrRecordNotFound = errors.New("record not found")
	ErrEditConflict   = errors.New("edit conflict")
)

const defaultTxTimeout = 5 * time.Second

type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type Models struct {
	Webhooks   WebHookModel
	Projects   ProjectModel
	Events     EventModel
	Deliveries DeliveryModel
	pool       *sql.DB
}

func NewModels(db *sql.DB) Models {
	return Models{
		Webhooks:   WebHookModel{db: db},
		Projects:   ProjectModel{db: db},
		Events:     EventModel{db: db},
		Deliveries: DeliveryModel{db: db},
		pool:       db,
	}
}

// Made for an atomic transaction that can be exported anywhere in codebase
func (m Models) WithTransaction(fn func(Models) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTxTimeout)
	defer cancel()

	tx, err := m.pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	txModels := Models{
		Webhooks:   WebHookModel{db: tx},
		Projects:   ProjectModel{db: tx},
		Events:     EventModel{db: tx},
		Deliveries: DeliveryModel{db: tx},
	}

	if err = fn(txModels); err != nil {
		return err
	}

	return tx.Commit()
}
