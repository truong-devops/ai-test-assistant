DROP TRIGGER IF EXISTS automation_artifacts_immutable_contract ON automation_artifacts;
DROP FUNCTION IF EXISTS prevent_automation_artifact_contract_update();
DROP TRIGGER IF EXISTS automation_generation_calls_immutable ON automation_generation_calls;
DROP TRIGGER IF EXISTS analysis_baseline_snapshots_immutable ON analysis_baseline_snapshots;
DROP TRIGGER IF EXISTS test_exports_immutable ON test_exports;
DROP FUNCTION IF EXISTS prevent_phase_6_8_snapshot_update();

DROP TABLE IF EXISTS automation_artifact_reviews;
DROP TABLE IF EXISTS automation_generation_calls;
DROP INDEX IF EXISTS automation_artifacts_analysis_idx;
ALTER TABLE automation_artifacts
    DROP CONSTRAINT IF EXISTS automation_technical_context_hash_check,
    DROP CONSTRAINT IF EXISTS automation_business_context_hash_check,
    DROP COLUMN technical_context_hash,
    DROP COLUMN business_context_hash,
    DROP COLUMN technical_context,
    DROP COLUMN business_context,
    DROP COLUMN test_case_snapshot,
    DROP COLUMN assertions,
    DROP COLUMN setup_text,
    DROP COLUMN analysis_job_id;

DROP TABLE IF EXISTS analysis_scope_decisions;
DROP TABLE IF EXISTS analysis_scope_signals;
DROP TABLE IF EXISTS analysis_test_scope_items;
DROP TABLE IF EXISTS analysis_baseline_snapshots;
DROP TABLE IF EXISTS project_document_baselines;
DROP TABLE IF EXISTS test_exports;
DROP INDEX IF EXISTS test_runs_analysis_unique_idx;
ALTER TABLE test_runs DROP CONSTRAINT IF EXISTS test_runs_id_suite_unique;
