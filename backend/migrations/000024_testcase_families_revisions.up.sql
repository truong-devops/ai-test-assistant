CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SEQUENCE test_case_public_key_seq;

CREATE TABLE test_case_families (
    id BIGSERIAL PRIMARY KEY,
    test_suite_id BIGINT NOT NULL,
    document_set_id BIGINT NOT NULL,
    public_key TEXT NOT NULL,
    legacy_key TEXT NOT NULL DEFAULT '',
    archived BOOLEAN NOT NULL DEFAULT FALSE,
    revision_counter INTEGER NOT NULL DEFAULT 0 CHECK (revision_counter >= 0),
    head_revision_id BIGINT,
    head_token TEXT NOT NULL CHECK (head_token ~ '^[0-9a-f]{64}$'),
    needs_identity_review BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (test_suite_id, document_set_id)
        REFERENCES test_suites(id, document_set_id) ON DELETE CASCADE,
    UNIQUE (test_suite_id, public_key),
    UNIQUE (id, document_set_id, test_suite_id),
    UNIQUE (id, document_set_id)
);

INSERT INTO test_case_families
    (test_suite_id,document_set_id,public_key,legacy_key,revision_counter,head_token,
     needs_identity_review,created_at,updated_at)
SELECT t.test_suite_id,t.document_set_id,t.test_case_key,t.test_case_key,
    max(t.version_number),
    encode(digest(convert_to('legacy-family:' || t.test_suite_id || ':' || t.test_case_key,
        'UTF8'),'sha256'),'hex'),
    count(DISTINCT lower(trim(t.title))) > 1,
    min(t.created_at),max(t.updated_at)
FROM test_cases t
GROUP BY t.test_suite_id,t.document_set_id,t.test_case_key;

ALTER TABLE test_cases
    ADD COLUMN family_id BIGINT,
    ADD COLUMN parent_revision_id BIGINT,
    ADD COLUMN restored_from_revision_id BIGINT,
    ADD COLUMN content_hash TEXT NOT NULL DEFAULT repeat('0',64)
        CHECK (content_hash ~ '^[0-9a-f]{64}$'),
    ADD COLUMN created_by TEXT NOT NULL DEFAULT 'LEGACY_BACKFILL',
    ADD COLUMN change_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN source_snapshot_id BIGINT REFERENCES document_source_snapshots(id) ON DELETE RESTRICT,
    ADD COLUMN provenance JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(provenance)='object'),
    ADD COLUMN sealed_at TIMESTAMPTZ;

UPDATE test_cases t SET family_id=f.id,parent_revision_id=t.supersedes_test_case_id,
    source_snapshot_id=(SELECT min(req.source_snapshot_id)
        FROM test_case_requirement_links l JOIN requirements req ON req.id=l.requirement_id
        WHERE l.test_case_id=t.id),
    provenance=jsonb_build_object('migration','UV-02','legacy_test_case_key',t.test_case_key)
FROM test_case_families f
WHERE f.test_suite_id=t.test_suite_id AND f.legacy_key=t.test_case_key;

-- Backfill can enqueue deferred FK checks from artifacts/runs that reference
-- legacy testcase rows. Drain them before altering the referenced table.
SET CONSTRAINTS ALL IMMEDIATE;

ALTER TABLE test_cases ALTER COLUMN family_id SET NOT NULL;
ALTER TABLE test_cases
    ADD CONSTRAINT test_cases_family_scope_fk
        FOREIGN KEY (family_id,document_set_id,test_suite_id)
        REFERENCES test_case_families(id,document_set_id,test_suite_id) ON DELETE CASCADE,
    ADD CONSTRAINT test_cases_family_version_key UNIQUE (family_id,version_number),
    ADD CONSTRAINT test_cases_id_family_key UNIQUE (id,family_id),
    ADD CONSTRAINT test_cases_id_family_set_key UNIQUE (id,family_id,document_set_id),
    ADD CONSTRAINT test_cases_parent_family_fk
        FOREIGN KEY (parent_revision_id,family_id)
        REFERENCES test_cases(id,family_id) DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT test_cases_restored_family_fk
        FOREIGN KEY (restored_from_revision_id,family_id)
        REFERENCES test_cases(id,family_id) DEFERRABLE INITIALLY DEFERRED;

CREATE INDEX test_cases_family_history_idx
    ON test_cases(family_id,version_number DESC,id DESC);

