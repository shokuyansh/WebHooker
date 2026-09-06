package main

import (
	"net/http"

	"github.com/shokuyansh/Webhooker/internal/data"
	"github.com/shokuyansh/Webhooker/internal/validator"
)

func (app *application) registerWebhookPost(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CallbackUrl      string   `json:"callback_url"`
		EventsRegistered []string `json:"events_registered"`
	}
	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestErrorResponse(w, r, err)
		return
	}

	webhook := &data.WebHook{
		CallbackURL: input.CallbackUrl,
		Events:      input.EventsRegistered,
	}
	v := validator.New()

	if data.ValidateWebhook(v, webhook); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	err = app.writeJSON(w, webhook, http.StatusCreated, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
