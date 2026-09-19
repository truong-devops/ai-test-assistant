CREATE TABLE test_suite_releases (
    id BIGSERIAL PRIMARY KEY,
    test_suite_id BIGINT NOT NULL,
    document_set_id BIGINT NOT NULL,
    release_number INTEGER NOT NULL CHECK (release_number > 0),
    name TEXT NOT NULL,
    source_snapshot_id BIGINT REFERENCES document_source_snapshots(id) ON DELETE RESTRICT,
    manifest_hash TEXT NOT NULL CHECK (manifest_hash ~ '^[0-9a-f]{64}$'),
    request_hash TEXT NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    scope_status TEXT NOT NULL CHECK (scope_status IN ('COMPLETE','PARTIAL')),
    approved_requirement_count INTEGER NOT NULL DEFAULT 0 CHECK (approved_requirement_count >= 0),
    covered_requirement_count INTEGER NOT NULL DEFAULT 0 CHECK (covered_requirement_count >= 0),
    uncovered_requirement_ids BIGINT[] NOT NULL DEFAULT '{}',
    scope_decision TEXT NOT NULL DEFAULT '',
    published_by TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    origin TEXT NOT NULL DEFAULT 'USER_PUBLISHED' CHECK
        (origin IN ('USER_PUBLISHED','MIGRATED_CURRENT_STATE')),
    published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (test_suite_id,document_set_id)
        REFERENCES test_suites(id,document_set_id) ON DELETE CASCADE,
    FOREIGN KEY (source_snapshot_id,document_set_id)
        REFERENCES document_source_snapshots(id,document_set_id) ON DELETE RESTRICT,
    UNIQUE (test_suite_id,release_number),
    UNIQUE (test_suite_id,idempotency_key),
    UNIQUE (test_suite_id,manifest_hash),
    UNIQUE (id,test_suite_id,document_set_id)
);

CREATE TABLE test_suite_release_items (
    release_id BIGINT NOT NULL,
    test_suite_id BIGINT NOT NULL,
    document_set_id BIGINT NOT NULL,
    family_id BIGINT NOT NULL,
    test_case_id BIGINT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal > 0),
    public_key TEXT NOT NULL,
    revision_number INTEGER NOT NULL CHECK (revision_number > 0),
    content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
    expected_result_hash TEXT NOT NULL CHECK (expected_result_hash ~ '^[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (release_id,family_id),
    UNIQUE (release_id,test_case_id),
    UNIQUE (release_id,ordinal),
    FOREIGN KEY (release_id,test_suite_id,document_set_id)
        REFERENCES test_suite_releases(id,test_suite_id,document_set_id) ON DELETE CASCADE,
    FOREIGN KEY (family_id,document_set_id,test_suite_id)
        REFERENCES test_case_families(id,document_set_id,test_suite_id) ON DELETE RESTRICT,
    FOREIGN KEY (test_case_id,family_id,document_set_id)
        REFERENCES test_cases(id,family_id,document_set_id) ON DELETE RESTRICT
);

CREATE INDEX test_suite_releases_set_published_idx
    ON test_suite_releases(document_set_id,published_at DESC,id DESC);
CREATE INDEX test_suite_release_items_revision_idx
    ON test_suite_release_items(test_case_id,release_id);

-- Preserve exactly the selection used by the legacy project binding. The release is
-- explicitly labelled as a migration snapshot, not as a historical user publication.
INSERT INTO test_suite_releases
    (test_suite_id,document_set_id,release_number,name,source_snapshot_id,
     manifest_hash,request_hash,scope_status,published_by,idempotency_key,origin,
     published_at,created_at)
SELECT DISTINCT b.test_suite_id,b.document_set_id,1,'Migrated baseline R1',
    CASE WHEN (SELECT count(DISTINCT t.source_snapshot_id)
                    FILTER (WHERE t.source_snapshot_id IS NOT NULL)
               FROM test_cases t
              WHERE t.test_suite_id=b.test_suite_id AND t.status='APPROVED'
                AND NOT EXISTS (SELECT 1 FROM test_cases n
                                 WHERE n.supersedes_test_case_id=t.id))=1
         THEN (SELECT min(t.source_snapshot_id) FROM test_cases t
                WHERE t.test_suite_id=b.test_suite_id AND t.status='APPROVED'
                  AND NOT EXISTS (SELECT 1 FROM test_cases n
                                   WHERE n.supersedes_test_case_id=t.id))
         ELSE NULL END,
    repeat('0',64),
    encode(digest(convert_to('migrated-project-baseline:' || b.test_suite_id,'UTF8'),'sha256'),'hex'),
    'PARTIAL','MIGRATION','migration-current-state','MIGRATED_CURRENT_STATE',
    min(b.updated_at),min(b.updated_at)
FROM project_document_baselines b
GROUP BY b.test_suite_id,b.document_set_id;

INSERT INTO test_suite_release_items
    (release_id,test_suite_id,document_set_id,family_id,test_case_id,ordinal,
     public_key,revision_number,content_hash,expected_result_hash,created_at)
