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
	Users    UserModel
	Projects ProjectModel
}

func NewModels(db *sql.DB) Models {
	return Models{
		Webhooks: WebHookModel{DB: db},
		Users:    UserModel{DB: db},
		Projects: ProjectModel{DB: db},
	}
}
