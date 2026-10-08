package main

import (
	"crypto/rand"
	"encoding/base64"
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
	secretKey := make([]byte, 32)
	_, err = rand.Read(secretKey)
	if err != nil || len(secretKey) != 32 {
		app.serverErrorResponse(w, r, err)
		return
	}
	webhook.SigningSecret = secretKey
	err = app.models.Webhooks.Insert(webhook)
	if err != nil {
		if errors.Is(err, data.ErrDuplicateURL) {
			v.AddError("url", "this url already exists")
			app.failedValidationResponse(w, r, v.Errors)
			return
		}
		if errors.Is(err, data.ErrNonExistentProject) {
			v.AddError("project_id", "this project does not exists")
			app.failedValidationResponse(w, r, v.Errors)
			return
		}
		app.serverErrorResponse(w, r, err)
		return
	}
	err = app.writeJSON(w, envelope{"webhook": webhook, "signing_secret(ONE TIME)": "whsec_" + base64.StdEncoding.EncodeToString(secretKey)}, http.StatusCreated, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) getWebhookHandler(w http.ResponseWriter, r *http.Request) {
	ids, err := app.readIDParams(r, "id", "webhook_id")
	if err != nil {
		app.notFoundErrorResponse(w, r)
		return
	}
	project_id, webhook_id := ids[0], ids[1]
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
	ids, err := app.readIDParams(r, "id", "webhook_id")
	if err != nil {
		app.notFoundErrorResponse(w, r)
		return
	}
	project_id, webhook_id := ids[0], ids[1]
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
		Activated        *bool    `json:"activated"`
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
	if input.Activated != nil {
		webhook.Activated = *input.Activated
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
	ids, err := app.readIDParams(r, "id", "webhook_id")
	if err != nil {
		app.notFoundErrorResponse(w, r)
		return
	}
	project_id, webhook_id := ids[0], ids[1]
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
	ids, err := app.readIDParams(r, "id")
	if err != nil {
		app.notFoundErrorResponse(w, r)
		return
	}
	project_id := ids[0]
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
