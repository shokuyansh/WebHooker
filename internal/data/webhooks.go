package data

import (
	"database/sql"
	"time"

	"github.com/shokuyansh/Webhooker/internal/validator"
)

type WebHook struct {
	ClientID    int64     `json:"id"`
	CallbackURL string    `json:"callback_url"`
	Events      []string  `json:"events"`
	CreatedAT   time.Time `json:"-"`
	Version     int64     `json:"version"`
}

type WebHookModel struct {
	db *sql.DB
}

func ValidateWebhook(v *validator.Validator, webhook *WebHook) {
	v.Check(len(webhook.CallbackURL) != 0, "callback_url", "must be provided")
	v.Check(validator.ValidURL(webhook.CallbackURL), "callback_url", "Not a valid url")

	v.Check(len(webhook.Events) != 0, "events", "must be provided")
	v.Check(len(webhook.Events) >= 1, "events", "registered events should be atleast 1")

}
