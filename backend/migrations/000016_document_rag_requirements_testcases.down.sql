DROP TRIGGER IF EXISTS test_cases_approval_evidence ON test_cases;
DROP FUNCTION IF EXISTS enforce_test_case_approval_evidence();
DROP TRIGGER IF EXISTS requirements_approval_evidence ON requirements;
DROP FUNCTION IF EXISTS enforce_requirement_approval_evidence();
DROP TRIGGER IF EXISTS document_context_snapshot_items_immutable ON document_context_snapshot_items;
DROP TRIGGER IF EXISTS document_context_snapshots_immutable ON document_context_snapshots;
DROP FUNCTION IF EXISTS prevent_context_snapshot_update();
DROP TRIGGER IF EXISTS document_versions_approval_dependencies ON document_versions;
DROP FUNCTION IF EXISTS enforce_document_version_approval_dependencies();

DROP INDEX IF EXISTS test_case_reviews_case_idx;
ALTER TABLE test_case_reviews
    ADD CONSTRAINT test_case_reviews_test_case_id_key UNIQUE (test_case_id);
DROP TABLE IF EXISTS test_case_dedupe_records;
DROP INDEX IF EXISTS test_cases_generation_key_idx;
ALTER TABLE test_cases DROP COLUMN generation_key, DROP COLUMN assumptions;

DROP INDEX IF EXISTS requirement_reviews_requirement_idx;
ALTER TABLE requirement_reviews
    ADD CONSTRAINT requirement_reviews_requirement_id_key UNIQUE (requirement_id);
DROP INDEX IF EXISTS open_questions_requirement_question_idx;
DROP TABLE IF EXISTS requirement_flow_steps;
DROP INDEX IF EXISTS requirements_set_actor_flow_idx;
DROP INDEX IF EXISTS requirements_extraction_key_idx;
ALTER TABLE requirements
    DROP COLUMN raw_payload,
    DROP COLUMN assumptions,
    DROP COLUMN source_fingerprint,
    DROP COLUMN extraction_key,
    DROP COLUMN risk;

DROP TABLE IF EXISTS document_version_reviews;
DROP TABLE IF EXISTS document_ai_calls;
DROP TABLE IF EXISTS document_context_snapshot_items;
DROP TABLE IF EXISTS document_context_snapshots;
DROP TABLE IF EXISTS document_chunk_source_blocks;
DROP INDEX IF EXISTS document_chunks_set_version_type_idx;
DROP INDEX IF EXISTS document_chunks_set_identifier_idx;
ALTER TABLE document_chunks DROP CONSTRAINT document_chunks_block_version_fk;
ALTER TABLE document_chunks
    DROP COLUMN raw_content,
    DROP COLUMN flow_type,
    DROP COLUMN parent_chunk_key,
    DROP COLUMN document_block_id;
DROP TABLE IF EXISTS document_index_status;
