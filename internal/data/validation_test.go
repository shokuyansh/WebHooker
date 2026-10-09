package data

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/shokuyansh/Webhooker/internal/validator"
)

func TestValidateProject(t *testing.T) {
	for _, tc := range []struct {
		name  string
		valid bool
	}{{"", false}, {"Order Service", true}} {
		t.Run(tc.name, func(t *testing.T) {
			v := validator.New()
			ValidateProject(v, Project{Name: tc.name})
			if v.Valid() != tc.valid {
				t.Fatalf("validation = %v; want valid=%v", v.Errors, tc.valid)
			}
			if !tc.valid && v.Errors["name"] == "" {
				t.Fatal("missing name error")
			}
		})
	}
}

func TestValidateEvent(t *testing.T) {
	for _, tc := range []struct{ name, eventType, payload, field string }{
		{"valid", "order.paid", `{"id":9007199254740993}`, ""},
		{"type boundary", strings.Repeat("a", 100), `{"ok":true}`, ""},
		{"missing type", "", `{"ok":true}`, "type"},
		{"long type", strings.Repeat("a", 101), `{"ok":true}`, "type"},
		{"missing payload", "order.paid", "", "payload"},
		{"null", "order.paid", "null", "payload"},
		{"empty object", "order.paid", "{}", "payload"},
		{"array", "order.paid", "[]", "payload"},
		{"scalar", "order.paid", "42", "payload"},
		{"string", "order.paid", `"value"`, "payload"},
		{"malformed", "order.paid", "{", "payload"},
		{"nested", "order.paid", `{"child":{"items":[1,null,true]}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := validator.New()
			ValidateEvent(v, &Event{Type: tc.eventType, Payload: json.RawMessage(tc.payload)})
			if tc.field == "" {
				if !v.Valid() {
					t.Fatalf("unexpected errors: %v", v.Errors)
				}
				return
			}
			if v.Errors[tc.field] == "" {
				t.Fatalf("missing %s error: %v", tc.field, v.Errors)
			}
		})
	}
}

func TestValidateWebhook(t *testing.T) {
	for _, tc := range []struct {
		name    string
		project int64
		url     string
		events  []string
		field   string
	}{
		{"valid", 1, "https://example.com", []string{"a"}, ""},
		{"five events", 1, "https://example.com", []string{"a", "b", "c", "d", "e"}, ""},
		{"zero project", 0, "https://example.com", []string{"a"}, "project_id"},
		{"negative project", -1, "https://example.com", []string{"a"}, "project_id"},
		{"missing callback", 1, "", []string{"a"}, "callback_url"},
		{"missing events", 1, "https://example.com", nil, "events"},
		{"empty events", 1, "https://example.com", []string{}, "events"},
		{"too many", 1, "https://example.com", []string{"a", "b", "c", "d", "e", "f"}, "events"},
		{"duplicate", 1, "https://example.com", []string{"a", "a"}, "events"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := validator.New()
			ValidateWebhook(v, &WebHook{ProjectID: tc.project, CallbackURL: tc.url, Events: tc.events})
			if tc.field == "" {
				if !v.Valid() {
					t.Fatalf("unexpected errors: %v", v.Errors)
				}
				return
			}
			if v.Errors[tc.field] == "" {
				t.Fatalf("missing %s error: %v", tc.field, v.Errors)
			}
		})
	}
}

func TestWebhookJSONHidesSecret(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	raw, err := json.Marshal(WebHook{ID: 1, SigningSecret: secret})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["signing_secret"]; ok {
		t.Fatal("secret field exposed")
	}
	if bytes.Contains(raw, secret) || bytes.Contains(raw, []byte("SigningSecret")) {
		t.Fatalf("secret exposed in %s", raw)
	}
}
