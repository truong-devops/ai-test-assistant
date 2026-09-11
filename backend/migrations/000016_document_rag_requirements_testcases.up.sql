CREATE TABLE document_index_status (
    document_set_id BIGINT PRIMARY KEY REFERENCES document_sets(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'NOT_INDEXED' CHECK (status IN
        ('NOT_INDEXED', 'INDEXING', 'READY', 'FAILED')),
    generation BIGINT NOT NULL DEFAULT 0 CHECK (generation >= 0),
    input_fingerprint TEXT NOT NULL DEFAULT '',
    version_count INTEGER NOT NULL DEFAULT 0 CHECK (version_count >= 0),
    skipped_version_count INTEGER NOT NULL DEFAULT 0 CHECK (skipped_version_count >= 0),
    chunk_count INTEGER NOT NULL DEFAULT 0 CHECK (chunk_count >= 0),
    warning_count INTEGER NOT NULL DEFAULT 0 CHECK (warning_count >= 0),
    embedding_model TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    requested_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE document_chunks
    ADD COLUMN document_block_id BIGINT,
    ADD COLUMN parent_chunk_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN flow_type TEXT NOT NULL DEFAULT 'NONE' CHECK (flow_type IN
        ('NONE', 'MAIN', 'ALTERNATE', 'EXCEPTION')),
    ADD COLUMN raw_content TEXT NOT NULL DEFAULT '';

ALTER TABLE document_chunks
    ADD CONSTRAINT document_chunks_block_version_fk
    FOREIGN KEY (document_block_id, document_version_id)
    REFERENCES document_blocks(id, document_version_id) ON DELETE CASCADE;

CREATE INDEX document_chunks_set_identifier_idx
    ON document_chunks (document_set_id, lower(identifier)) WHERE identifier <> '';
CREATE INDEX document_chunks_set_version_type_idx
    ON document_chunks (document_set_id, document_version_id, chunk_type, flow_type);

CREATE TABLE document_chunk_source_blocks (
    document_chunk_id BIGINT NOT NULL REFERENCES document_chunks(id) ON DELETE CASCADE,
    document_version_id BIGINT NOT NULL REFERENCES document_versions(id) ON DELETE CASCADE,
    document_block_id BIGINT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal > 0),
    PRIMARY KEY (document_chunk_id, document_block_id),
    FOREIGN KEY (document_block_id, document_version_id)
        REFERENCES document_blocks(id, document_version_id) ON DELETE CASCADE
);

CREATE TABLE document_context_snapshots (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    purpose TEXT NOT NULL,
    query_text TEXT NOT NULL,
    version_policy TEXT NOT NULL CHECK (version_policy IN
        ('LATEST', 'LATEST_APPROVED', 'ALL_INDEXED')),
    index_generation BIGINT NOT NULL CHECK (index_generation >= 0),
    embedding_model TEXT NOT NULL,
    retrieval_config JSONB NOT NULL DEFAULT '{}'::jsonb CHECK
        (jsonb_typeof(retrieval_config) = 'object'),
    snapshot_hash TEXT NOT NULL CHECK (snapshot_hash ~ '^[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (id, document_set_id)
);

CREATE TABLE document_context_snapshot_items (
    id BIGSERIAL PRIMARY KEY,
    context_snapshot_id BIGINT NOT NULL REFERENCES document_context_snapshots(id) ON DELETE CASCADE,
    document_set_id BIGINT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal > 0),
    document_chunk_id BIGINT NOT NULL,
    document_version_id BIGINT NOT NULL,
    chunk_key TEXT NOT NULL,
    chunk_type TEXT NOT NULL,
    identifier TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL,
    content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
    source_locator TEXT NOT NULL,
    approval_status TEXT NOT NULL CHECK (approval_status IN ('DRAFT', 'APPROVED', 'REJECTED')),
    exact_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    lexical_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    semantic_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    authority_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    total_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (context_snapshot_id, document_set_id)
        REFERENCES document_context_snapshots(id, document_set_id) ON DELETE CASCADE,
    FOREIGN KEY (document_chunk_id, document_set_id)
        REFERENCES document_chunks(id, document_set_id) ON DELETE CASCADE,
    FOREIGN KEY (document_version_id, document_set_id)
        REFERENCES document_versions(id, document_set_id) ON DELETE CASCADE,
    UNIQUE (context_snapshot_id, ordinal)
);

CREATE TABLE document_ai_calls (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    phase TEXT NOT NULL CHECK (phase IN ('REQUIREMENT_EXTRACTION', 'TEST_CASE_GENERATION')),
    subject_type TEXT NOT NULL,
    subject_key TEXT NOT NULL,
    context_snapshot_id BIGINT,
    provider TEXT NOT NULL,
    model_name TEXT NOT NULL,
    prompt_version TEXT NOT NULL,
    instructions TEXT NOT NULL,
    prompt_text TEXT NOT NULL,
    request_schema JSONB NOT NULL CHECK (jsonb_typeof(request_schema) = 'object'),
    response_text TEXT NOT NULL DEFAULT '',
    provider_response_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('COMPLETED', 'FAILED', 'INVALID_OUTPUT', 'DETERMINISTIC')),
    error_message TEXT NOT NULL DEFAULT '',
    input_tokens INTEGER NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens INTEGER NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    latency_ms BIGINT NOT NULL DEFAULT 0 CHECK (latency_ms >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (context_snapshot_id, document_set_id)
        REFERENCES document_context_snapshots(id, document_set_id) ON DELETE CASCADE
);

CREATE INDEX document_ai_calls_set_phase_idx
    ON document_ai_calls (document_set_id, phase, created_at DESC);

CREATE TABLE document_version_reviews (
    id BIGSERIAL PRIMARY KEY,
    document_version_id BIGINT NOT NULL REFERENCES document_versions(id) ON DELETE CASCADE,
    reviewer_name TEXT NOT NULL,
    decision TEXT NOT NULL CHECK (decision IN ('APPROVED', 'REJECTED')),
    comment TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX document_version_reviews_version_idx
    ON document_version_reviews (document_version_id, created_at DESC, id DESC);

CREATE OR REPLACE FUNCTION enforce_document_version_approval_dependencies()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.approval_status = 'APPROVED' AND OLD.approval_status IS DISTINCT FROM 'APPROVED'
       AND NEW.parse_status <> 'PARSED' THEN
        RAISE EXCEPTION 'document versions must be parsed before approval';
    END IF;
    IF OLD.approval_status = 'APPROVED' AND NEW.approval_status IS DISTINCT FROM 'APPROVED'
       AND EXISTS (
           SELECT 1 FROM requirement_evidence e
           JOIN requirements r ON r.id=e.requirement_id
           WHERE e.document_version_id=OLD.id AND r.status='APPROVED'
       ) THEN
        RAISE EXCEPTION 'approved document evidence is in use; version cannot be revoked';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER document_versions_approval_dependencies
BEFORE UPDATE ON document_versions
FOR EACH ROW EXECUTE FUNCTION enforce_document_version_approval_dependencies();

ALTER TABLE requirements
    ADD COLUMN risk TEXT NOT NULL DEFAULT 'MEDIUM' CHECK (risk IN ('LOW', 'MEDIUM', 'HIGH')),
    ADD COLUMN extraction_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN source_fingerprint TEXT NOT NULL DEFAULT '',
    ADD COLUMN assumptions JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(assumptions) = 'array'),
    ADD COLUMN raw_payload JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(raw_payload) = 'object');

