CREATE TABLE document_sets (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    product_name TEXT NOT NULL DEFAULT '',
    scope TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'ARCHIVED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (name)
);

CREATE TABLE documents (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    document_type TEXT NOT NULL CHECK (document_type IN
        ('REQUIREMENTS', 'USER_STORY', 'SYSTEM_DESIGN', 'DATABASE_DESIGN',
         'API_CONTRACT', 'BUG_HISTORY', 'TEST_REFERENCE', 'OTHER')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (document_set_id, name),
    UNIQUE (id, document_set_id)
);

CREATE TABLE document_versions (
    id BIGSERIAL PRIMARY KEY,
    document_id BIGINT NOT NULL,
    document_set_id BIGINT NOT NULL,
    version_number INTEGER NOT NULL CHECK (version_number > 0),
    original_filename TEXT NOT NULL,
    media_type TEXT NOT NULL CHECK (media_type IN
        ('text/markdown',
         'application/vnd.openxmlformats-officedocument.wordprocessingml.document')),
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    sha256 TEXT NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    storage_key TEXT NOT NULL UNIQUE,
    approval_status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (approval_status IN
        ('DRAFT', 'APPROVED', 'REJECTED')),
    parse_status TEXT NOT NULL DEFAULT 'UPLOADED' CHECK (parse_status IN
        ('UPLOADED', 'PARSING', 'PARSED', 'FAILED')),
    parse_error TEXT NOT NULL DEFAULT '',
    block_count INTEGER NOT NULL DEFAULT 0 CHECK (block_count >= 0),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_expires_at TIMESTAMPTZ,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    parsed_at TIMESTAMPTZ,
    FOREIGN KEY (document_id, document_set_id)
        REFERENCES documents(id, document_set_id) ON DELETE CASCADE,
    UNIQUE (document_id, version_number),
    UNIQUE (document_id, sha256),
    UNIQUE (id, document_set_id)
);

CREATE INDEX document_versions_claim_idx
    ON document_versions (parse_status, next_attempt_at, uploaded_at);
CREATE INDEX document_versions_set_uploaded_idx
    ON document_versions (document_set_id, uploaded_at DESC, id DESC);

CREATE TABLE document_blocks (
    id BIGSERIAL PRIMARY KEY,
    document_version_id BIGINT NOT NULL REFERENCES document_versions(id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL CHECK (ordinal > 0),
    block_type TEXT NOT NULL CHECK (block_type IN
        ('HEADING', 'PARAGRAPH', 'LIST', 'TABLE', 'CODE')),
    heading_level INTEGER NOT NULL DEFAULT 0 CHECK (heading_level BETWEEN 0 AND 6),
    content TEXT NOT NULL CHECK (content <> ''),
    source_locator TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (document_version_id, ordinal),
    UNIQUE (id, document_version_id)
);

CREATE INDEX document_blocks_version_ordinal_idx
    ON document_blocks (document_version_id, ordinal);
CREATE INDEX document_blocks_source_locator_idx
    ON document_blocks (document_version_id, source_locator);

CREATE TABLE document_chunks (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    document_version_id BIGINT NOT NULL REFERENCES document_versions(id) ON DELETE CASCADE,
    chunk_key TEXT NOT NULL,
    chunk_type TEXT NOT NULL,
    identifier TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL CHECK (content <> ''),
    content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
    source_locator TEXT NOT NULL,
    embedding_model TEXT NOT NULL DEFAULT '',
    embedding VECTOR(384),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    search_vector TSVECTOR GENERATED ALWAYS AS (
        to_tsvector('simple', identifier || ' ' || title || ' ' || content)
    ) STORED,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (document_version_id, document_set_id)
        REFERENCES document_versions(id, document_set_id) ON DELETE CASCADE,
    UNIQUE (document_version_id, chunk_key),
    UNIQUE (id, document_set_id)
);

CREATE INDEX document_chunks_set_idx ON document_chunks (document_set_id);
CREATE INDEX document_chunks_search_idx ON document_chunks USING GIN (search_vector);
CREATE INDEX document_chunks_embedding_hnsw_idx
    ON document_chunks USING HNSW (embedding vector_cosine_ops) WHERE embedding IS NOT NULL;

CREATE TABLE requirements (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    requirement_key TEXT NOT NULL,
    version_number INTEGER NOT NULL DEFAULT 1 CHECK (version_number > 0),
    title TEXT NOT NULL,
    statement TEXT NOT NULL,
    requirement_type TEXT NOT NULL CHECK (requirement_type IN
        ('FUNCTIONAL', 'NON_FUNCTIONAL', 'BUSINESS_RULE', 'ACCEPTANCE_CRITERION',
         'USE_CASE', 'REGRESSION')),
    flow_type TEXT NOT NULL DEFAULT 'NONE' CHECK (flow_type IN
        ('NONE', 'MAIN', 'ALTERNATE', 'EXCEPTION')),
    actor TEXT NOT NULL DEFAULT '',
    precondition TEXT NOT NULL DEFAULT '',
    postcondition TEXT NOT NULL DEFAULT '',
    priority TEXT NOT NULL DEFAULT 'MEDIUM' CHECK (priority IN ('LOW', 'MEDIUM', 'HIGH')),
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN
        ('DRAFT', 'APPROVED', 'REJECTED', 'CONFLICT', 'TBD')),
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (confidence BETWEEN 0 AND 1),
    supersedes_requirement_id BIGINT REFERENCES requirements(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (document_set_id, requirement_key, version_number),
    UNIQUE (id, document_set_id)
);

CREATE INDEX requirements_set_status_idx
    ON requirements (document_set_id, status, requirement_type, id);

CREATE TABLE requirement_evidence (
    id BIGSERIAL PRIMARY KEY,
    requirement_id BIGINT NOT NULL,
    document_set_id BIGINT NOT NULL,
    document_version_id BIGINT NOT NULL,
    document_block_id BIGINT NOT NULL,
    source_locator TEXT NOT NULL,
    excerpt_hash TEXT NOT NULL CHECK (excerpt_hash ~ '^[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (requirement_id, document_set_id)
        REFERENCES requirements(id, document_set_id) ON DELETE CASCADE,
    FOREIGN KEY (document_version_id, document_set_id)
        REFERENCES document_versions(id, document_set_id) ON DELETE RESTRICT,
    FOREIGN KEY (document_block_id, document_version_id)
        REFERENCES document_blocks(id, document_version_id) ON DELETE RESTRICT,
    UNIQUE (requirement_id, document_block_id)
);

CREATE INDEX requirement_evidence_version_locator_idx
    ON requirement_evidence (document_version_id, source_locator);

CREATE TABLE requirement_conflicts (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    left_requirement_id BIGINT NOT NULL,
    right_requirement_id BIGINT NOT NULL,
    reason TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'RESOLVED', 'DISMISSED')),
    resolution TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    FOREIGN KEY (left_requirement_id, document_set_id)
        REFERENCES requirements(id, document_set_id) ON DELETE CASCADE,
    FOREIGN KEY (right_requirement_id, document_set_id)
        REFERENCES requirements(id, document_set_id) ON DELETE CASCADE,
    CHECK (left_requirement_id <> right_requirement_id),
    UNIQUE (document_set_id, left_requirement_id, right_requirement_id)
);

CREATE TABLE open_questions (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    requirement_id BIGINT,
    question TEXT NOT NULL,
    owner_role TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'ANSWERED', 'DISMISSED')),
    answer TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    answered_at TIMESTAMPTZ,
    FOREIGN KEY (requirement_id, document_set_id)
        REFERENCES requirements(id, document_set_id) ON DELETE CASCADE
);

