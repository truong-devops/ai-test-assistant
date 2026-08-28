CREATE TABLE evaluation_runs (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    schema_version TEXT NOT NULL CHECK (schema_version = 'evaluation-v1'),
    dataset_hash TEXT NOT NULL UNIQUE CHECK (length(dataset_hash) = 64),
    description TEXT NOT NULL DEFAULT '' CHECK (length(description) <= 4000),
    observation_count INTEGER NOT NULL CHECK (observation_count > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE evaluation_observations (
    id BIGSERIAL PRIMARY KEY,
    evaluation_run_id BIGINT NOT NULL REFERENCES evaluation_runs(id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL CHECK (ordinal > 0),
    observation_key TEXT NOT NULL CHECK (length(observation_key) BETWEEN 1 AND 200),
    experiment TEXT NOT NULL CHECK (
        experiment IN ('CONTEXT_IMPACT', 'REPAIR_IMPACT', 'HUMAN_EFFORT')
    ),
    variant TEXT NOT NULL CHECK (
        (experiment = 'CONTEXT_IMPACT' AND variant IN ('DIFF_ONLY', 'DIFF_RAG'))
        OR (experiment = 'REPAIR_IMPACT' AND variant IN ('GENERATE_ONLY', 'GENERATE_REPAIR'))
        OR (experiment = 'HUMAN_EFFORT' AND variant IN ('MANUAL', 'AI_ASSISTED'))
    ),
    scenario TEXT NOT NULL CHECK (length(scenario) BETWEEN 1 AND 500),
    replicate INTEGER NOT NULL CHECK (replicate > 0),
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (evaluation_run_id, observation_key),
    UNIQUE (evaluation_run_id, ordinal)
);

CREATE INDEX evaluation_observations_run_variant_idx
    ON evaluation_observations (evaluation_run_id, experiment, variant, scenario, replicate);
