BEGIN;

-- Migration 25 copied binding.updated_at into snapshot timestamps. That is not
-- evidence of a historical publication or of the original migration's time.
-- Record the original values before correcting metadata to THIS migration's
-- transaction time. Never change IDs, manifests, bindings or persisted proof.
CREATE TABLE test_suite_release_timestamp_audits (
    release_id BIGINT PRIMARY KEY REFERENCES test_suite_releases(id) ON DELETE CASCADE,
    migration_version INTEGER NOT NULL CHECK (migration_version = 31),
    previous_published_at TIMESTAMPTZ NOT NULL,
    previous_created_at TIMESTAMPTZ NOT NULL,
    previous_item_created_at JSONB NOT NULL CHECK (jsonb_typeof(previous_item_created_at) = 'array'),
    corrected_at TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    reason TEXT NOT NULL
);

-- Exclusive locks keep application sessions from seeing disabled protections.
-- Trigger changes and corrections are transactional, including on failure.
LOCK TABLE test_suite_releases, test_suite_release_items IN ACCESS EXCLUSIVE MODE;
INSERT INTO test_suite_release_timestamp_audits
    (release_id,migration_version,previous_published_at,previous_created_at,
     previous_item_created_at,reason)
SELECT r.id,31,r.published_at,r.created_at,
    COALESCE((SELECT jsonb_agg(jsonb_build_object('family_id',i.family_id,
        'test_case_id',i.test_case_id,'created_at',i.created_at) ORDER BY i.ordinal)
        FROM test_suite_release_items i WHERE i.release_id=r.id),'[]'::jsonb),
    'Migration 25 used legacy binding time; corrected at migration 31, not a historical user publication. Original migration time is unknown.'
FROM test_suite_releases r WHERE r.origin='MIGRATED_CURRENT_STATE';

ALTER TABLE test_suite_releases DISABLE TRIGGER test_suite_releases_immutable;
ALTER TABLE test_suite_release_items DISABLE TRIGGER test_suite_release_items_immutable;
UPDATE test_suite_releases r SET published_at=a.corrected_at,created_at=a.corrected_at
FROM test_suite_release_timestamp_audits a WHERE a.release_id=r.id;
UPDATE test_suite_release_items i SET created_at=a.corrected_at
FROM test_suite_release_timestamp_audits a WHERE a.release_id=i.release_id;
ALTER TABLE test_suite_release_items ENABLE TRIGGER test_suite_release_items_immutable;
ALTER TABLE test_suite_releases ENABLE TRIGGER test_suite_releases_immutable;

CREATE TRIGGER test_suite_release_timestamp_audits_immutable
BEFORE UPDATE OR DELETE ON test_suite_release_timestamp_audits
FOR EACH ROW EXECUTE FUNCTION protect_test_suite_release_graph();

COMMIT;
