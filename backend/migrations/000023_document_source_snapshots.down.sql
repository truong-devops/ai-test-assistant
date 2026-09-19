DROP INDEX IF EXISTS requirements_source_snapshot_idx;
ALTER TABLE requirements DROP COLUMN IF EXISTS source_snapshot_id;

ALTER TABLE requirement_extraction_jobs
    DROP COLUMN IF EXISTS is_current,
    DROP COLUMN IF EXISTS source_revision,
    DROP COLUMN IF EXISTS source_snapshot_id;

ALTER TABLE document_context_snapshots DROP COLUMN IF EXISTS source_snapshot_id;

DROP TABLE IF EXISTS document_index_generation_chunks;
DROP TABLE IF EXISTS document_index_generations;

ALTER TABLE document_index_status
    DROP COLUMN IF EXISTS content_warning_count,
    DROP COLUMN IF EXISTS indexed_source_revision,
    DROP COLUMN IF EXISTS source_snapshot_id;

DROP TRIGGER IF EXISTS document_source_snapshot_items_immutable ON document_source_snapshot_items;
DROP TRIGGER IF EXISTS document_source_snapshots_immutable ON document_source_snapshots;
DROP TABLE IF EXISTS document_source_snapshot_items;
DROP TABLE IF EXISTS document_source_snapshots;

DROP TRIGGER IF EXISTS document_versions_bump_source_revision ON document_versions;
DROP FUNCTION IF EXISTS bump_document_set_source_revision();
ALTER TABLE document_sets DROP COLUMN IF EXISTS source_revision;