CREATE TABLE requirement_reviews (
    id BIGSERIAL PRIMARY KEY,
    requirement_id BIGINT NOT NULL UNIQUE REFERENCES requirements(id) ON DELETE CASCADE,
    reviewer_name TEXT NOT NULL,
    decision TEXT NOT NULL CHECK (decision IN ('APPROVED', 'REJECTED')),
    comment TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE test_suites (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'APPROVED', 'ARCHIVED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (document_set_id, name),
    UNIQUE (id, document_set_id)
);

CREATE TABLE test_cases (
    id BIGSERIAL PRIMARY KEY,
    test_suite_id BIGINT NOT NULL,
    document_set_id BIGINT NOT NULL,
    test_case_key TEXT NOT NULL,
    version_number INTEGER NOT NULL DEFAULT 1 CHECK (version_number > 0),
    title TEXT NOT NULL,
    test_type TEXT NOT NULL CHECK (test_type IN
        ('HAPPY', 'NEGATIVE', 'BOUNDARY', 'PERMISSION', 'STATE', 'INTEGRATION',
         'REGRESSION', 'NFR')),
    risk TEXT NOT NULL DEFAULT 'MEDIUM' CHECK (risk IN ('LOW', 'MEDIUM', 'HIGH')),
    actor TEXT NOT NULL DEFAULT '',
    precondition TEXT NOT NULL DEFAULT '',
    test_data TEXT NOT NULL DEFAULT '',
    expected_result TEXT NOT NULL,
    expected_result_hash TEXT NOT NULL CHECK (expected_result_hash ~ '^[0-9a-f]{64}$'),
    postcondition TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'APPROVED', 'REJECTED')),
    automation_status TEXT NOT NULL DEFAULT 'MANUAL' CHECK (automation_status IN
        ('MANUAL', 'AUTOMATABLE', 'AUTOMATED', 'BLOCKED')),
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (confidence BETWEEN 0 AND 1),
    generated_by TEXT NOT NULL DEFAULT 'AI',
    supersedes_test_case_id BIGINT REFERENCES test_cases(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (test_suite_id, document_set_id)
        REFERENCES test_suites(id, document_set_id) ON DELETE CASCADE,
    UNIQUE (test_suite_id, test_case_key, version_number),
    UNIQUE (id, document_set_id)
);

