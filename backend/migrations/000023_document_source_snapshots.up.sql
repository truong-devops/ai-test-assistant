ALTER TABLE document_sets
    ADD COLUMN source_revision BIGINT NOT NULL DEFAULT 0 CHECK (source_revision >= 0);

UPDATE document_sets s SET source_revision=(
    SELECT count(*) FROM document_versions v WHERE v.document_set_id=s.id
);

CREATE OR REPLACE FUNCTION bump_document_set_source_revision()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE document_sets SET source_revision=source_revision+1,updated_at=NOW()
    WHERE id=NEW.document_set_id;
    RETURN NEW;
END;
$$;

CREATE TRIGGER document_versions_bump_source_revision
AFTER INSERT ON document_versions
FOR EACH ROW EXECUTE FUNCTION bump_document_set_source_revision();

CREATE TABLE document_source_snapshots (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    source_revision BIGINT NOT NULL CHECK (source_revision >= 0),
    fingerprint TEXT NOT NULL CHECK (fingerprint ~ '^[0-9a-f]{64}$'),
    included_count INTEGER NOT NULL CHECK (included_count >= 0),
    excluded_count INTEGER NOT NULL CHECK (excluded_count >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (document_set_id, source_revision, fingerprint),
    UNIQUE (id, document_set_id)
);

CREATE TABLE document_source_snapshot_items (
    id BIGSERIAL PRIMARY KEY,
    source_snapshot_id BIGINT NOT NULL,
    document_set_id BIGINT NOT NULL,
    document_id BIGINT NOT NULL,
    document_name TEXT NOT NULL,
    document_version_id BIGINT NOT NULL,
    version_number INTEGER NOT NULL CHECK (version_number > 0),
    sha256 TEXT NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    parse_status TEXT NOT NULL CHECK (parse_status IN ('UPLOADED','PARSING','PARSED','FAILED')),
    approval_status TEXT NOT NULL CHECK (approval_status IN ('DRAFT','APPROVED','REJECTED')),
    included BOOLEAN NOT NULL,
    exclusion_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (source_snapshot_id, document_set_id)
        REFERENCES document_source_snapshots(id, document_set_id) ON DELETE CASCADE,
    FOREIGN KEY (document_id, document_set_id)
        REFERENCES documents(id, document_set_id) ON DELETE RESTRICT,
    FOREIGN KEY (document_version_id, document_set_id)
        REFERENCES document_versions(id, document_set_id) ON DELETE RESTRICT,
    UNIQUE (source_snapshot_id, document_id),
    CHECK (included OR exclusion_reason <> '')
);

CREATE TRIGGER document_source_snapshots_immutable
BEFORE UPDATE ON document_source_snapshots
FOR EACH ROW EXECUTE FUNCTION prevent_context_snapshot_update();

CREATE TRIGGER document_source_snapshot_items_immutable
BEFORE UPDATE ON document_source_snapshot_items
FOR EACH ROW EXECUTE FUNCTION prevent_context_snapshot_update();

ALTER TABLE document_index_status
    ADD COLUMN source_snapshot_id BIGINT REFERENCES document_source_snapshots(id) ON DELETE RESTRICT,
    ADD COLUMN indexed_source_revision BIGINT NOT NULL DEFAULT 0 CHECK (indexed_source_revision >= 0),
    ADD COLUMN content_warning_count INTEGER NOT NULL DEFAULT 0 CHECK (content_warning_count >= 0);

CREATE TABLE document_index_generations (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    generation BIGINT NOT NULL CHECK (generation > 0),
    source_snapshot_id BIGINT,
    source_revision BIGINT NOT NULL DEFAULT 0 CHECK (source_revision >= 0),
    input_fingerprint TEXT NOT NULL DEFAULT '',
    embedding_model TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('INDEXING','READY','FAILED','SUPERSEDED')),
    version_count INTEGER NOT NULL DEFAULT 0 CHECK (version_count >= 0),
    excluded_version_count INTEGER NOT NULL DEFAULT 0 CHECK (excluded_version_count >= 0),
    chunk_count INTEGER NOT NULL DEFAULT 0 CHECK (chunk_count >= 0),
    content_warning_count INTEGER NOT NULL DEFAULT 0 CHECK (content_warning_count >= 0),
    error_message TEXT NOT NULL DEFAULT '',
    requested_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (source_snapshot_id, document_set_id)
        REFERENCES document_source_snapshots(id, document_set_id) ON DELETE RESTRICT,
    UNIQUE (document_set_id, generation)
);

CREATE INDEX document_index_generations_set_history_idx
    ON document_index_generations (document_set_id, generation DESC);

CREATE TABLE document_index_generation_chunks (
    document_set_id BIGINT NOT NULL,
    generation BIGINT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal > 0),
    document_chunk_id BIGINT NOT NULL,
    document_version_id BIGINT NOT NULL,
    PRIMARY KEY (document_set_id, generation, document_chunk_id),
    UNIQUE (document_set_id, generation, ordinal),
    FOREIGN KEY (document_set_id, generation)
        REFERENCES document_index_generations(document_set_id, generation) ON DELETE CASCADE,
    FOREIGN KEY (document_chunk_id, document_set_id)
        REFERENCES document_chunks(id, document_set_id) ON DELETE CASCADE,
    FOREIGN KEY (document_version_id, document_set_id)
        REFERENCES document_versions(id, document_set_id) ON DELETE RESTRICT
);

INSERT INTO document_index_generations
    (document_set_id,generation,source_revision,input_fingerprint,embedding_model,status,
     version_count,excluded_version_count,chunk_count,content_warning_count,error_message,
     requested_at,started_at,finished_at,created_at,updated_at)
SELECT document_set_id,generation,0,input_fingerprint,embedding_model,
    CASE WHEN status='NOT_INDEXED' THEN 'FAILED' ELSE status END,
    version_count,skipped_version_count,chunk_count,warning_count,error_message,
    requested_at,started_at,finished_at,updated_at,updated_at
FROM document_index_status WHERE generation > 0;

INSERT INTO document_index_generation_chunks
    (document_set_id,generation,ordinal,document_chunk_id,document_version_id)
SELECT s.document_set_id,s.generation,
    row_number() OVER (PARTITION BY s.document_set_id,s.generation ORDER BY c.document_version_id,c.id),
    c.id,c.document_version_id
FROM document_index_status s JOIN document_chunks c ON c.document_set_id=s.document_set_id
WHERE s.generation > 0;

ALTER TABLE document_context_snapshots
    ADD COLUMN source_snapshot_id BIGINT REFERENCES document_source_snapshots(id) ON DELETE RESTRICT;

ALTER TABLE requirement_extraction_jobs
    ADD COLUMN source_snapshot_id BIGINT REFERENCES document_source_snapshots(id) ON DELETE RESTRICT,
    ADD COLUMN source_revision BIGINT NOT NULL DEFAULT 0 CHECK (source_revision >= 0),
    ADD COLUMN is_current BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE requirements
    ADD COLUMN source_snapshot_id BIGINT REFERENCES document_source_snapshots(id) ON DELETE RESTRICT;

CREATE INDEX requirements_source_snapshot_idx
    ON requirements (source_snapshot_id) WHERE source_snapshot_id IS NOT NULL;
