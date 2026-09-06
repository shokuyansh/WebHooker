package main

import (
	"net/http"
)

func (app *application) errorResponse(w http.ResponseWriter, r *http.Request, msg any, statusCode int) {
	errMsg := envelope{
		"error": msg,
	}
	err := app.writeJSON(w, errMsg, statusCode, nil)
	if err != nil {
		app.logger.Error(err.Error())
		w.WriteHeader(500)
	}
}

func (app *application) serverErrorResponse(w http.ResponseWriter, r *http.Request, err error) {
	app.logger.Error(err.Error())
	msg := "Server could not process your request."
	app.errorResponse(w, r, msg, http.StatusInternalServerError)
}
func (app *application) notFoundErrorResponse(w http.ResponseWriter, r *http.Request) {
	msg := "Resource could not be found!"
	app.errorResponse(w, r, msg, http.StatusNotFound)
}

func (app *application) methodNotAllowedErrorResponse(w http.ResponseWriter, r *http.Request) {
	msg := "Method is not allowed"
	app.errorResponse(w, r, msg, http.StatusMethodNotAllowed)
}

func (app *application) badRequestErrorResponse(w http.ResponseWriter, r *http.Request, err error) {
	app.errorResponse(w, r, err.Error(), http.StatusBadRequest)
}

func (app *application) failedValidationResponse(w http.ResponseWriter, r *http.Request, err map[string]string) {
	app.errorResponse(w, r, err, http.StatusUnprocessableEntity)
}
