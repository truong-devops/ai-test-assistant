CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE project_indexes (
    project_id BIGINT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    ref TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('PENDING', 'INDEXING', 'READY', 'FAILED')),
    generation BIGINT NOT NULL DEFAULT 1 CHECK (generation > 0),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    file_count INTEGER NOT NULL DEFAULT 0 CHECK (file_count >= 0),
    skipped_file_count INTEGER NOT NULL DEFAULT 0 CHECK (skipped_file_count >= 0),
    chunk_count INTEGER NOT NULL DEFAULT 0 CHECK (chunk_count >= 0),
    embedding_model TEXT NOT NULL DEFAULT '',
    error_message TEXT,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_expires_at TIMESTAMPTZ,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX project_indexes_claim_idx
    ON project_indexes (status, next_attempt_at, requested_at);

CREATE TABLE knowledge_chunks (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    chunk_key TEXT NOT NULL,
    file_path TEXT NOT NULL,
    package_name TEXT NOT NULL DEFAULT '',
    symbol_name TEXT NOT NULL DEFAULT '',
    chunk_type TEXT NOT NULL,
    content TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    start_line INTEGER NOT NULL CHECK (start_line > 0),
    end_line INTEGER NOT NULL CHECK (end_line >= start_line),
    embedding_model TEXT NOT NULL,
    embedding VECTOR(384) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    search_vector TSVECTOR GENERATED ALWAYS AS (
        to_tsvector('simple', file_path || ' ' || package_name || ' ' || symbol_name || ' ' || content)
    ) STORED,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, chunk_key)
);

CREATE INDEX knowledge_chunks_project_id_idx ON knowledge_chunks (project_id);
CREATE INDEX knowledge_chunks_search_idx ON knowledge_chunks USING GIN (search_vector);
CREATE INDEX knowledge_chunks_embedding_hnsw_idx
    ON knowledge_chunks USING HNSW (embedding vector_cosine_ops);
