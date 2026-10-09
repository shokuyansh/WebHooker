package data

import (
	"errors"
	"fmt"
	"testing"
)

func TestModelsRejectInvalidIDsBeforeQuery(t *testing.T) {
	m := NewModels(nil)
	for _, id := range []int64{0, -1} {
		t.Run(fmt.Sprintf("id_%d", id), func(t *testing.T) {
			checks := []func() error{
				func() error { _, err := m.Events.Get(id); return err },
				func() error { _, err := m.Deliveries.Get(1, id); return err },
				func() error { _, err := m.Deliveries.DeliveriesForWebhook(id); return err },
				func() error { _, err := m.Webhooks.Get(int(id), 1); return err },
				func() error { _, err := m.Webhooks.Get(1, int(id)); return err },
				func() error { _, err := m.Webhooks.GetWithSecret(int(id), 1); return err },
				func() error { _, err := m.Webhooks.GetWithSecret(1, int(id)); return err },
				func() error { return m.Webhooks.Delete(id, 1) },
				func() error { return m.Webhooks.Delete(1, id) },
			}
			for i, check := range checks {
				if err := check(); !errors.Is(err, ErrRecordNotFound) {
					t.Fatalf("check %d error=%v", i, err)
				}
			}
		})
	}
}
