package main

import (
	"net/http"
)

func (app *application) healthcheckup(w http.ResponseWriter, r *http.Request) {
	data := envelope{
		"status": "Available",
		"system_info": map[string]string{
			"Environment": app.config.env,
			"Version":     version,
		},
	}
	err := app.writeJSON(w, data, http.StatusOK, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
