package project

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound      = errors.New("project not found")
	ErrAlreadyExists = errors.New("project already exists")
)

type Repository interface {
	Create(ctx context.Context, input CreateInput) (Project, error)
	List(ctx context.Context) ([]Project, error)
	GetByID(ctx context.Context, id int64) (Project, error)
	GetByGitLabProjectID(ctx context.Context, gitLabProjectID int64) (Project, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, input CreateInput) (Project, error) {
	const query = `
		INSERT INTO projects (name, gitlab_project_id, repository_url, default_branch, language, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, gitlab_project_id, repository_url, default_branch, language, status, created_at, updated_at`

	var result Project
	err := r.pool.QueryRow(ctx, query, input.Name, input.GitLabProjectID, input.RepositoryURL,
		input.DefaultBranch, input.Language, StatusActive).Scan(projectDestinations(&result)...)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return Project{}, ErrAlreadyExists
		}
		return Project{}, fmt.Errorf("insert project: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) List(ctx context.Context) ([]Project, error) {
	const query = `
		SELECT id, name, gitlab_project_id, repository_url, default_branch, language, status, created_at, updated_at
		FROM projects ORDER BY id`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()

	projects := make([]Project, 0)
	for rows.Next() {
		var item Project
		if err := rows.Scan(projectDestinations(&item)...); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projects: %w", err)
	}
	return projects, nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id int64) (Project, error) {
	const query = `
		SELECT id, name, gitlab_project_id, repository_url, default_branch, language, status, created_at, updated_at
		FROM projects WHERE id = $1`
	var result Project
	if err := r.pool.QueryRow(ctx, query, id).Scan(projectDestinations(&result)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Project{}, ErrNotFound
		}
		return Project{}, fmt.Errorf("query project %d: %w", id, err)
	}
	return result, nil
}

func (r *PostgresRepository) GetByGitLabProjectID(ctx context.Context, gitLabProjectID int64) (Project, error) {
	const query = `
		SELECT id, name, gitlab_project_id, repository_url, default_branch, language, status, created_at, updated_at
		FROM projects WHERE gitlab_project_id = $1`
	var result Project
	if err := r.pool.QueryRow(ctx, query, gitLabProjectID).Scan(projectDestinations(&result)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Project{}, ErrNotFound
		}
		return Project{}, fmt.Errorf("query project by GitLab id %d: %w", gitLabProjectID, err)
	}
	return result, nil
}

func projectDestinations(item *Project) []any {
	return []any{&item.ID, &item.Name, &item.GitLabProjectID, &item.RepositoryURL,
		&item.DefaultBranch, &item.Language, &item.Status, &item.CreatedAt, &item.UpdatedAt}
}
