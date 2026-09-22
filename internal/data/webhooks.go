package data

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
	"github.com/shokuyansh/Webhooker/internal/validator"
)

var (
	ErrDuplicateURL = errors.New("duplicate url")
)

type WebHook struct {
	ID          int64     `json:"id"`
	ProjectID   int64     `json:"project_id"`
	CallbackURL string    `json:"callback_url"`
	Events      []string  `json:"events"`
	Activated   bool      `json:"activated"`
	CreatedAT   time.Time `json:"-"`
	Version     int64     `json:"version"`
}

type WebHookModel struct {
	DB *sql.DB
}

func ValidateWebhook(v *validator.Validator, webhook *WebHook) {
	v.Check(len(webhook.CallbackURL) != 0, "callback_url", "must be provided")
	v.Check(validator.ValidURL(webhook.CallbackURL), "callback_url", "Not a valid url")

	v.Check(webhook.Events != nil, "events", "must be provided")
	v.Check(len(webhook.Events) >= 1, "events", "registered events should be atleast 1")
	v.Check(len(webhook.Events) <= 5, "events", "must not contain more than 5 events")
	v.Check(validator.Unique(webhook.Events), "events", "must not contain duplicate values")
}

func (m WebHookModel) Insert(webhook *WebHook) error {
	stmt := `Insert into webhooks(project_id,callback_url,events)
	values($1,$2,$3)
	returning id,created_at,version`
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := m.DB.QueryRowContext(ctx, stmt, webhook.ProjectID, webhook.CallbackURL, pq.Array(webhook.Events)).Scan(
		&webhook.ID, &webhook.CreatedAT, &webhook.Version)
	if err != nil {
		var pqErr *pq.Error
		switch {
		case errors.As(err, &pqErr) &&
			pqErr.Code == "23505" &&
			pqErr.Constraint == "webhooks_callback_url_key":
			return ErrDuplicateURL
		default:
			return err
		}
	}
	return nil
}

func (m WebHookModel) Get(id int) (*WebHook, error) {
	if id < 1 {
		return nil, ErrRecordNotFound
	}
	query := `select id,project_id,callback_url,events,activated,created_at,version from webhooks
	where id=$1`
	var webhook WebHook
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := m.DB.QueryRowContext(ctx, query, id).Scan(
		&webhook.ID,
		&webhook.ProjectID,
		&webhook.CallbackURL,
		pq.Array(&webhook.Events),
		&webhook.Activated,
		&webhook.CreatedAT,
		&webhook.Version,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}
	return &webhook, nil
}

func (m WebHookModel) Update(webhook *WebHook) error {
	query := `update webhooks set
	project_id=$1,callback_url=$2,events=$3,activated=$4,version=version+1
	where id=$5 and version=$6
	returning version
	`
	args := []any{webhook.ProjectID, webhook.CallbackURL, pq.Array(webhook.Events), webhook.Activated, webhook.ID, webhook.Version}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := m.DB.QueryRowContext(ctx, query, args...).Scan(&webhook.Version)
	if err != nil {
		var pqErr *pq.Error
		switch {
		case errors.As(err, &pqErr) &&
			pqErr.Code == "23505" &&
			pqErr.Constraint == "webhooks_callback_url_key":
			return ErrDuplicateURL
		case errors.Is(err, sql.ErrNoRows):
			return ErrEditConflict
		default:
			return err
		}
	}
	return nil
}

func (m WebHookModel) Delete(id int) error {
	if id < 1 {
		return ErrRecordNotFound
	}
	query := `delete from webhooks
	where id=$1`
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := m.DB.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrRecordNotFound
	}
	return nil
}
