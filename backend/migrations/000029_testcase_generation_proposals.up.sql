CREATE TABLE test_case_generation_proposals (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    test_suite_id BIGINT NOT NULL REFERENCES test_suites(id) ON DELETE CASCADE,
    workflow_job_id BIGINT NOT NULL REFERENCES document_workflow_jobs(id) ON DELETE CASCADE,
    proposal_key TEXT NOT NULL,
    classification TEXT NOT NULL CHECK(classification IN ('NEW_CASE','NEW_REVISION','UNCHANGED','RETIRE_CANDIDATE','AMBIGUOUS_MATCH')),
    reason TEXT NOT NULL,
    content JSONB NOT NULL,
    content_hash TEXT NOT NULL,
    generation JSONB NOT NULL,
    candidates JSONB NOT NULL CHECK(jsonb_typeof(candidates)='array'),
    source_revision BIGINT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','APPLIED','DISMISSED')),
    result_revision_id BIGINT REFERENCES test_cases(id) ON DELETE RESTRICT,
    decision TEXT NOT NULL DEFAULT '',
    decision_reason TEXT NOT NULL DEFAULT '',
    decided_by TEXT NOT NULL DEFAULT '',
    decided_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workflow_job_id,proposal_key)
);
CREATE INDEX testcase_proposals_inventory_idx ON test_case_generation_proposals(document_set_id,id DESC);

-- Preserve exactly what the provider proposed and the heads observed before it ran.
CREATE FUNCTION protect_testcase_proposal_input() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (to_jsonb(NEW)-'status'-'result_revision_id'-'decision'-'decision_reason'-'decided_by'-'decided_at')
        IS DISTINCT FROM (to_jsonb(OLD)-'status'-'result_revision_id'-'decision'-'decision_reason'-'decided_by'-'decided_at') THEN
        RAISE EXCEPTION 'generation proposal input is immutable';
    END IF;
    IF OLD.status <> 'PENDING' AND to_jsonb(NEW) IS DISTINCT FROM to_jsonb(OLD) THEN
        RAISE EXCEPTION 'generation proposal decision is immutable';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER testcase_proposal_input_immutable BEFORE UPDATE ON test_case_generation_proposals
FOR EACH ROW EXECUTE FUNCTION protect_testcase_proposal_input();

CREATE TABLE test_case_proposal_commands (
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    proposal_id BIGINT NOT NULL REFERENCES test_case_generation_proposals(id) ON DELETE CASCADE,
    request_hash TEXT NOT NULL,
    result JSONB NOT NULL,
    actor TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(document_set_id,idempotency_key)
);
