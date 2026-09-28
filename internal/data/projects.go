package data

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/shokuyansh/Webhooker/internal/validator"
)

type Project struct {
	Project_ID int64     `json:"project_id"`
	Name       string    `json:"name"`
	Created_AT time.Time `json:"created_at"`
}

type ProjectModel struct {
	DB *sql.DB
}

func ValidateProject(v *validator.Validator, project Project) {
	v.Check(len(project.Name) != 0, "name", "must be provided")
}

func (m ProjectModel) CREATE(project *Project) error {
	query := `insert into projects(name) 
	values($1)
	returning project_id,created_at`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, project.Name).Scan(&project.Project_ID, &project.Created_AT)

	if err != nil {
		return err
	}
	return nil
}

func (m ProjectModel) GET(project_id int64) (*Project, error) {
	query := `select project_id,name,created_at from projects
	where project_id=$1`

	project := Project{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, project_id).Scan(&project.Project_ID, &project.Name, &project.Created_AT)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}
	return &project, nil
}
