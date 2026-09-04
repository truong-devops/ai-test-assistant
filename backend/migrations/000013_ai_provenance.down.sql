DROP TRIGGER IF EXISTS context_snapshot_items_immutable ON context_snapshot_items;
DROP TRIGGER IF EXISTS context_snapshots_immutable ON context_snapshots;
DROP TRIGGER IF EXISTS llm_calls_immutable ON llm_calls;
DROP FUNCTION IF EXISTS reject_ai_provenance_update();
DROP TABLE IF EXISTS context_snapshot_items;
DROP TABLE IF EXISTS context_snapshots;
DROP TABLE IF EXISTS llm_calls;
ALTER TABLE analysis_jobs
    DROP CONSTRAINT IF EXISTS analysis_jobs_id_project_id_unique;
