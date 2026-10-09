//go:build integration

package main

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/shokuyansh/Webhooker/internal/data"
	"github.com/shokuyansh/Webhooker/internal/testutil"
)

func integrationApplication(t *testing.T) (*application, *sql.DB) {
	t.Helper()
	db := testutil.OpenDB(t)
	app := testApplication()
	app.models = data.NewModels(db)
	app.config.env = "test"
	return app, db
}
func apiRequest(t *testing.T, h http.Handler, method, path string, input any, want int) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	var err error
	if input != nil {
		body, err = json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s status=%d; want=%d body=%s", method, path, w.Code, want, w.Body)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected Content-Type: %s", w.Header().Get("Content-Type"))
	}
	return w
}
func resource[T any](t *testing.T, w *httptest.ResponseRecorder, key string) T {
	t.Helper()
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	raw, ok := envelope[key]
	if !ok {
		t.Fatalf("missing %q in response %s", key, w.Body)
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
func apiProject(t *testing.T, h http.Handler, name string) data.Project {
	t.Helper()
	return resource[data.Project](t, apiRequest(t, h, "POST", "/v1/projects", map[string]any{"name": name}, 201), "project")
}
func apiWebhook(t *testing.T, h http.Handler, project int64, url string, events ...string) (data.WebHook, []byte) {
	t.Helper()
	w := apiRequest(t, h, "POST", "/v1/webhook", map[string]any{"project_id": project, "callback_url": url, "events_registered": events}, 201)
	hook := resource[data.WebHook](t, w, "webhook")
	secret := resource[string](t, w, "signing_secret(ONE TIME)")
	if !strings.HasPrefix(secret, "whsec_") {
		t.Fatalf("secret prefix missing: %q", secret)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil || len(decoded) != 32 {
		t.Fatalf("invalid signing secret length=%d error=%v", len(decoded), err)
	}
	return hook, decoded
}
func deliveryFixture(t *testing.T, url string) (*application, *sql.DB, *data.Delivery, *data.Event, *data.WebHook) {
	t.Helper()
	app, db := integrationApplication(t)
	p := data.Project{Name: "Orders"}
	if err := app.models.Projects.Create(&p); err != nil {
		t.Fatal(err)
	}
	w := data.WebHook{ProjectID: p.ProjectID, CallbackURL: url, Events: []string{"order.paid"}, SigningSecret: bytes.Repeat([]byte{0x42}, 32)}
	if err := app.models.Webhooks.Insert(&w); err != nil {
		t.Fatal(err)
	}
	e := data.Event{ProjectID: p.ProjectID, Type: "order.paid", Payload: json.RawMessage(`{"id":9007199254740993,"ok":true}`)}
	if err := app.models.Events.Insert(&e); err != nil {
		t.Fatal(err)
	}
	stored, err := app.models.Events.Get(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	d := data.Delivery{EventID: e.ID, WebHookID: w.ID}
	if err := app.models.Deliveries.Create(&d); err != nil {
		t.Fatal(err)
	}
	loaded, err := app.models.Deliveries.Get(w.ID, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	return app, db, loaded, stored, &w
}

func TestIntegrationAPIProjects(t *testing.T) {
	app, _ := integrationApplication(t)
	h := app.routes()
	p := apiProject(t, h, "Orders")
	if p.ProjectID < 1 || p.Name != "Orders" || p.CreatedAT.IsZero() {
		t.Fatalf("created project=%+v", p)
	}
	got := resource[data.Project](t, apiRequest(t, h, "GET", fmt.Sprintf("/v1/projects/%d", p.ProjectID), nil, 200), "project")
	if got != p {
		t.Fatalf("project=%+v want=%+v", got, p)
	}
	apiRequest(t, h, "GET", "/v1/projects/999999", nil, 404)
}

func TestIntegrationAPIWebhookLifecycle(t *testing.T) {
	app, db := integrationApplication(t)
	h := app.routes()
	p := apiProject(t, h, "Orders")
	w, secret := apiWebhook(t, h, p.ProjectID, "https://example.com/orders", "order.paid")
	other, otherSecret := apiWebhook(t, h, p.ProjectID, "https://example.com/other", "order.cancelled")
	if bytes.Equal(secret, otherSecret) {
		t.Fatal("secrets must be generated independently")
	}
	path := fmt.Sprintf("/v1/projects/%d/webhooks/%d", p.ProjectID, w.ID)
	resp := apiRequest(t, h, "GET", path, nil, 200)
	if strings.Contains(resp.Body.String(), "whsec_") || strings.Contains(resp.Body.String(), "signing_secret") {
		t.Fatal("GET exposed signing secret")
	}
	got := resource[data.WebHook](t, resp, "webhook")
	if !got.Activated || got.Version != 1 {
		t.Fatalf("defaults=%+v", got)
	}
	apiRequest(t, h, "POST", "/v1/webhook", map[string]any{"project_id": p.ProjectID, "callback_url": w.CallbackURL, "events_registered": []string{"order.paid"}}, 422)
	apiRequest(t, h, "POST", "/v1/webhook", map[string]any{"project_id": 999999, "callback_url": "https://example.com/missing", "events_registered": []string{"order.paid"}}, 422)
	apiRequest(t, h, "PATCH", path, map[string]any{"callback_url": "http://127.0.0.1/hook"}, 422)
	apiRequest(t, h, "PATCH", path, map[string]any{"callback_url": other.CallbackURL}, 422)
	changed := resource[data.WebHook](t, apiRequest(t, h, "PATCH", path, map[string]any{"activated": false}, 200), "webhook")
	if changed.Activated || changed.Version != 2 || changed.CallbackURL != w.CallbackURL || !reflect.DeepEqual(changed.Events, w.Events) {
		t.Fatalf("partial update=%+v", changed)
	}
	changed = resource[data.WebHook](t, apiRequest(t, h, "PATCH", path, map[string]any{"events_registered": []string{"order.refunded"}, "callback_url": "https://example.com/refunded", "activated": true}, 200), "webhook")
	if changed.Version != 3 || !changed.Activated || changed.CallbackURL != "https://example.com/refunded" || !reflect.DeepEqual(changed.Events, []string{"order.refunded"}) {
		t.Fatalf("replacement=%+v", changed)
	}
	full, err := app.models.Webhooks.GetWithSecret(int(p.ProjectID), int(w.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(full.SigningSecret, secret) {
		t.Fatal("update changed signing secret")
	}
	list := resource[[]data.WebHook](t, apiRequest(t, h, "GET", fmt.Sprintf("/v1/projects/%d/webhooks", p.ProjectID), nil, 200), "webhooks")
	if len(list) != 2 {
		t.Fatalf("webhooks=%d", len(list))
	}
	event := data.Event{ProjectID: p.ProjectID, Type: "order.refunded", Payload: json.RawMessage(`{"ok":true}`)}
	if err := app.models.Events.Insert(&event); err != nil {
		t.Fatal(err)
	}
	d := data.Delivery{EventID: event.ID, WebHookID: w.ID}
	if err := app.models.Deliveries.Create(&d); err != nil {
		t.Fatal(err)
	}
	apiRequest(t, h, "DELETE", path, nil, 200)
	apiRequest(t, h, "GET", path, nil, 404)
	apiRequest(t, h, "DELETE", path, nil, 404)
	var count int
	if err := db.QueryRow("SELECT count(*) FROM deliveries WHERE webhook_id=$1", w.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cascade=%d err=%v", count, err)
	}
}

func TestIntegrationAPIProjectIsolation(t *testing.T) {
	app, _, d, e, w := deliveryFixture(t, "https://example.com/orders")
	h := app.routes()
	other := apiProject(t, h, "Other")
	wrong := fmt.Sprintf("/v1/projects/%d/webhooks/%d", other.ProjectID, w.ID)
	apiRequest(t, h, "GET", wrong, nil, 404)
	apiRequest(t, h, "PATCH", wrong, map[string]any{"activated": false}, 404)
	apiRequest(t, h, "DELETE", wrong, nil, 404)
	apiRequest(t, h, "GET", wrong+"/deliveries", nil, 404)
	apiRequest(t, h, "GET", fmt.Sprintf("%s/deliveries/%d", wrong, d.ID), nil, 404)
	correct := fmt.Sprintf("/v1/projects/%d/webhooks/%d", e.ProjectID, w.ID)
	apiRequest(t, h, "GET", correct, nil, 200)
	empty := resource[[]data.WebHook](t, apiRequest(t, h, "GET", fmt.Sprintf("/v1/projects/%d/webhooks", other.ProjectID), nil, 200), "webhooks")
	if len(empty) != 0 {
		t.Fatalf("cross-project listing leaked %d webhooks", len(empty))
	}
}

func TestIntegrationAPIEventFanout(t *testing.T) {
	app, _ := integrationApplication(t)
	h := app.routes()
	p := apiProject(t, h, "Orders")
	other := apiProject(t, h, "Other")
	a, _ := apiWebhook(t, h, p.ProjectID, "https://example.com/a", "order.paid")
	b, _ := apiWebhook(t, h, p.ProjectID, "https://example.com/b", "order.paid", "order.cancelled")
	apiWebhook(t, h, p.ProjectID, "https://example.com/unrelated", "order.cancelled")
	inactive, _ := apiWebhook(t, h, p.ProjectID, "https://example.com/inactive", "order.paid")
	apiRequest(t, h, "PATCH", fmt.Sprintf("/v1/projects/%d/webhooks/%d", p.ProjectID, inactive.ID), map[string]any{"activated": false}, 200)
	apiWebhook(t, h, other.ProjectID, "https://example.com/other", "order.paid")
	path := fmt.Sprintf("/v1/projects/%d/events", p.ProjectID)
	event := resource[data.Event](t, apiRequest(t, h, "POST", path, map[string]any{"type": "order.paid", "payload": json.RawMessage(`{"id":9007199254740993}`)}, 201), "event")
	if event.ID < 1 || event.ProjectID != p.ProjectID || !strings.Contains(string(event.Payload), "9007199254740993") {
		t.Fatalf("event=%+v", event)
	}
	deliveries, err := app.models.Deliveries.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("fan-out=%d; want 2", len(deliveries))
	}
	matched := map[int64]bool{a.ID: true, b.ID: true}
	for _, d := range deliveries {
		if !matched[d.WebHookID] || d.EventID != event.ID || d.Status != "PENDING" || d.AttemptCount != 0 {
			t.Fatalf("unexpected delivery=%+v", d)
		}
		delete(matched, d.WebHookID)
	}
	if len(matched) != 0 {
		t.Fatalf("missing destinations=%v", matched)
	}
	noMatch := resource[data.Event](t, apiRequest(t, h, "POST", path, map[string]any{"type": "order.unknown", "payload": map[string]any{"ok": true}}, 201), "event")
	if _, err := app.models.Events.Get(noMatch.ID); err != nil {
		t.Fatal(err)
	}
	deliveries, err = app.models.Deliveries.ListAll()
	if err != nil || len(deliveries) != 2 {
		t.Fatalf("unmatched event created deliveries=%d err=%v", len(deliveries), err)
	}
}

func TestIntegrationAPIEventRollback(t *testing.T) {
	app, db := integrationApplication(t)
	h := app.routes()
	p := apiProject(t, h, "Orders")
	apiWebhook(t, h, p.ProjectID, "https://example.com/first", "order.paid")
	rejected, _ := apiWebhook(t, h, p.ProjectID, "https://example.com/rejected", "order.paid")
	_, err := db.Exec(fmt.Sprintf(`CREATE FUNCTION reject_test_delivery() RETURNS trigger LANGUAGE plpgsql AS $$
        BEGIN IF NEW.webhook_id=%d THEN RAISE EXCEPTION 'injected delivery insert failure'; END IF; RETURN NEW; END $$;
        CREATE TRIGGER reject_delivery BEFORE INSERT ON deliveries FOR EACH ROW EXECUTE FUNCTION reject_test_delivery()`, rejected.ID))
	if err != nil {
		t.Fatal(err)
	}
	apiRequest(t, h, "POST", fmt.Sprintf("/v1/projects/%d/events", p.ProjectID), map[string]any{"type": "order.paid", "payload": map[string]any{"ok": true}}, 500)
	for _, table := range []string{"events", "deliveries"} {
		var count int
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s partially committed: count=%d err=%v", table, count, err)
		}
	}
}

func TestIntegrationAPIMissingEventProject(t *testing.T) {
	app, _ := integrationApplication(t)
	apiRequest(t, app.routes(), "POST", "/v1/projects/999999/events", map[string]any{"type": "order.paid", "payload": map[string]any{"ok": true}}, 404)
}

func TestIntegrationAPIDeliveryHistory(t *testing.T) {
	app, _, d, e, w := deliveryFixture(t, "https://example.com/orders")
	h := app.routes()
	second := data.Delivery{EventID: e.ID, WebHookID: w.ID}
	if err := app.models.Deliveries.Create(&second); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/v1/projects/%d/webhooks/%d/deliveries", e.ProjectID, w.ID)
	got := resource[data.Delivery](t, apiRequest(t, h, "GET", fmt.Sprintf("%s/%d", path, d.ID), nil, 200), "delivery")
	if got.ID != d.ID || got.Status != "PENDING" || got.AttemptCount != 0 || got.ResponseStatus != nil || got.LastAttemptAt != nil || got.NextAttemptAt.IsZero() {
		t.Fatalf("delivery=%+v", got)
	}
	list := resource[[]data.Delivery](t, apiRequest(t, h, "GET", path, nil, 200), "deliveries")
	if len(list) != 2 || list[0].ID != second.ID || list[1].ID != d.ID {
		t.Fatalf("history=%+v", list)
	}
	all := resource[[]data.Delivery](t, apiRequest(t, h, "GET", "/v1/deliveries", nil, 200), "deliveries")
	if len(all) != 2 {
		t.Fatalf("global history=%d", len(all))
	}
	apiRequest(t, h, "GET", path+"/99999", nil, 404)
	foreign, _ := apiWebhook(t, h, e.ProjectID, "https://example.com/foreign", "order.paid")
	apiRequest(t, h, "GET", fmt.Sprintf("/v1/projects/%d/webhooks/%d/deliveries/%d", e.ProjectID, foreign.ID, d.ID), nil, 404)
}
