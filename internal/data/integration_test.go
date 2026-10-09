//go:build integration

package data

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/shokuyansh/Webhooker/internal/testutil"
)

func integrationModels(t *testing.T) (Models, *sql.DB) {
	t.Helper()
	db := testutil.OpenDB(t)
	return NewModels(db), db
}
func seedProject(t *testing.T, m Models, name string) *Project {
	t.Helper()
	p := &Project{Name: name}
	if err := m.Projects.Create(p); err != nil {
		t.Fatal(err)
	}
	return p
}
func seedWebhook(t *testing.T, m Models, project int64, url string, events ...string) *WebHook {
	t.Helper()
	w := &WebHook{ProjectID: project, CallbackURL: url, Events: events, SigningSecret: bytes.Repeat([]byte{0x42}, 32)}
	if err := m.Webhooks.Insert(w); err != nil {
		t.Fatal(err)
	}
	return w
}
func seedEvent(t *testing.T, m Models, project int64) *Event {
	t.Helper()
	e := &Event{ProjectID: project, Type: "order.paid", Payload: json.RawMessage(`{"id":9007199254740993}`)}
	if err := m.Events.Insert(e); err != nil {
		t.Fatal(err)
	}
	return e
}
func seedDelivery(t *testing.T, m Models, e *Event, w *WebHook) *Delivery {
	t.Helper()
	d := &Delivery{EventID: e.ID, WebHookID: w.ID}
	if err := m.Deliveries.Create(d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestIntegrationProjects(t *testing.T) {
	m, _ := integrationModels(t)
	p := seedProject(t, m, "Orders")
	if p.ProjectID < 1 || p.CreatedAT.IsZero() {
		t.Fatalf("defaults not populated: %+v", p)
	}
	got, err := m.Projects.Get(p.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if *got != *p {
		t.Fatalf("got=%+v want=%+v", got, p)
	}
	if _, err := m.Projects.Get(p.ProjectID + 1); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("missing project error=%v", err)
	}
}

func TestIntegrationWebhookCRUD(t *testing.T) {
	m, _ := integrationModels(t)
	p := seedProject(t, m, "Orders")
	other := seedProject(t, m, "Other")
	w := seedWebhook(t, m, p.ProjectID, "https://example.com/orders", "order.paid")
	if w.ID < 1 || !w.Activated || w.Version != 1 || w.CreatedAT.IsZero() {
		t.Fatalf("defaults=%+v", w)
	}
	got, err := m.Webhooks.Get(int(p.ProjectID), int(w.ID))
	if err != nil {
		t.Fatal(err)
	}
	if got.SigningSecret != nil || got.CallbackURL != w.CallbackURL || !reflect.DeepEqual(got.Events, w.Events) {
		t.Fatalf("public lookup=%+v", got)
	}
	full, err := m.Webhooks.GetWithSecret(int(p.ProjectID), int(w.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(full.SigningSecret, w.SigningSecret) {
		t.Fatal("stored signing secret changed")
	}
	if _, err := m.Webhooks.Get(int(other.ProjectID), int(w.ID)); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("cross-project lookup=%v", err)
	}
	list, err := m.Webhooks.GetAllForProject(p.ProjectID)
	if err != nil || len(list) != 1 || list[0].ID != w.ID {
		t.Fatalf("list=%v err=%v", list, err)
	}
	empty, err := m.Webhooks.GetAllForProject(other.ProjectID)
	if err != nil || len(empty) != 0 {
		t.Fatalf("cross-project list=%v err=%v", empty, err)
	}
	got.Activated = false
	got.Events = []string{"order.cancelled"}
	got.CallbackURL = "https://example.com/changed"
	if err := m.Webhooks.Update(got); err != nil {
		t.Fatal(err)
	}
	updated, err := m.Webhooks.GetWithSecret(int(p.ProjectID), int(w.ID))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.Activated || !reflect.DeepEqual(updated.Events, got.Events) || updated.CallbackURL != got.CallbackURL || !bytes.Equal(updated.SigningSecret, w.SigningSecret) {
		t.Fatalf("update=%+v", updated)
	}
	if err := m.Webhooks.Delete(other.ProjectID, w.ID); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("cross-project deletion=%v", err)
	}
	if err := m.Webhooks.Delete(p.ProjectID, w.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.Webhooks.Delete(p.ProjectID, w.ID); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("repeated deletion=%v", err)
	}
}

func TestIntegrationWebhookConstraints(t *testing.T) {
	m, _ := integrationModels(t)
	p := seedProject(t, m, "Orders")
	w := seedWebhook(t, m, p.ProjectID, "https://example.com/a", "order.paid")
	duplicate := *w
	duplicate.ID = 0
	if err := m.Webhooks.Insert(&duplicate); !errors.Is(err, ErrDuplicateURL) {
		t.Fatalf("duplicate error=%v", err)
	}
	invalid := duplicate
	invalid.ProjectID = p.ProjectID + 100
	invalid.CallbackURL = "https://example.com/missing"
	if err := m.Webhooks.Insert(&invalid); !errors.Is(err, ErrNonExistentProject) {
		t.Fatalf("foreign key error=%v", err)
	}
	invalid.ProjectID = p.ProjectID
	invalid.SigningSecret = []byte("short")
	var pqErr *pq.Error
	if err := m.Webhooks.Insert(&invalid); !errors.As(err, &pqErr) || pqErr.Code != "23514" {
		t.Fatalf("short secret error=%v", err)
	}
	stale := *w
	fresh := *w
	if err := m.Webhooks.Update(&fresh); err != nil {
		t.Fatal(err)
	}
	if err := m.Webhooks.Update(&stale); !errors.Is(err, ErrEditConflict) {
		t.Fatalf("stale update error=%v", err)
	}
	other := seedWebhook(t, m, p.ProjectID, "https://example.com/b", "order.paid")
	other.CallbackURL = w.CallbackURL
	if err := m.Webhooks.Update(other); !errors.Is(err, ErrDuplicateURL) {
		t.Fatalf("duplicate update error=%v", err)
	}
}

func TestIntegrationWebhookMatching(t *testing.T) {
	m, _ := integrationModels(t)
	p := seedProject(t, m, "Orders")
	other := seedProject(t, m, "Other")
	match := seedWebhook(t, m, p.ProjectID, "https://example.com/match", "order.paid", "order.cancelled")
	seedWebhook(t, m, p.ProjectID, "https://example.com/unrelated", "order.paid.extra")
	seedWebhook(t, m, other.ProjectID, "https://example.com/other", "order.paid")
	inactive := seedWebhook(t, m, p.ProjectID, "https://example.com/inactive", "order.paid")
	inactive.Activated = false
	if err := m.Webhooks.Update(inactive); err != nil {
		t.Fatal(err)
	}
	got, err := m.Webhooks.GetActiveWebhooksForEvent(p.ProjectID, "order.paid")
	if err != nil || len(got) != 1 || got[0].ID != match.ID {
		t.Fatalf("matches=%v error=%v", got, err)
	}
	got, err = m.Webhooks.GetActiveWebhooksForEvent(p.ProjectID, "ORDER.PAID")
	if err != nil || len(got) != 0 {
		t.Fatalf("case-insensitive match=%v error=%v", got, err)
	}
}

func TestIntegrationEventRoundTrip(t *testing.T) {
	m, _ := integrationModels(t)
	p := seedProject(t, m, "Orders")
	e := seedEvent(t, m, p.ProjectID)
	got, err := m.Events.Get(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if e.ID < 1 || e.CreatedAT.IsZero() || got.ProjectID != e.ProjectID || got.Type != e.Type || !strings.Contains(string(got.Payload), "9007199254740993") {
		t.Fatalf("event=%+v", got)
	}
}

func TestIntegrationEventErrorMapping(t *testing.T) {
	m, _ := integrationModels(t)
	t.Run("missing project", func(t *testing.T) {
		e := Event{ProjectID: 99999, Type: "order.paid", Payload: json.RawMessage(`{"ok":true}`)}
		if err := m.Events.Insert(&e); !errors.Is(err, ErrNonExistentProject) {
			t.Fatalf("error=%v; want ErrNonExistentProject", err)
		}
	})
	t.Run("missing event", func(t *testing.T) {
		if _, err := m.Events.Get(99999); !errors.Is(err, ErrRecordNotFound) {
			t.Fatalf("error=%v; want ErrRecordNotFound", err)
		}
	})
}

func TestIntegrationTransactions(t *testing.T) {
	m, db := integrationModels(t)
	p := seedProject(t, m, "Orders")
	w := seedWebhook(t, m, p.ProjectID, "https://example.com", "order.paid")
	e := Event{ProjectID: p.ProjectID, Type: "order.paid", Payload: json.RawMessage(`{"ok":true}`)}
	if err := m.WithTransaction(func(tx Models) error {
		if err := tx.Events.Insert(&e); err != nil {
			return err
		}
		return tx.Deliveries.BulkInsert([]*WebHook{w}, e)
	}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM deliveries WHERE event_id=$1", e.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("committed deliveries=%d err=%v", count, err)
	}
	sentinel := errors.New("abort transaction")
	failed := e
	failed.ID = 0
	err := m.WithTransaction(func(tx Models) error {
		if err := tx.Events.Insert(&failed); err != nil {
			return err
		}
		if err := tx.Deliveries.BulkInsert([]*WebHook{w}, failed); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("transaction error=%v", err)
	}
	if err := db.QueryRow("SELECT count(*) FROM events WHERE id=$1", failed.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rolled-back events=%d err=%v", count, err)
	}
	if err := db.QueryRow("SELECT count(*) FROM deliveries WHERE event_id=$1", failed.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rolled-back deliveries=%d err=%v", count, err)
	}
	failed.ID = 0
	err = m.WithTransaction(func(tx Models) error {
		if err := tx.Events.Insert(&failed); err != nil {
			return err
		}
		return tx.Deliveries.BulkInsert([]*WebHook{w, {ID: 999999}}, failed)
	})
	if err == nil {
		t.Fatal("foreign-key failure unexpectedly committed")
	}
	if err := db.QueryRow("SELECT count(*) FROM events WHERE id=$1", failed.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial fan-out retained event: %d err=%v", count, err)
	}
	if err := db.QueryRow("SELECT count(*) FROM deliveries WHERE event_id=$1", failed.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial fan-out retained delivery: %d err=%v", count, err)
	}
}

func TestIntegrationDeliveryCRUD(t *testing.T) {
	m, _ := integrationModels(t)
	p := seedProject(t, m, "Orders")
	w := seedWebhook(t, m, p.ProjectID, "https://example.com", "order.paid")
	e := seedEvent(t, m, p.ProjectID)
	d := seedDelivery(t, m, e, w)
	got, err := m.Deliveries.Get(w.ID, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "PENDING" || got.AttemptCount != 0 || got.ResponseStatus != nil || got.LastAttemptAt != nil || got.NextAttemptAt.IsZero() || got.CreatedAt.IsZero() {
		t.Fatalf("defaults=%+v", got)
	}
	if _, err := m.Deliveries.Get(w.ID+1, d.ID); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("wrong webhook lookup=%v", err)
	}
	code := 503
	now := time.Now().UTC().Truncate(time.Second)
	got.Status = "RETRYING"
	got.ResponseStatus = &code
	got.LastAttemptAt = &now
	got.NextAttemptAt = now.Add(time.Minute)
	if err := m.Deliveries.Update(got); err != nil {
		t.Fatal(err)
	}
	stored, err := m.Deliveries.Get(w.ID, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AttemptCount != 1 || stored.Status != "RETRYING" || stored.ResponseStatus == nil || *stored.ResponseStatus != 503 || stored.LastAttemptAt == nil || !stored.LastAttemptAt.Equal(now) || !stored.NextAttemptAt.Equal(got.NextAttemptAt) {
		t.Fatalf("persisted=%+v", stored)
	}
	if err := m.Deliveries.Update(&Delivery{ID: 99999, Status: "FAILED", NextAttemptAt: now}); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("missing update=%v", err)
	}
	list, err := m.Deliveries.DeliveriesForWebhook(w.ID)
	if err != nil || len(list) != 1 || list[0].ID != d.ID {
		t.Fatalf("history=%v error=%v", list, err)
	}
	list, err = m.Deliveries.ListAll()
	if err != nil || len(list) != 1 {
		t.Fatalf("all=%v error=%v", list, err)
	}
}

func TestIntegrationDeliveryQueue(t *testing.T) {
	m, db := integrationModels(t)
	p := seedProject(t, m, "Orders")
	w := seedWebhook(t, m, p.ProjectID, "https://example.com", "order.paid")
	e := seedEvent(t, m, p.ProjectID)
	_, err := db.Exec(`INSERT INTO deliveries(event_id,webhook_id,status,next_attempt_at)
        SELECT $1,$2,CASE WHEN i%2=0 THEN 'PENDING' ELSE 'RETRYING' END,NOW()-interval '1 minute' FROM generate_series(1,60) i`, e.ID, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"SUCCESS", "FAILED", "PENDING", "RETRYING"} {
		when := "NOW()-interval '1 hour'"
		if status == "PENDING" || status == "RETRYING" {
			when = "NOW()+interval '1 hour'"
		}
		if _, err := db.Exec("INSERT INTO deliveries(event_id,webhook_id,status,next_attempt_at) VALUES($1,$2,$3,"+when+")", e.ID, w.ID, status); err != nil {
			t.Fatal(err)
		}
	}
	pending, err := m.Deliveries.PendingDeliveries()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 50 {
		t.Fatalf("pending batch size=%d; want 50", len(pending))
	}
	for i, d := range pending {
		if d.ID != int64(i+1) || d.Status != "PENDING" && d.Status != "RETRYING" || d.NextAttemptAt.After(time.Now()) {
			t.Fatalf("queue item %d=%+v", i, d)
		}
	}
	all, err := m.Deliveries.ListAll()
	if err != nil || len(all) != 64 {
		t.Fatalf("all=%d error=%v", len(all), err)
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].ID <= all[i].ID {
			t.Fatal("history tie-breaker must be descending ID")
		}
	}
	if _, err := db.Exec("UPDATE deliveries SET next_attempt_at=NOW()+interval '1 hour'"); err != nil {
		t.Fatal(err)
	}
	pending, err = m.Deliveries.PendingDeliveries()
	if err != nil || len(pending) != 0 {
		t.Fatalf("future deliveries selected: %v error=%v", pending, err)
	}
}

func TestIntegrationWebhookDeletionCascadesDeliveries(t *testing.T) {
	m, db := integrationModels(t)
	p := seedProject(t, m, "Orders")
	w := seedWebhook(t, m, p.ProjectID, "https://example.com", "order.paid")
	e := seedEvent(t, m, p.ProjectID)
	seedDelivery(t, m, e, w)
	if err := m.Webhooks.Delete(p.ProjectID, w.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM deliveries").Scan(&count); err != nil || count != 0 {
		t.Fatalf("delivery cascade count=%d error=%v", count, err)
	}
	if _, err := m.Events.Get(e.ID); err != nil {
		t.Fatalf("deleting webhook removed event: %v", err)
	}
}