CREATE UNIQUE INDEX requirements_extraction_key_idx
    ON requirements (document_set_id, extraction_key) WHERE extraction_key <> '';
CREATE INDEX requirements_set_actor_flow_idx
    ON requirements (document_set_id, actor, flow_type, status);

CREATE TABLE requirement_flow_steps (
    id BIGSERIAL PRIMARY KEY,
    requirement_id BIGINT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL CHECK (ordinal > 0),
    action TEXT NOT NULL,
    expected_result TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (requirement_id, ordinal)
);

CREATE UNIQUE INDEX open_questions_requirement_question_idx
    ON open_questions (document_set_id, requirement_id, question);

ALTER TABLE requirement_reviews
    DROP CONSTRAINT requirement_reviews_requirement_id_key;
CREATE INDEX requirement_reviews_requirement_idx
    ON requirement_reviews (requirement_id, created_at DESC, id DESC);

ALTER TABLE test_cases
    ADD COLUMN assumptions JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(assumptions) = 'array'),
    ADD COLUMN generation_key TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX test_cases_generation_key_idx
    ON test_cases (document_set_id, generation_key, version_number) WHERE generation_key <> '';

CREATE TABLE test_case_dedupe_records (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    test_suite_id BIGINT NOT NULL,
    kept_test_case_id BIGINT REFERENCES test_cases(id) ON DELETE CASCADE,
    suppressed_key TEXT NOT NULL,
    match_type TEXT NOT NULL CHECK (match_type IN ('EXACT', 'SEMANTIC')),
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (test_suite_id, document_set_id)
        REFERENCES test_suites(id, document_set_id) ON DELETE CASCADE,
    UNIQUE (test_suite_id, suppressed_key)
);

