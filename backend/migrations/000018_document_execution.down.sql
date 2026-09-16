DROP INDEX IF EXISTS test_run_classification_reviews_item_idx;
DROP TABLE IF EXISTS test_run_classification_reviews;
DROP INDEX IF EXISTS test_runs_execution_queue_idx;

ALTER TABLE test_runs
    DROP COLUMN IF EXISTS error_message,
    DROP COLUMN IF EXISTS lease_expires_at,
    DROP COLUMN IF EXISTS next_attempt_at,
    DROP COLUMN IF EXISTS attempt_count,
    DROP COLUMN IF EXISTS image_digest,
    DROP COLUMN IF EXISTS image_reference,
    DROP COLUMN IF EXISTS execution_requested_by,
    DROP COLUMN IF EXISTS execution_requested_at;

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

