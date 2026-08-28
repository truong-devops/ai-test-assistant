package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("evaluation run not found")

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) Save(ctx context.Context, dataset Dataset, report Report) (Run, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Run{}, fmt.Errorf("begin save evaluation run: %w", err)
	}
	defer tx.Rollback(ctx)

	const insertRun = `INSERT INTO evaluation_runs
		(name, schema_version, dataset_hash, description, observation_count)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (dataset_hash) DO NOTHING
		RETURNING id, name, schema_version, dataset_hash, description, observation_count, created_at`
	var result Run
	err = tx.QueryRow(ctx, insertRun, dataset.Name, dataset.SchemaVersion, report.DatasetHash,
		dataset.Description, len(dataset.Observations)).Scan(runDestinations(&result)...)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id, name, schema_version, dataset_hash, description,
			observation_count, created_at FROM evaluation_runs WHERE dataset_hash=$1`,
			report.DatasetHash).Scan(runDestinations(&result)...)
		if err != nil {
			return Run{}, fmt.Errorf("get existing evaluation run: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return Run{}, fmt.Errorf("commit existing evaluation run: %w", err)
		}
		return result, nil
	}
	if err != nil {
		return Run{}, fmt.Errorf("insert evaluation run: %w", err)
	}

	const insertObservation = `INSERT INTO evaluation_observations
		(evaluation_run_id, ordinal, observation_key, experiment, variant, scenario, replicate, payload)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`
	for index, observation := range dataset.Observations {
		payload, err := json.Marshal(observation)
		if err != nil {
			return Run{}, fmt.Errorf("encode evaluation observation %q: %w", observation.Key, err)
		}
		if _, err := tx.Exec(ctx, insertObservation, result.ID, index+1, observation.Key,
			observation.Experiment, observation.Variant, observation.Scenario,
			observation.Replicate, payload); err != nil {
			return Run{}, fmt.Errorf("insert evaluation observation %q: %w", observation.Key, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Run{}, fmt.Errorf("commit evaluation run: %w", err)
	}
	return result, nil
}

func (r *Repository) List(ctx context.Context) ([]Run, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, schema_version, dataset_hash,
		description, observation_count, created_at
		FROM evaluation_runs ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list evaluation runs: %w", err)
	}
	defer rows.Close()
	results := make([]Run, 0)
	for rows.Next() {
		var item Run
		if err := rows.Scan(runDestinations(&item)...); err != nil {
			return nil, fmt.Errorf("scan evaluation run: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate evaluation runs: %w", err)
	}
	return results, nil
}

func (r *Repository) Get(ctx context.Context, id int64) (Run, Dataset, error) {
	var run Run
	err := r.pool.QueryRow(ctx, `SELECT id, name, schema_version, dataset_hash,
		description, observation_count, created_at FROM evaluation_runs WHERE id=$1`, id).
		Scan(runDestinations(&run)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, Dataset{}, ErrNotFound
	}
	if err != nil {
		return Run{}, Dataset{}, fmt.Errorf("get evaluation run: %w", err)
	}
	rows, err := r.pool.Query(ctx, `SELECT payload FROM evaluation_observations
		WHERE evaluation_run_id=$1 ORDER BY ordinal`, id)
	if err != nil {
		return Run{}, Dataset{}, fmt.Errorf("list evaluation observations: %w", err)
	}
	defer rows.Close()
	dataset := Dataset{SchemaVersion: run.SchemaVersion, Name: run.Name, Description: run.Description,
		Observations: make([]Observation, 0, run.ObservationCount)}
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return Run{}, Dataset{}, fmt.Errorf("scan evaluation observation: %w", err)
		}
		var observation Observation
		if err := json.Unmarshal(payload, &observation); err != nil {
			return Run{}, Dataset{}, fmt.Errorf("decode evaluation observation: %w", err)
		}
		dataset.Observations = append(dataset.Observations, observation)
	}
	if err := rows.Err(); err != nil {
		return Run{}, Dataset{}, fmt.Errorf("iterate evaluation observations: %w", err)
	}
	if len(dataset.Observations) != run.ObservationCount {
		return Run{}, Dataset{}, fmt.Errorf("evaluation run %d has %d observations, want %d",
			id, len(dataset.Observations), run.ObservationCount)
	}
	return run, dataset, nil
}

func runDestinations(item *Run) []any {
	return []any{&item.ID, &item.Name, &item.SchemaVersion, &item.DatasetHash,
		&item.Description, &item.ObservationCount, &item.CreatedAt}
}