ALTER TABLE test_case_reviews
    DROP CONSTRAINT test_case_reviews_test_case_id_key;
CREATE INDEX test_case_reviews_case_idx
    ON test_case_reviews (test_case_id, created_at DESC, id DESC);

CREATE OR REPLACE FUNCTION prevent_context_snapshot_update()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% rows are immutable', TG_TABLE_NAME;
END;
$$;

CREATE TRIGGER document_context_snapshots_immutable
BEFORE UPDATE ON document_context_snapshots
FOR EACH ROW EXECUTE FUNCTION prevent_context_snapshot_update();

CREATE TRIGGER document_context_snapshot_items_immutable
BEFORE UPDATE ON document_context_snapshot_items
FOR EACH ROW EXECUTE FUNCTION prevent_context_snapshot_update();

CREATE OR REPLACE FUNCTION enforce_requirement_approval_evidence()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.status = 'APPROVED' AND OLD.status IS DISTINCT FROM 'APPROVED' THEN
        IF NOT EXISTS (
            SELECT 1 FROM requirement_evidence e
            JOIN document_versions v ON v.id=e.document_version_id
            WHERE e.requirement_id=NEW.id AND v.approval_status='APPROVED'
        ) THEN
            RAISE EXCEPTION 'approved requirements require evidence from an approved document version';
        END IF;
    END IF;
    IF OLD.status = 'APPROVED' AND (
        NEW.requirement_key IS DISTINCT FROM OLD.requirement_key OR
        NEW.title IS DISTINCT FROM OLD.title OR
        NEW.statement IS DISTINCT FROM OLD.statement OR
        NEW.requirement_type IS DISTINCT FROM OLD.requirement_type OR
        NEW.flow_type IS DISTINCT FROM OLD.flow_type OR
        NEW.actor IS DISTINCT FROM OLD.actor OR
        NEW.precondition IS DISTINCT FROM OLD.precondition OR
        NEW.postcondition IS DISTINCT FROM OLD.postcondition
    ) THEN
        RAISE EXCEPTION 'approved requirement content is immutable; create a new version';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER requirements_approval_evidence
BEFORE UPDATE ON requirements
FOR EACH ROW EXECUTE FUNCTION enforce_requirement_approval_evidence();

CREATE OR REPLACE FUNCTION enforce_test_case_approval_evidence()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.status = 'APPROVED' AND OLD.status IS DISTINCT FROM 'APPROVED' THEN
        IF NOT EXISTS (
            SELECT 1 FROM test_case_requirement_links l
            JOIN requirements r ON r.id=l.requirement_id
            JOIN requirement_evidence e ON e.requirement_id=r.id
            JOIN document_versions v ON v.id=e.document_version_id
            WHERE l.test_case_id=NEW.id AND r.status='APPROVED'
              AND v.approval_status='APPROVED'
        ) THEN
            RAISE EXCEPTION 'approved test cases require an approved requirement with evidence';
        END IF;
    END IF;
    IF OLD.status = 'APPROVED' AND (
        NEW.test_case_key IS DISTINCT FROM OLD.test_case_key OR
        NEW.title IS DISTINCT FROM OLD.title OR
        NEW.test_type IS DISTINCT FROM OLD.test_type OR
        NEW.actor IS DISTINCT FROM OLD.actor OR
        NEW.precondition IS DISTINCT FROM OLD.precondition OR
        NEW.test_data IS DISTINCT FROM OLD.test_data OR
        NEW.expected_result IS DISTINCT FROM OLD.expected_result OR
        NEW.postcondition IS DISTINCT FROM OLD.postcondition
    ) THEN
        RAISE EXCEPTION 'approved test case content is immutable; create a new version';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER test_cases_approval_evidence
BEFORE UPDATE ON test_cases
FOR EACH ROW EXECUTE FUNCTION enforce_test_case_approval_evidence();