CREATE INDEX test_cases_suite_status_idx
    ON test_cases (test_suite_id, status, automation_status, id);

CREATE TABLE test_case_steps (
    id BIGSERIAL PRIMARY KEY,
    test_case_id BIGINT NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL CHECK (ordinal > 0),
    action TEXT NOT NULL,
    expected_result TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (test_case_id, ordinal)
);

CREATE TABLE test_case_requirement_links (
    test_case_id BIGINT NOT NULL,
    requirement_id BIGINT NOT NULL,
    document_set_id BIGINT NOT NULL,
    coverage_type TEXT NOT NULL CHECK (coverage_type IN
        ('DIRECT', 'BOUNDARY', 'NEGATIVE', 'PERMISSION', 'STATE', 'INTEGRATION', 'REGRESSION')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (test_case_id, requirement_id, coverage_type),
    FOREIGN KEY (test_case_id, document_set_id)
        REFERENCES test_cases(id, document_set_id) ON DELETE CASCADE,
    FOREIGN KEY (requirement_id, document_set_id)
        REFERENCES requirements(id, document_set_id) ON DELETE CASCADE
);

CREATE INDEX test_case_requirement_links_requirement_idx
    ON test_case_requirement_links (requirement_id, test_case_id);

CREATE TABLE test_case_reviews (
    id BIGSERIAL PRIMARY KEY,
    test_case_id BIGINT NOT NULL UNIQUE REFERENCES test_cases(id) ON DELETE CASCADE,
    reviewer_name TEXT NOT NULL,
    decision TEXT NOT NULL CHECK (decision IN ('APPROVED', 'REJECTED')),
    comment TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE automation_artifacts (
    id BIGSERIAL PRIMARY KEY,
    test_case_id BIGINT NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    version_number INTEGER NOT NULL CHECK (version_number > 0),
    framework TEXT NOT NULL,
    file_path TEXT NOT NULL,
    source TEXT NOT NULL,
    source_hash TEXT NOT NULL CHECK (source_hash ~ '^[0-9a-f]{64}$'),
    expected_result_hash TEXT NOT NULL CHECK (expected_result_hash ~ '^[0-9a-f]{64}$'),
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN
        ('DRAFT', 'APPROVED', 'REJECTED', 'UNREPAIRABLE')),
    model_name TEXT NOT NULL DEFAULT '',
    prompt_version TEXT NOT NULL DEFAULT '',
    provider_response_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (test_case_id, version_number),
    UNIQUE (id, test_case_id)
);

CREATE TABLE test_runs (
    id BIGSERIAL PRIMARY KEY,
    test_suite_id BIGINT NOT NULL REFERENCES test_suites(id) ON DELETE RESTRICT,
    project_id BIGINT REFERENCES projects(id) ON DELETE SET NULL,
    analysis_job_id BIGINT REFERENCES analysis_jobs(id) ON DELETE SET NULL,
    source_sha TEXT NOT NULL DEFAULT '',
    target_sha TEXT NOT NULL DEFAULT '',
    environment TEXT NOT NULL DEFAULT '',
    environment_fingerprint TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN
        ('PENDING', 'RUNNING', 'COMPLETED', 'FAILED')),
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);

