package data

import (
	"database/sql"
	"errors"
)

var (
	ErrRecordNotFound = errors.New("record not found")
	ErrEditConflict   = errors.New("edit conflict")
)

type Models struct {
	Webhooks WebHookModel
	Projects ProjectModel
	Events   EventModel
}

func NewModels(db *sql.DB) Models {
	return Models{
		Webhooks: WebHookModel{DB: db},
		Projects: ProjectModel{DB: db},
		Events:   EventModel{DB: db},
	}
}
