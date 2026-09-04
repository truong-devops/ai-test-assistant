CREATE TABLE impact_analysis_runs (
    id BIGSERIAL PRIMARY KEY,
    analysis_job_id BIGINT NOT NULL UNIQUE REFERENCES analysis_jobs(id) ON DELETE CASCADE,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    source_sha TEXT NOT NULL,
    mode TEXT NOT NULL CHECK (mode IN ('SSA', 'AST_FALLBACK')),
    algorithm TEXT NOT NULL,
    max_depth INTEGER NOT NULL CHECK (max_depth BETWEEN 1 AND 20),
    max_nodes INTEGER NOT NULL CHECK (max_nodes BETWEEN 1 AND 10000),
    package_count INTEGER NOT NULL DEFAULT 0 CHECK (package_count >= 0),
    fallback_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (id, project_id)
);

CREATE INDEX impact_analysis_runs_project_created_idx
    ON impact_analysis_runs (project_id, created_at DESC, id DESC);

CREATE TABLE impact_nodes (
    id BIGSERIAL PRIMARY KEY,
    impact_run_id BIGINT NOT NULL REFERENCES impact_analysis_runs(id) ON DELETE CASCADE,
    project_id BIGINT NOT NULL,
    stable_key TEXT NOT NULL,
    package_path TEXT NOT NULL,
    package_name TEXT NOT NULL,
    symbol_name TEXT NOT NULL,
    receiver_name TEXT NOT NULL DEFAULT '',
    symbol_kind TEXT NOT NULL,
    file_path TEXT NOT NULL,
    start_line INTEGER NOT NULL DEFAULT 0 CHECK (start_line >= 0),
    end_line INTEGER NOT NULL DEFAULT 0 CHECK (end_line >= start_line),
    direct_change BOOLEAN NOT NULL DEFAULT FALSE,
    existing_test BOOLEAN NOT NULL DEFAULT FALSE,
    depth INTEGER NOT NULL CHECK (depth >= 0),
    score DOUBLE PRECISION NOT NULL CHECK (score >= 0 AND score <= 1),
    reason_codes JSONB NOT NULL CHECK (jsonb_typeof(reason_codes) = 'array'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (impact_run_id, stable_key),
    UNIQUE (id, impact_run_id),
    FOREIGN KEY (impact_run_id, project_id)
        REFERENCES impact_analysis_runs(id, project_id) ON DELETE CASCADE
);

CREATE INDEX impact_nodes_run_score_idx
    ON impact_nodes (impact_run_id, direct_change DESC, score DESC, id);

CREATE TABLE impact_edges (
    id BIGSERIAL PRIMARY KEY,
    impact_run_id BIGINT NOT NULL REFERENCES impact_analysis_runs(id) ON DELETE CASCADE,
    from_node_id BIGINT NOT NULL,
    to_node_id BIGINT NOT NULL,
    relation TEXT NOT NULL CHECK (relation IN ('CALLS', 'IMPLEMENTS', 'USES_TYPE')),
    reason_code TEXT NOT NULL CHECK (reason_code IN
        ('CALLER', 'CALLEE', 'INTERFACE_IMPLEMENTATION', 'TYPE_USAGE', 'EXISTING_TEST')),
    depth INTEGER NOT NULL CHECK (depth BETWEEN 1 AND 20),
    score DOUBLE PRECISION NOT NULL CHECK (score >= 0 AND score <= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (impact_run_id, from_node_id, to_node_id, reason_code),
    FOREIGN KEY (from_node_id, impact_run_id)
        REFERENCES impact_nodes(id, impact_run_id) ON DELETE CASCADE,
    FOREIGN KEY (to_node_id, impact_run_id)
        REFERENCES impact_nodes(id, impact_run_id) ON DELETE CASCADE
);

CREATE INDEX impact_edges_run_depth_idx
    ON impact_edges (impact_run_id, depth, id);
