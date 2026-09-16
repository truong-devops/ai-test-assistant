ALTER TABLE test_runs
    ADD COLUMN execution_requested_at TIMESTAMPTZ,
    ADD COLUMN execution_requested_by TEXT NOT NULL DEFAULT '',
    ADD COLUMN image_reference TEXT NOT NULL DEFAULT '',
    ADD COLUMN image_digest TEXT NOT NULL DEFAULT '',
    ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    ADD COLUMN next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN lease_expires_at TIMESTAMPTZ,
    ADD COLUMN error_message TEXT NOT NULL DEFAULT '';

CREATE INDEX test_runs_execution_queue_idx
    ON test_runs (status, next_attempt_at, requested_at)
    WHERE execution_requested_at IS NOT NULL AND status = 'PENDING';

CREATE TABLE test_run_classification_reviews (
    id BIGSERIAL PRIMARY KEY,
    test_run_item_id BIGINT NOT NULL REFERENCES test_run_items(id) ON DELETE CASCADE,
    reviewer_name TEXT NOT NULL,
    previous_status TEXT NOT NULL CHECK (previous_status IN
        ('NOT_RUN', 'PASSED', 'PRODUCT_FAILED', 'AUTOMATION_ERROR', 'INFRA_ERROR',
         'TIMED_OUT', 'BLOCKED')),
    new_status TEXT NOT NULL CHECK (new_status IN
        ('PASSED', 'PRODUCT_FAILED', 'AUTOMATION_ERROR', 'INFRA_ERROR',
         'TIMED_OUT', 'BLOCKED')),
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX test_run_classification_reviews_item_idx
    ON test_run_classification_reviews (test_run_item_id, created_at DESC, id DESC);

CREATE OR REPLACE FUNCTION prevent_test_run_expected_update()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    artifact_hash TEXT;
    artifact_expected_hash TEXT;
BEGIN
    IF NEW.test_case_id <> OLD.test_case_id
       OR NEW.expected_result_snapshot <> OLD.expected_result_snapshot
       OR NEW.expected_result_hash <> OLD.expected_result_hash THEN
        RAISE EXCEPTION 'test run business snapshot is immutable';
    END IF;

    IF NEW.automation_artifact_id IS DISTINCT FROM OLD.automation_artifact_id
       OR NEW.automation_source_hash <> OLD.automation_source_hash THEN
        IF OLD.status <> 'NOT_RUN'
           OR OLD.automation_artifact_id IS NOT NULL
           OR OLD.automation_source_hash <> ''
           OR NEW.automation_artifact_id IS NULL THEN
            RAISE EXCEPTION 'test run artifact snapshot is immutable once bound';
        END IF;
        SELECT source_hash, expected_result_hash
          INTO artifact_hash, artifact_expected_hash
          FROM automation_artifacts
         WHERE id = NEW.automation_artifact_id
           AND test_case_id = NEW.test_case_id
           AND status = 'APPROVED';
        IF artifact_hash IS NULL
           OR NEW.automation_source_hash <> artifact_hash
           OR artifact_expected_hash <> NEW.expected_result_hash THEN
            RAISE EXCEPTION 'test run artifact must be approved and match the immutable business contract';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

