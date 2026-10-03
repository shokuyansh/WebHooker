package main

import (
	"errors"
	"net/http"

	"github.com/shokuyansh/Webhooker/internal/data"
	"github.com/shokuyansh/Webhooker/internal/validator"
)

func (app *application) createProjectHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestErrorResponse(w, r, err)
		return
	}
	v := validator.New()

	project := data.Project{}

	project.Name = input.Name

	if data.ValidateProject(v, project); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	err = app.models.Projects.Create(&project)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, envelope{"project": project}, http.StatusCreated, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) getProjectHandler(w http.ResponseWriter, r *http.Request) {
	project_id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundErrorResponse(w, r)
		return
	}
	project, err := app.models.Projects.Get(int64(project_id))

	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			app.notFoundErrorResponse(w, r)
			return
		}
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, envelope{"project": project}, http.StatusOK, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
