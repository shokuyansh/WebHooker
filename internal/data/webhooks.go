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
	v.Check(webhook.ProjectID > 0, "project_id", "must be greater than 0")

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

func (m WebHookModel) Get(project_id, webhook_id int) (*WebHook, error) {
	if project_id < 1 || webhook_id < 1 {
		return nil, ErrRecordNotFound
	}
	query := `select id,project_id,callback_url,events,activated,created_at,version from webhooks
	where project_id=$1 and id = $2`
	var webhook WebHook
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := m.DB.QueryRowContext(ctx, query, project_id, webhook_id).Scan(
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
	callback_url=$1,events=$2,version=version+1
	where id=$3 and project_id=$4 and version=$5
	returning version
	`
	args := []any{webhook.CallbackURL, pq.Array(webhook.Events), webhook.ID, webhook.ProjectID, webhook.Version}
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

func (m WebHookModel) Delete(project_id, webhook_id int64) error {
	if project_id < 1 || webhook_id < 1 {
		return ErrRecordNotFound
	}
	query := `delete from webhooks
	where id=$1 and project_id=$2`
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := m.DB.ExecContext(ctx, query, webhook_id, project_id)
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

func (m WebHookModel) GetAllForProject(project_id int64) ([]*WebHook, error) {
	query := `select id,project_id,callback_url,events,activated,created_at,version from webhooks
	where project_id=$1`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	res, err := m.DB.QueryContext(ctx, query, project_id)

	if err != nil {
		return nil, err
	}

	defer res.Close()

	var results []*WebHook
	for res.Next() {
		var webhook WebHook
		err := res.Scan(
			&webhook.ID,
			&webhook.ProjectID,
			&webhook.CallbackURL,
			pq.Array(&webhook.Events),
			&webhook.Activated,
			&webhook.CreatedAT,
			&webhook.Version,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, &webhook)
	}
	if err = res.Err(); err != nil {
		return nil, err
	}
	return results, err
}
