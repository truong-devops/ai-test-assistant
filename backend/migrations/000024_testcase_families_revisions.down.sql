DROP TRIGGER IF EXISTS test_cases_approval_evidence ON test_cases;
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
BEFORE UPDATE ON test_cases FOR EACH ROW EXECUTE FUNCTION enforce_test_case_approval_evidence();

DROP TRIGGER IF EXISTS test_case_evidence_links_revision_immutable ON test_case_evidence_links;
DROP TRIGGER IF EXISTS test_case_requirement_links_revision_immutable ON test_case_requirement_links;
DROP TRIGGER IF EXISTS test_case_steps_revision_immutable ON test_case_steps;
DROP TRIGGER IF EXISTS test_cases_revision_content_immutable ON test_cases;
DROP TRIGGER IF EXISTS test_cases_family_head_after_insert ON test_cases;
DROP TRIGGER IF EXISTS test_cases_legacy_family_before_insert ON test_cases;
DROP FUNCTION IF EXISTS protect_test_case_revision_child();
DROP FUNCTION IF EXISTS protect_test_case_revision_content();
DROP FUNCTION IF EXISTS update_test_case_family_head_on_insert();
DROP FUNCTION IF EXISTS ensure_test_case_family_on_legacy_insert();

ALTER TABLE test_case_reviews DROP COLUMN IF EXISTS actor,DROP COLUMN IF EXISTS content_hash;
DROP TABLE IF EXISTS test_case_identity_migration_issues;
DROP TABLE IF EXISTS test_case_revision_audit;
DROP TABLE IF EXISTS test_case_revision_commands;

ALTER TABLE test_case_families DROP CONSTRAINT IF EXISTS test_case_families_head_scope_fk;
DROP FUNCTION IF EXISTS test_case_revision_content_hash(BIGINT);
DROP TABLE IF EXISTS test_case_evidence_links;

ALTER TABLE test_cases
    DROP CONSTRAINT IF EXISTS test_cases_restored_family_fk,
    DROP CONSTRAINT IF EXISTS test_cases_parent_family_fk,
    DROP CONSTRAINT IF EXISTS test_cases_id_family_set_key,
    DROP CONSTRAINT IF EXISTS test_cases_id_family_key,
    DROP CONSTRAINT IF EXISTS test_cases_family_version_key,
    DROP CONSTRAINT IF EXISTS test_cases_family_scope_fk,
    DROP COLUMN IF EXISTS sealed_at,
    DROP COLUMN IF EXISTS provenance,
    DROP COLUMN IF EXISTS source_snapshot_id,
    DROP COLUMN IF EXISTS change_reason,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS content_hash,
    DROP COLUMN IF EXISTS restored_from_revision_id,
    DROP COLUMN IF EXISTS parent_revision_id,
    DROP COLUMN IF EXISTS family_id;

DROP TABLE IF EXISTS test_case_families;
DROP SEQUENCE IF EXISTS test_case_public_key_seq;