SELECT release.id,release.test_suite_id,release.document_set_id,t.family_id,t.id,
    (row_number() OVER (PARTITION BY release.id ORDER BY t.test_case_key,t.id))::integer,
    t.test_case_key,t.version_number,t.content_hash,t.expected_result_hash,release.created_at
FROM test_suite_releases release
JOIN test_cases t ON t.test_suite_id=release.test_suite_id
WHERE release.origin='MIGRATED_CURRENT_STATE' AND t.status='APPROVED'
  AND NOT EXISTS (SELECT 1 FROM test_cases newer
                   WHERE newer.supersedes_test_case_id=t.id);

WITH manifests AS (
    SELECT release.id,
        encode(digest(convert_to(COALESCE(jsonb_agg(jsonb_build_object(
            'family_id',item.family_id,'test_case_id',item.test_case_id,
            'content_hash',item.content_hash,'expected_result_hash',item.expected_result_hash)
            ORDER BY item.ordinal),'[]'::jsonb)::text,'UTF8'),'sha256'),'hex') AS manifest_hash
    FROM test_suite_releases release
    LEFT JOIN test_suite_release_items item ON item.release_id=release.id
    WHERE release.origin='MIGRATED_CURRENT_STATE'
    GROUP BY release.id
), coverage AS (
    SELECT release.id,
        (SELECT count(*) FROM requirements requirement
          WHERE requirement.document_set_id=release.document_set_id
            AND requirement.status='APPROVED'
            AND NOT EXISTS (SELECT 1 FROM requirements newer
                             WHERE newer.supersedes_requirement_id=requirement.id)) AS approved_count,
        (SELECT count(DISTINCT link.requirement_id)
           FROM test_suite_release_items item
           JOIN test_case_requirement_links link ON link.test_case_id=item.test_case_id
          WHERE item.release_id=release.id) AS covered_count,
        ARRAY(SELECT requirement.id FROM requirements requirement
               WHERE requirement.document_set_id=release.document_set_id
                 AND requirement.status='APPROVED'
                 AND NOT EXISTS (SELECT 1 FROM requirements newer
                                  WHERE newer.supersedes_requirement_id=requirement.id)
                 AND NOT EXISTS (
                     SELECT 1 FROM test_suite_release_items item
                     JOIN test_case_requirement_links link ON link.test_case_id=item.test_case_id
                     WHERE item.release_id=release.id AND link.requirement_id=requirement.id)
               ORDER BY requirement.id) AS uncovered_ids
    FROM test_suite_releases release
    WHERE release.origin='MIGRATED_CURRENT_STATE'
)
UPDATE test_suite_releases release SET
    manifest_hash=manifests.manifest_hash,
    approved_requirement_count=coverage.approved_count,
    covered_requirement_count=coverage.covered_count,
    uncovered_requirement_ids=coverage.uncovered_ids,
    scope_status=CASE WHEN cardinality(coverage.uncovered_ids)=0 THEN 'COMPLETE' ELSE 'PARTIAL' END,
    scope_decision=CASE WHEN cardinality(coverage.uncovered_ids)=0 THEN ''
        ELSE 'Migrated from the exact legacy current-state selection; coverage requires review' END
FROM manifests,coverage
WHERE release.id=manifests.id AND release.id=coverage.id;

ALTER TABLE project_document_baselines
    ADD COLUMN suite_release_id BIGINT;

UPDATE project_document_baselines baseline SET suite_release_id=release.id
FROM test_suite_releases release
WHERE release.test_suite_id=baseline.test_suite_id
  AND release.origin='MIGRATED_CURRENT_STATE';

ALTER TABLE project_document_baselines
    ADD CONSTRAINT project_document_baselines_release_scope_fk
    FOREIGN KEY (suite_release_id,test_suite_id,document_set_id)
    REFERENCES test_suite_releases(id,test_suite_id,document_set_id) ON DELETE RESTRICT;

ALTER TABLE analysis_baseline_snapshots
    ADD COLUMN suite_release_id BIGINT REFERENCES test_suite_releases(id) ON DELETE RESTRICT;
ALTER TABLE test_runs
    ADD COLUMN suite_release_id BIGINT REFERENCES test_suite_releases(id) ON DELETE RESTRICT;
ALTER TABLE test_exports
    ADD COLUMN suite_release_id BIGINT REFERENCES test_suite_releases(id) ON DELETE RESTRICT;

CREATE OR REPLACE FUNCTION protect_test_suite_release_graph()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='DELETE' AND (pg_trigger_depth()>1 OR
        current_setting('app.allow_testcase_graph_delete',TRUE)='on') THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'published suite release rows are immutable';
END;
$$;

CREATE TRIGGER test_suite_releases_immutable
BEFORE UPDATE OR DELETE ON test_suite_releases
FOR EACH ROW EXECUTE FUNCTION protect_test_suite_release_graph();
CREATE TRIGGER test_suite_release_items_immutable
BEFORE UPDATE OR DELETE ON test_suite_release_items
FOR EACH ROW EXECUTE FUNCTION protect_test_suite_release_graph();
