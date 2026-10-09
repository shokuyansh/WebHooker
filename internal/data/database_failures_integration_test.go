//go:build integration

package data

import (
	"encoding/json"
	"testing"
)

func TestIntegrationModelsPropagateDatabaseFailures(t *testing.T) {
	m, db := integrationModels(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	calls := []struct {
		name string
		call func() error
	}{
		{"create project", func() error { return m.Projects.Create(&Project{Name: "Orders"}) }},
		{"get project", func() error { _, err := m.Projects.Get(1); return err }},
		{"insert event", func() error {
			return m.Events.Insert(&Event{ProjectID: 1, Type: "a", Payload: json.RawMessage(`{"ok":true}`)})
		}},
		{"get event", func() error { _, err := m.Events.Get(1); return err }},
		{"insert webhook", func() error {
			return m.Webhooks.Insert(&WebHook{ProjectID: 1, CallbackURL: "https://example.com", Events: []string{"a"}})
		}},
		{"get webhook", func() error { _, err := m.Webhooks.Get(1, 1); return err }},
		{"get webhook secret", func() error { _, err := m.Webhooks.GetWithSecret(1, 1); return err }},
		{"update webhook", func() error { return m.Webhooks.Update(&WebHook{ID: 1, ProjectID: 1, Version: 1}) }},
		{"delete webhook", func() error { return m.Webhooks.Delete(1, 1) }},
		{"list webhooks", func() error { _, err := m.Webhooks.GetAllForProject(1); return err }},
		{"match webhooks", func() error { _, err := m.Webhooks.GetActiveWebhooksForEvent(1, "a"); return err }},
		{"create delivery", func() error { return m.Deliveries.Create(&Delivery{EventID: 1, WebHookID: 1}) }},
		{"get delivery", func() error { _, err := m.Deliveries.Get(1, 1); return err }},
		{"update delivery", func() error { return m.Deliveries.Update(&Delivery{ID: 1}) }},
		{"list deliveries", func() error { _, err := m.Deliveries.ListAll(); return err }},
		{"delivery history", func() error { _, err := m.Deliveries.DeliveriesForWebhook(1); return err }},
		{"pending deliveries", func() error { _, err := m.Deliveries.PendingDeliveries(); return err }},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Fatal("database failure was swallowed")
			}
		})
	}
	called := false
	if err := m.WithTransaction(func(Models) error { called = true; return nil }); err == nil || called {
		t.Fatalf("failed BeginTx: error=%v callback called=%v", err, called)
	}
}
