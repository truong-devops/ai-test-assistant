BEGIN;
-- Empty database roundtrips are supported. On populated databases retain the
-- correction audit: use a forward fix or a coordinated pre-migration restore.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM test_suite_release_timestamp_audits) THEN
        RAISE EXCEPTION 'cannot downgrade migration 31 with timestamp correction audits; use a forward fix or coordinated backup restore';
    END IF;
END $$;
DROP TABLE test_suite_release_timestamp_audits;
COMMIT;
