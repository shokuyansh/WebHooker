package data

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Delivery struct {
	ID             int64      `json:"id"`
	EventID        int64      `json:"event_id"`
	WebHookID      int64      `json:"webhook_id"`
	Status         string     `json:"status"`
	AttemptCount   int64      `json:"attempt_count"`
	ResponseStatus *int       `json:"response_status"`
	NextAttemptAt  time.Time  `json:"next_attempt_at"`
	LastAttemptAt  *time.Time `json:"last_attempt_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

type DeliveryModel struct {
	db Querier
}

func (m DeliveryModel) Create(delivery *Delivery) error {
	query := `insert into deliveries(event_id,webhook_id)
	values($1,$2)
	returning id,status,attempt_count,created_at`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	defer cancel()

	err := m.db.QueryRowContext(ctx, query, delivery.EventID, delivery.WebHookID).Scan(
		&delivery.ID,
		&delivery.Status,
		&delivery.AttemptCount,
		&delivery.CreatedAt,
	)

	if err != nil {
		return err
	}

	return nil

}

func (m DeliveryModel) Get(webhook_id, id int64) (*Delivery, error) {
	if id < 1 {
		return nil, ErrRecordNotFound
	}
	query := `select id,event_id,webhook_id,status,attempt_count,response_status,next_attempt_at,last_attempt_at,created_at
	from deliveries where id=$1 and webhook_id=$2`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	defer cancel()

	var d Delivery

	err := m.db.QueryRowContext(ctx, query, id, webhook_id).Scan(
		&d.ID,
		&d.EventID,
		&d.WebHookID,
		&d.Status,
		&d.AttemptCount,
		&d.ResponseStatus,
		&d.NextAttemptAt,
		&d.LastAttemptAt,
		&d.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}
	return &d, nil
}

func (m DeliveryModel) Update(delivery *Delivery) error {
	query := `update deliveries set
	status=$1,attempt_count=attempt_count+1,response_status=$2,next_attempt_at=$3,last_attempt_at=$4
	where id=$5`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	defer cancel()

	args := []any{delivery.Status, delivery.ResponseStatus, delivery.NextAttemptAt, delivery.LastAttemptAt, delivery.ID}
	res, err := m.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrRecordNotFound
	}
	return nil
}

func (m DeliveryModel) BulkInsert(hooks []*WebHook, event Event) error {
	for _, hook := range hooks {
		delivery := Delivery{
			EventID:   event.ID,
			WebHookID: hook.ID,
		}
		err := m.Create(&delivery)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m DeliveryModel) ListAll() ([]*Delivery, error) {
	query := `select id,event_id,webhook_id,status,attempt_count,response_status,next_attempt_at,last_attempt_at,created_at
	from deliveries
	order by created_at desc,id desc`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	defer cancel()

	res, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	var deliveries []*Delivery
	for res.Next() {
		var d Delivery
		err := res.Scan(
			&d.ID,
			&d.EventID,
			&d.WebHookID,
			&d.Status,
			&d.AttemptCount,
			&d.ResponseStatus,
			&d.NextAttemptAt,
			&d.LastAttemptAt,
			&d.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, &d)
	}
	if err = res.Err(); err != nil {
		return nil, err
	}
	return deliveries, nil
}

func (m DeliveryModel) PendingDeliveries() ([]*Delivery, error) {
	query := `select id,event_id,webhook_id,status,attempt_count,response_status,next_attempt_at,last_attempt_at,created_at
	from deliveries where status in ('PENDING','RETRYING') and next_attempt_at<NOW()
	order by next_attempt_at,id limit 50`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	defer cancel()

	res, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	var deliveries []*Delivery
	for res.Next() {
		var d Delivery
		err := res.Scan(
			&d.ID,
			&d.EventID,
			&d.WebHookID,
			&d.Status,
			&d.AttemptCount,
			&d.ResponseStatus,
			&d.NextAttemptAt,
			&d.LastAttemptAt,
			&d.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, &d)
	}
	if err = res.Err(); err != nil {
		return nil, err
	}
	return deliveries, nil
}

func (m DeliveryModel) DeliveriesForWebhook(webhook_id int64) ([]*Delivery, error) {
	if webhook_id < 1 {
		return nil, ErrRecordNotFound
	}
	query := `select id,event_id,webhook_id,status,attempt_count,response_status,next_attempt_at,last_attempt_at,created_at
	from deliveries where webhook_id=$1
	order by created_at desc,id desc`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	defer cancel()

	res, err := m.db.QueryContext(ctx, query, webhook_id)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	var deliveries []*Delivery
	for res.Next() {
		var d Delivery
		err := res.Scan(
			&d.ID,
			&d.EventID,
			&d.WebHookID,
			&d.Status,
			&d.AttemptCount,
			&d.ResponseStatus,
			&d.NextAttemptAt,
			&d.LastAttemptAt,
			&d.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, &d)
	}
	if err = res.Err(); err != nil {
		return nil, err
	}
	return deliveries, nil
}
