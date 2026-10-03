package data

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/shokuyansh/Webhooker/internal/validator"
)

type Project struct {
	ProjectID int64     `json:"project_id"`
	Name      string    `json:"name"`
	CreatedAT time.Time `json:"created_at"`
}

type ProjectModel struct {
	db Querier
}

func ValidateProject(v *validator.Validator, project Project) {
	v.Check(len(project.Name) != 0, "name", "must be provided")
}

func (m ProjectModel) Create(project *Project) error {
	query := `insert into projects(name) 
	values($1)
	returning project_id,created_at`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	defer cancel()

	err := m.db.QueryRowContext(ctx, query, project.Name).Scan(&project.ProjectID, &project.CreatedAT)

	if err != nil {
		return err
	}
	return nil
}

func (m ProjectModel) Get(project_id int64) (*Project, error) {
	query := `select project_id,name,created_at from projects
	where project_id=$1`

	project := Project{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	defer cancel()

	err := m.db.QueryRowContext(ctx, query, project_id).Scan(&project.ProjectID, &project.Name, &project.CreatedAT)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}
	return &project, nil
}
