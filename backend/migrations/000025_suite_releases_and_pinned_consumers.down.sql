DROP TRIGGER IF EXISTS test_suite_release_items_immutable ON test_suite_release_items;
DROP TRIGGER IF EXISTS test_suite_releases_immutable ON test_suite_releases;
DROP FUNCTION IF EXISTS protect_test_suite_release_graph();

ALTER TABLE test_exports DROP COLUMN IF EXISTS suite_release_id;
ALTER TABLE test_runs DROP COLUMN IF EXISTS suite_release_id;
ALTER TABLE analysis_baseline_snapshots DROP COLUMN IF EXISTS suite_release_id;
ALTER TABLE project_document_baselines
    DROP CONSTRAINT IF EXISTS project_document_baselines_release_scope_fk,
    DROP COLUMN IF EXISTS suite_release_id;

DROP TABLE IF EXISTS test_suite_release_items;
DROP TABLE IF EXISTS test_suite_releases;
