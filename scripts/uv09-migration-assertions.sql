-- Read-only assertions for the isolated schema-23 UV09 legacy fixture after up31.
-- Used by CI's populated migration lane; not a production data quality audit.
BEGIN READ ONLY;
DO $$
BEGIN
    IF NOT COALESCE((SELECT count(*)=1 AND bool_and(
        r.origin='MIGRATED_CURRENT_STATE' AND a.migration_version=31
        AND a.previous_published_at='2001-01-01T00:00:00Z'::timestamptz
        AND a.previous_created_at=a.previous_published_at
        AND r.published_at=a.corrected_at AND r.created_at=a.corrected_at
        AND a.corrected_at>a.previous_published_at
        AND jsonb_array_length(a.previous_item_created_at)=1
        AND (a.previous_item_created_at->0->>'created_at')::timestamptz=a.previous_created_at)
        FROM test_suite_release_timestamp_audits a JOIN test_suite_releases r ON r.id=a.release_id),FALSE) THEN
        RAISE EXCEPTION 'migration timestamp correction/audit mismatch';
    END IF;
    IF NOT (SELECT count(*)=1 AND bool_and(t.test_case_key='TC-INDEPENDENT'
        AND i.content_hash=t.content_hash AND i.expected_result_hash=t.expected_result_hash
        AND i.created_at=a.corrected_at)
        FROM test_suite_release_items i JOIN test_cases t ON t.id=i.test_case_id
        JOIN test_suite_release_timestamp_audits a ON a.release_id=i.release_id) THEN
        RAISE EXCEPTION 'legacy release selection/hash changed';
    END IF;
    IF NOT (SELECT count(*)=1 AND bool_and(suite_release_id IS NULL) FROM test_runs)
        OR NOT (SELECT count(*)=1 AND bool_and(status='PASSED' AND expected_result_snapshot='Expected v1') FROM test_run_items)
        OR NOT (SELECT count(*)=1 AND bool_and(content_hash=encode(sha256(content),'hex')) FROM test_exports) THEN
        RAISE EXCEPTION 'legacy run/export proof changed';
    END IF;
    IF NOT (SELECT count(*)=3 AND bool_and(tgenabled='O') FROM pg_trigger
        WHERE tgname IN ('test_suite_releases_immutable','test_suite_release_items_immutable','test_suite_release_timestamp_audits_immutable')) THEN
        RAISE EXCEPTION 'immutable trigger not enabled';
    END IF;
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE contype='f' AND NOT convalidated) THEN
        RAISE EXCEPTION 'unvalidated foreign key';
    END IF;
END $$;
COMMIT;
