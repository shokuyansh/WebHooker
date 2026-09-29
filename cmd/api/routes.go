package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (app *application) routes() http.Handler {
	router := httprouter.New()

	router.NotFound = http.HandlerFunc(app.notFoundErrorResponse)
	router.MethodNotAllowed = http.HandlerFunc(app.methodNotAllowedErrorResponse)

	router.HandlerFunc(http.MethodGet, "/v1/healthcheckup", app.healthcheckup)
	router.HandlerFunc(http.MethodPost, "/v1/webhook", app.registerWebhookHandlerPost)
	router.HandlerFunc(http.MethodGet, "/v1/projects/:project_id/webhooks/:webhook_id", app.getWebhookHandler)
	router.HandlerFunc(http.MethodPatch, "/v1/projects/:project_id/webhooks/:webhook_id", app.updateWebhookHandler)
	router.HandlerFunc(http.MethodDelete, "/v1/projects/:project_id/webhooks/:webhook_id", app.deleteWebhookHandler)

	router.HandlerFunc(http.MethodGet, "/v1/projects/:project_id/webhooks", app.listWebhooksHandler)
	router.HandlerFunc(http.MethodPost, "/v1/projects", app.createProjectHandler)
	router.HandlerFunc(http.MethodGet, "/v1/projects/:project_id", app.getProjectHandler)

	return app.recoverPanic(router)
}