CREATE TABLE test_run_items (
    id BIGSERIAL PRIMARY KEY,
    test_run_id BIGINT NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
    test_case_id BIGINT NOT NULL REFERENCES test_cases(id) ON DELETE RESTRICT,
    automation_artifact_id BIGINT,
    attempt_number INTEGER NOT NULL DEFAULT 1 CHECK (attempt_number > 0),
    status TEXT NOT NULL DEFAULT 'NOT_RUN' CHECK (status IN
        ('NOT_RUN', 'PASSED', 'PRODUCT_FAILED', 'AUTOMATION_ERROR', 'INFRA_ERROR',
         'TIMED_OUT', 'BLOCKED')),
    expected_result_snapshot TEXT NOT NULL,
    expected_result_hash TEXT NOT NULL CHECK (expected_result_hash ~ '^[0-9a-f]{64}$'),
    automation_source_hash TEXT NOT NULL DEFAULT '' CHECK
        (automation_source_hash = '' OR automation_source_hash ~ '^[0-9a-f]{64}$'),
    actual_result TEXT NOT NULL DEFAULT '',
    command TEXT NOT NULL DEFAULT '',
    exit_code INTEGER,
    duration_ms BIGINT NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    output_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (automation_artifact_id, test_case_id)
        REFERENCES automation_artifacts(id, test_case_id) ON DELETE RESTRICT,
    CHECK ((automation_artifact_id IS NULL AND automation_source_hash = '') OR
           (automation_artifact_id IS NOT NULL AND automation_source_hash ~ '^[0-9a-f]{64}$')),
    UNIQUE (test_run_id, test_case_id, attempt_number),
    UNIQUE (id, test_run_id)
);

CREATE TABLE test_run_evidence (
    id BIGSERIAL PRIMARY KEY,
    test_run_item_id BIGINT NOT NULL REFERENCES test_run_items(id) ON DELETE CASCADE,
    evidence_type TEXT NOT NULL CHECK (evidence_type IN ('STDOUT', 'STDERR', 'LOG', 'SCREENSHOT', 'ARTIFACT')),
    content TEXT NOT NULL DEFAULT '',
    storage_key TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (content <> '' OR storage_key <> '')
);

CREATE OR REPLACE FUNCTION prevent_document_version_identity_update()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.document_id <> OLD.document_id
       OR NEW.document_set_id <> OLD.document_set_id
       OR NEW.version_number <> OLD.version_number
       OR NEW.original_filename <> OLD.original_filename
       OR NEW.media_type <> OLD.media_type
       OR NEW.size_bytes <> OLD.size_bytes
       OR NEW.sha256 <> OLD.sha256
       OR NEW.storage_key <> OLD.storage_key
       OR NEW.uploaded_at <> OLD.uploaded_at THEN
        RAISE EXCEPTION 'document version identity is immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER document_versions_immutable_identity
BEFORE UPDATE ON document_versions
FOR EACH ROW EXECUTE FUNCTION prevent_document_version_identity_update();

CREATE OR REPLACE FUNCTION prevent_source_evidence_update()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% rows are immutable', TG_TABLE_NAME;
END;
$$;

CREATE TRIGGER document_blocks_immutable
BEFORE UPDATE ON document_blocks
FOR EACH ROW EXECUTE FUNCTION prevent_source_evidence_update();

CREATE TRIGGER requirement_evidence_immutable
BEFORE UPDATE ON requirement_evidence
FOR EACH ROW EXECUTE FUNCTION prevent_source_evidence_update();

CREATE OR REPLACE FUNCTION validate_automation_artifact_business_contract()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    test_case_status TEXT;
    test_case_expected_hash TEXT;
BEGIN
    SELECT status, expected_result_hash
      INTO test_case_status, test_case_expected_hash
      FROM test_cases WHERE id=NEW.test_case_id;
    IF test_case_status IS NULL THEN
        RAISE EXCEPTION 'automation artifact test case does not exist';
    END IF;
    IF test_case_status <> 'APPROVED' THEN
        RAISE EXCEPTION 'automation artifacts require an approved test case';
    END IF;
    IF NEW.expected_result_hash <> test_case_expected_hash THEN
        RAISE EXCEPTION 'automation artifact expected result hash does not match test case';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER automation_artifacts_business_contract
BEFORE INSERT OR UPDATE ON automation_artifacts
FOR EACH ROW EXECUTE FUNCTION validate_automation_artifact_business_contract();

CREATE OR REPLACE FUNCTION prevent_test_run_expected_update()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.test_case_id <> OLD.test_case_id
       OR NEW.automation_artifact_id IS DISTINCT FROM OLD.automation_artifact_id
       OR NEW.automation_source_hash <> OLD.automation_source_hash
       OR NEW.expected_result_snapshot <> OLD.expected_result_snapshot
       OR NEW.expected_result_hash <> OLD.expected_result_hash THEN
        RAISE EXCEPTION 'test run business and artifact snapshots are immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER test_run_items_immutable_expected
BEFORE UPDATE ON test_run_items
FOR EACH ROW EXECUTE FUNCTION prevent_test_run_expected_update();
