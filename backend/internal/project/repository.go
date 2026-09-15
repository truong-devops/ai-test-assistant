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
	GetByProviderProjectID(ctx context.Context, provider string, providerProjectID int64) (Project, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, input CreateInput) (Project, error) {
	const query = `
		INSERT INTO projects (name, provider, provider_project_id, repository_url, default_branch, language, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, name, provider, provider_project_id, repository_url, default_branch, language, status, pipeline_mode, created_at, updated_at`

	var result Project
	err := r.pool.QueryRow(ctx, query, input.Name, input.Provider, input.ProviderProjectID, input.RepositoryURL,
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
		SELECT id, name, provider, provider_project_id, repository_url, default_branch, language, status, pipeline_mode, created_at, updated_at
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
		SELECT id, name, provider, provider_project_id, repository_url, default_branch, language, status, pipeline_mode, created_at, updated_at
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
	return r.GetByProviderProjectID(ctx, "gitlab", gitLabProjectID)
}

func (r *PostgresRepository) GetByProviderProjectID(ctx context.Context, provider string, providerProjectID int64) (Project, error) {
	const query = `
		SELECT id, name, provider, provider_project_id, repository_url, default_branch, language, status, pipeline_mode, created_at, updated_at
		FROM projects WHERE provider = $1 AND provider_project_id = $2`
	var result Project
	if err := r.pool.QueryRow(ctx, query, provider, providerProjectID).Scan(projectDestinations(&result)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Project{}, ErrNotFound
		}
		return Project{}, fmt.Errorf("query project by %s id %d: %w", provider, providerProjectID, err)
	}
	return result, nil
}

func (r *PostgresRepository) SetPipelineMode(ctx context.Context, id int64, input PipelineModeInput) (Project, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback(ctx)
	var previous string
	if err = tx.QueryRow(ctx, `SELECT pipeline_mode FROM projects WHERE id=$1 FOR UPDATE`, id).Scan(&previous); errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	} else if err != nil {
		return Project{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE projects SET pipeline_mode=$2,updated_at=NOW() WHERE id=$1`, id, input.Mode); err != nil {
		return Project{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO project_pipeline_mode_audit(project_id,actor,previous_mode,new_mode,reason)
		VALUES($1,$2,$3,$4,$5)`, id, input.Actor, previous, input.Mode, input.Reason); err != nil {
		return Project{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Project{}, err
	}
	return r.GetByID(ctx, id)
}

func projectDestinations(item *Project) []any {
	return []any{&item.ID, &item.Name, &item.Provider, &item.ProviderProjectID, &item.RepositoryURL,
		&item.DefaultBranch, &item.Language, &item.Status, &item.PipelineMode, &item.CreatedAt, &item.UpdatedAt}
}