CREATE TABLE test_case_evidence_links (
    test_case_id BIGINT NOT NULL,
    family_id BIGINT NOT NULL,
    document_set_id BIGINT NOT NULL,
    requirement_id BIGINT NOT NULL,
    requirement_evidence_id BIGINT NOT NULL REFERENCES requirement_evidence(id)
        DEFERRABLE INITIALLY DEFERRED,
    document_version_id BIGINT NOT NULL,
    document_block_id BIGINT NOT NULL,
    source_locator TEXT NOT NULL,
    excerpt_hash TEXT NOT NULL CHECK (excerpt_hash ~ '^[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (test_case_id,requirement_evidence_id),
    FOREIGN KEY (test_case_id,family_id,document_set_id)
        REFERENCES test_cases(id,family_id,document_set_id) ON DELETE CASCADE,
    FOREIGN KEY (requirement_id,document_set_id)
        REFERENCES requirements(id,document_set_id) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (document_version_id,document_set_id)
        REFERENCES document_versions(id,document_set_id) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (document_block_id,document_version_id)
        REFERENCES document_blocks(id,document_version_id) DEFERRABLE INITIALLY DEFERRED
);

INSERT INTO test_case_evidence_links
    (test_case_id,family_id,document_set_id,requirement_id,requirement_evidence_id,
     document_version_id,document_block_id,source_locator,excerpt_hash,created_at)
SELECT l.test_case_id,t.family_id,l.document_set_id,l.requirement_id,e.id,
    e.document_version_id,e.document_block_id,e.source_locator,e.excerpt_hash,e.created_at
FROM test_case_requirement_links l
JOIN test_cases t ON t.id=l.test_case_id
JOIN requirement_evidence e ON e.requirement_id=l.requirement_id;

CREATE OR REPLACE FUNCTION test_case_revision_content_hash(revision_id BIGINT)
RETURNS TEXT LANGUAGE sql STABLE AS $$
    SELECT encode(digest(convert_to(jsonb_build_object(
        'title',trim(t.title),
        'test_type',t.test_type,
        'risk',t.risk,
        'actor',trim(t.actor),
        'precondition',trim(t.precondition),
        'test_data',trim(t.test_data),
        'steps',COALESCE((SELECT jsonb_agg(jsonb_build_object(
            'ordinal',s.ordinal,'action',trim(s.action),'expected_result',trim(s.expected_result))
            ORDER BY s.ordinal) FROM test_case_steps s WHERE s.test_case_id=t.id),'[]'::jsonb),
        'expected_result',trim(t.expected_result),
        'postcondition',trim(t.postcondition),
        'assumptions',t.assumptions,
        'requirement_revision_ids',COALESCE((SELECT jsonb_agg(x.requirement_id ORDER BY x.requirement_id)
            FROM (SELECT DISTINCT l.requirement_id FROM test_case_requirement_links l
                  WHERE l.test_case_id=t.id) x),'[]'::jsonb),
        'evidence_refs',COALESCE((SELECT jsonb_agg(jsonb_build_object(
            'requirement_revision_id',e.requirement_id,
            'document_version_id',e.document_version_id,
            'document_block_id',e.document_block_id,
            'source_locator',e.source_locator,
            'excerpt_hash',e.excerpt_hash)
            ORDER BY e.requirement_id,e.document_version_id,e.document_block_id,e.requirement_evidence_id)
            FROM test_case_evidence_links e WHERE e.test_case_id=t.id),'[]'::jsonb),
        'source_snapshot_id',t.source_snapshot_id
    )::text,'UTF8'),'sha256'),'hex')
    FROM test_cases t WHERE t.id=revision_id;
$$;

UPDATE test_cases SET content_hash=test_case_revision_content_hash(id),sealed_at=created_at;

WITH heads AS (
    SELECT DISTINCT ON (family_id) family_id,id,content_hash
    FROM test_cases ORDER BY family_id,version_number DESC,id DESC
)
UPDATE test_case_families f SET
    head_revision_id=h.id,
    head_token=encode(digest(convert_to(f.id || ':' || h.id || ':' || h.content_hash,
        'UTF8'),'sha256'),'hex')
FROM heads h WHERE h.family_id=f.id;

ALTER TABLE test_case_families
    ADD CONSTRAINT test_case_families_head_scope_fk
    FOREIGN KEY (head_revision_id,id,document_set_id)
    REFERENCES test_cases(id,family_id,document_set_id) DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE test_case_revision_commands (
    id BIGSERIAL PRIMARY KEY,
    family_id BIGINT NOT NULL REFERENCES test_case_families(id) ON DELETE CASCADE,
    operation TEXT NOT NULL CHECK (operation IN ('CREATE_REVISION','RESTORE')),
    idempotency_key TEXT NOT NULL,
    request_hash TEXT NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    result_revision_id BIGINT NOT NULL REFERENCES test_cases(id)
        DEFERRABLE INITIALLY DEFERRED,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (family_id,idempotency_key)
);

CREATE TABLE test_case_revision_audit (
    id BIGSERIAL PRIMARY KEY,
    family_id BIGINT NOT NULL REFERENCES test_case_families(id) ON DELETE CASCADE,
    test_case_id BIGINT REFERENCES test_cases(id) DEFERRABLE INITIALLY DEFERRED,
    event_type TEXT NOT NULL CHECK (event_type IN
        ('MIGRATED','CREATED','RESTORED','REVIEWED','ARCHIVED','UNARCHIVED')),
    actor TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata)='object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO test_case_revision_audit
    (family_id,test_case_id,event_type,actor,content_hash,reason,created_at)
SELECT family_id,id,'MIGRATED','MIGRATION',content_hash,
    'Backfilled by UV-02 without inferring or splitting legacy scenario identity',created_at
FROM test_cases;

CREATE TABLE test_case_identity_migration_issues (
    id BIGSERIAL PRIMARY KEY,
    family_id BIGINT NOT NULL REFERENCES test_case_families(id) ON DELETE CASCADE,
    issue_type TEXT NOT NULL CHECK (issue_type IN ('POSSIBLE_MIXED_SCENARIO')),
    details JSONB NOT NULL CHECK (jsonb_typeof(details)='object'),
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (family_id,issue_type)
);

INSERT INTO test_case_identity_migration_issues(family_id,issue_type,details)
SELECT f.id,'POSSIBLE_MIXED_SCENARIO',jsonb_build_object(
    'legacy_key',f.legacy_key,
    'revision_count',(SELECT count(*) FROM test_cases t WHERE t.family_id=f.id),
    'distinct_titles',(SELECT jsonb_agg(x.title ORDER BY x.title) FROM (
        SELECT DISTINCT trim(t.title) AS title FROM test_cases t WHERE t.family_id=f.id) x),
    'action','Manual review required; migration did not merge or split revisions')
FROM test_case_families f WHERE f.needs_identity_review;

ALTER TABLE test_case_reviews
    ADD COLUMN content_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN actor TEXT NOT NULL DEFAULT 'LEGACY_REVIEWER';

UPDATE test_case_reviews r SET content_hash=t.content_hash FROM test_cases t
WHERE t.id=r.test_case_id;

CREATE OR REPLACE FUNCTION ensure_test_case_family_on_legacy_insert()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE selected_family BIGINT;
BEGIN
    IF NEW.family_id IS NULL THEN
        SELECT id INTO selected_family FROM test_case_families
        WHERE test_suite_id=NEW.test_suite_id AND
              (public_key=NEW.test_case_key OR legacy_key=NEW.test_case_key)
        ORDER BY id LIMIT 1;
        IF selected_family IS NULL THEN
            INSERT INTO test_case_families
                (test_suite_id,document_set_id,public_key,legacy_key,head_token)
            VALUES(NEW.test_suite_id,NEW.document_set_id,NEW.test_case_key,NEW.test_case_key,
                encode(digest(convert_to('legacy-insert:' || NEW.test_suite_id || ':' ||
                    NEW.test_case_key,'UTF8'),'sha256'),'hex'))
            RETURNING id INTO selected_family;
        END IF;
        NEW.family_id := selected_family;
    END IF;
    RETURN NEW;
END;
$$;

ALTER TABLE test_cases ALTER COLUMN family_id DROP NOT NULL;
CREATE TRIGGER test_cases_legacy_family_before_insert
BEFORE INSERT ON test_cases FOR EACH ROW EXECUTE FUNCTION ensure_test_case_family_on_legacy_insert();
ALTER TABLE test_cases ALTER COLUMN family_id SET NOT NULL;

CREATE OR REPLACE FUNCTION update_test_case_family_head_on_insert()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE test_case_families SET
        revision_counter=GREATEST(revision_counter,NEW.version_number),
        head_revision_id=CASE WHEN NEW.version_number>=revision_counter THEN NEW.id ELSE head_revision_id END,
        head_token=CASE WHEN NEW.version_number>=revision_counter THEN
            encode(digest(convert_to(id || ':' || NEW.id || ':' || NEW.content_hash,
                'UTF8'),'sha256'),'hex') ELSE head_token END,
        updated_at=NOW()
    WHERE id=NEW.family_id;
    RETURN NEW;
END;
$$;

CREATE TRIGGER test_cases_family_head_after_insert
AFTER INSERT ON test_cases FOR EACH ROW EXECUTE FUNCTION update_test_case_family_head_on_insert();

CREATE OR REPLACE FUNCTION protect_test_case_revision_content()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='DELETE' THEN
        IF pg_trigger_depth()>1 OR current_setting('app.allow_testcase_graph_delete',TRUE)='on' THEN
            RETURN OLD;
        END IF;
        RAISE EXCEPTION 'testcase revisions are immutable; archive the family or purge its document set';
    END IF;
    IF OLD.sealed_at IS NOT NULL AND (
        NEW.test_suite_id IS DISTINCT FROM OLD.test_suite_id OR
        NEW.document_set_id IS DISTINCT FROM OLD.document_set_id OR
        NEW.family_id IS DISTINCT FROM OLD.family_id OR
        NEW.test_case_key IS DISTINCT FROM OLD.test_case_key OR
        NEW.version_number IS DISTINCT FROM OLD.version_number OR
        NEW.title IS DISTINCT FROM OLD.title OR
        NEW.test_type IS DISTINCT FROM OLD.test_type OR
        NEW.risk IS DISTINCT FROM OLD.risk OR
        NEW.actor IS DISTINCT FROM OLD.actor OR
        NEW.precondition IS DISTINCT FROM OLD.precondition OR
        NEW.test_data IS DISTINCT FROM OLD.test_data OR
        NEW.expected_result IS DISTINCT FROM OLD.expected_result OR
        NEW.postcondition IS DISTINCT FROM OLD.postcondition OR
        NEW.assumptions IS DISTINCT FROM OLD.assumptions OR
        NEW.parent_revision_id IS DISTINCT FROM OLD.parent_revision_id OR
        NEW.restored_from_revision_id IS DISTINCT FROM OLD.restored_from_revision_id OR
        NEW.source_snapshot_id IS DISTINCT FROM OLD.source_snapshot_id OR
        NEW.content_hash IS DISTINCT FROM OLD.content_hash OR
        NEW.provenance IS DISTINCT FROM OLD.provenance
    ) THEN
        RAISE EXCEPTION 'testcase revision content is immutable; create a new revision';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER test_cases_revision_content_immutable
BEFORE UPDATE OR DELETE ON test_cases
FOR EACH ROW EXECUTE FUNCTION protect_test_case_revision_content();

CREATE OR REPLACE FUNCTION protect_test_case_revision_child()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE owner_id BIGINT; owner_sealed TIMESTAMPTZ;
BEGIN
    owner_id := CASE WHEN TG_OP='DELETE' THEN OLD.test_case_id ELSE NEW.test_case_id END;
    SELECT sealed_at INTO owner_sealed FROM test_cases WHERE id=owner_id;
    IF owner_sealed IS NOT NULL AND NOT (
        TG_OP='DELETE' AND (pg_trigger_depth()>1 OR
            current_setting('app.allow_testcase_graph_delete',TRUE)='on')
    ) THEN
        RAISE EXCEPTION 'sealed testcase revision children are immutable';
    END IF;
    RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END;
$$;

CREATE TRIGGER test_case_steps_revision_immutable
BEFORE INSERT OR UPDATE OR DELETE ON test_case_steps
FOR EACH ROW EXECUTE FUNCTION protect_test_case_revision_child();
CREATE TRIGGER test_case_requirement_links_revision_immutable
BEFORE INSERT OR UPDATE OR DELETE ON test_case_requirement_links
FOR EACH ROW EXECUTE FUNCTION protect_test_case_revision_child();
CREATE TRIGGER test_case_evidence_links_revision_immutable
BEFORE INSERT OR UPDATE OR DELETE ON test_case_evidence_links
FOR EACH ROW EXECUTE FUNCTION protect_test_case_revision_child();

DROP TRIGGER IF EXISTS test_cases_approval_evidence ON test_cases;
CREATE OR REPLACE FUNCTION enforce_test_case_approval_evidence()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.status='APPROVED' AND OLD.status IS DISTINCT FROM 'APPROVED' THEN
        IF NEW.sealed_at IS NULL OR NEW.content_hash=repeat('0',64) OR
           trim(NEW.expected_result)='' OR
           NOT EXISTS (SELECT 1 FROM test_case_requirement_links l WHERE l.test_case_id=NEW.id) OR
           EXISTS (SELECT 1 FROM test_case_steps s
                   WHERE s.test_case_id=NEW.id AND trim(s.expected_result)='') OR
           EXISTS (SELECT 1 FROM test_case_requirement_links l
                   JOIN requirements r ON r.id=l.requirement_id
                   WHERE l.test_case_id=NEW.id AND
                     (r.document_set_id<>NEW.document_set_id OR r.status<>'APPROVED' OR
                      NOT EXISTS (SELECT 1 FROM test_case_evidence_links e
                                  WHERE e.test_case_id=NEW.id AND e.requirement_id=r.id))) OR
           EXISTS (SELECT 1 FROM test_case_evidence_links e
                   JOIN document_versions v ON v.id=e.document_version_id
                   WHERE e.test_case_id=NEW.id AND
                     (e.document_set_id<>NEW.document_set_id OR v.approval_status<>'APPROVED')) THEN
            RAISE EXCEPTION 'approved testcase revisions require complete approved evidence for case and step expectations';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER test_cases_approval_evidence
BEFORE UPDATE ON test_cases
FOR EACH ROW EXECUTE FUNCTION enforce_test_case_approval_evidence();
