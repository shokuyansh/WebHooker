package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/shokuyansh/Webhooker/internal/data"
	"github.com/shokuyansh/Webhooker/internal/validator"
)

func (app *application) createEventHandler(w http.ResponseWriter, r *http.Request) {
	project_id, err := app.readIDParam(r)
	if err != nil {
		app.badRequestErrorResponse(w, r, err)
		return
	}

	var input struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}

	err = app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestErrorResponse(w, r, err)
		return
	}

	event := data.Event{}
	event.ProjectID = int64(project_id)
	event.Type = input.Type
	event.Payload = input.Payload

	v := validator.New()
	if data.ValidateEvent(v, &event); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	err = app.models.WithTransaction(func(m data.Models) error {
		if err := m.Events.Insert(&event); err != nil {
			return err
		}
		_, err := m.Webhooks.GetActiveWebhooksForEvent(event.ProjectID, event.Type)
		if err != nil {
			return err
		}
		// deliveries triggered on the hooks acquired
		return nil
	})

	if err != nil {
		if errors.Is(err, data.ErrNonExistentProject) {
			app.notFoundErrorResponse(w, r)
			return
		}
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, envelope{"event": event}, http.StatusCreated, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
