package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/shokuyansh/Webhooker/internal/data"
	"github.com/shokuyansh/Webhooker/internal/validator"
)

func (app *application) createEventHandler(w http.ResponseWriter, r *http.Request) {
	ids, err := app.readIDParams(r, "id")
	if err != nil {
		app.badRequestErrorResponse(w, r, err)
		return
	}
	project_id := ids[0]
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
		hooks, err := m.Webhooks.GetActiveWebhooksForEvent(event.ProjectID, event.Type)
		if err != nil {
			return err
		}
		// deliveries triggered on the hooks acquired
		err = m.Deliveries.BulkInsert(hooks, event)
		if err != nil {
			return err
		}
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
