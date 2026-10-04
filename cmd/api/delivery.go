package main

import (
	"net/http"
)

func (app *application) listAllDeliveries(w http.ResponseWriter, r *http.Request) {
	deliveries, err := app.models.Deliveries.ListAll()
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
	err = app.writeJSON(w, envelope{"deliveries": deliveries}, http.StatusOK, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
