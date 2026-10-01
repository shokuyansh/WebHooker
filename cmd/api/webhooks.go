package main

import (
	"errors"
	"net/http"

	"github.com/shokuyansh/Webhooker/internal/data"
	"github.com/shokuyansh/Webhooker/internal/urlguard"
	"github.com/shokuyansh/Webhooker/internal/validator"
)

func (app *application) registerWebhookHandlerPost(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ProjectID        int64    `json:"project_id"`
		CallbackUrl      string   `json:"callback_url"`
		EventsRegistered []string `json:"events_registered"`
	}
	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestErrorResponse(w, r, err)
		return
	}

	webhook := &data.WebHook{
		ProjectID:   input.ProjectID,
		CallbackURL: input.CallbackUrl,
		Events:      input.EventsRegistered,
	}
	v := validator.New()

	v.Check(urlguard.ValidURL(webhook.CallbackURL), "callback_url", "must be a valid public http or https URL")

	if data.ValidateWebhook(v, webhook); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}
	err = app.models.Webhooks.Insert(webhook)
	if err != nil {
		if errors.Is(err, data.ErrDuplicateURL) {
			v.AddError("url", "this url already exists")
			app.failedValidationResponse(w, r, v.Errors)
			return
		}
		app.serverErrorResponse(w, r, err)
		return
	}
	err = app.writeJSON(w, envelope{"webhook": webhook}, http.StatusCreated, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) getWebhookHandler(w http.ResponseWriter, r *http.Request) {
	project_id, webhook_id, err := app.readIDParams(r)
	if err != nil {
		app.notFoundErrorResponse(w, r)
		return
	}
	webhook, err := app.models.Webhooks.Get(project_id, webhook_id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundErrorResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}
	err = app.writeJSON(w, envelope{"webhook": webhook}, http.StatusOK, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) updateWebhookHandler(w http.ResponseWriter, r *http.Request) {
	project_id, webhook_id, err := app.readIDParams(r)
	if err != nil {
		app.notFoundErrorResponse(w, r)
		return
	}
	v := validator.New()

	webhook, err := app.models.Webhooks.Get(project_id, webhook_id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrDuplicateURL):
			v.AddError("url", "this url already exists")
			app.failedValidationResponse(w, r, v.Errors)
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundErrorResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	var input struct {
		CallbackUrl      *string  `json:"callback_url"`
		EventsRegistered []string `json:"events_registered"`
	}
	err = app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestErrorResponse(w, r, err)
		return
	}
	if input.CallbackUrl != nil {
		webhook.CallbackURL = *input.CallbackUrl
	}
	if input.EventsRegistered != nil {
		webhook.Events = input.EventsRegistered
	}
	v.Check(urlguard.ValidURL(webhook.CallbackURL), "callback_url", "must be a valid public http or https URL")

	if data.ValidateWebhook(v, webhook); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}
	err = app.models.Webhooks.Update(webhook)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrDuplicateURL):
			v.AddError("url", "this url already exists")
			app.failedValidationResponse(w, r, v.Errors)
		case errors.Is(err, data.ErrEditConflict):
			app.editConflictResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}
	err = app.writeJSON(w, envelope{"webhook": webhook}, http.StatusOK, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) deleteWebhookHandler(w http.ResponseWriter, r *http.Request) {
	project_id, webhook_id, err := app.readIDParams(r)
	if err != nil {
		app.notFoundErrorResponse(w, r)
		return
	}
	err = app.models.Webhooks.Delete(int64(project_id), int64(webhook_id))
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundErrorResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}
	err = app.writeJSON(w, envelope{"message": "webhook successfully deleted"}, http.StatusOK, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) listWebhooksHandler(w http.ResponseWriter, r *http.Request) {
	project_id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundErrorResponse(w, r)
		return
	}

	webhooks, err := app.models.Webhooks.GetAllForProject(int64(project_id))
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, envelope{"webhooks": webhooks}, http.StatusOK, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
