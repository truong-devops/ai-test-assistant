ALTER TABLE requirements
    ADD COLUMN stable_identifier TEXT NOT NULL DEFAULT '',
    ADD COLUMN source_state TEXT NOT NULL DEFAULT 'CURRENT' CHECK(source_state IN ('CURRENT','HISTORICAL','REMOVED'));
UPDATE requirements SET stable_identifier=COALESCE(raw_payload->>'identifier','');

CREATE TABLE requirement_review_commands (
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    request JSONB NOT NULL,
    result JSONB NOT NULL,
    actor TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(document_set_id,idempotency_key)
);
CREATE TABLE requirement_clarification_audit (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK(kind IN ('CONFLICT','QUESTION')),
    subject_id BIGINT NOT NULL,
    actor TEXT NOT NULL,
    resolution TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE requirement_source_comparisons (
    id BIGSERIAL PRIMARY KEY,
    document_set_id BIGINT NOT NULL REFERENCES document_sets(id) ON DELETE CASCADE,
    from_snapshot_id BIGINT REFERENCES document_source_snapshots(id),
    to_snapshot_id BIGINT NOT NULL REFERENCES document_source_snapshots(id),
    index_generation BIGINT NOT NULL,
    items JSONB NOT NULL CHECK(jsonb_typeof(items)='array'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(document_set_id,to_snapshot_id)
);

CREATE FUNCTION requirement_is_current(requirement_id BIGINT) RETURNS BOOLEAN
LANGUAGE SQL STABLE AS $$
    SELECT EXISTS(SELECT 1 FROM requirements r WHERE r.id=requirement_id
      AND r.source_state='CURRENT' AND NOT EXISTS(SELECT 1 FROM requirements n WHERE n.supersedes_requirement_id=r.id))
$$;

CREATE FUNCTION requirement_review_blockers(requirement_id BIGINT) RETURNS TEXT[]
LANGUAGE SQL STABLE AS $$
    SELECT array_remove(ARRAY[
      CASE WHEN NOT requirement_is_current(requirement_id) THEN 'STALE_REVISION' END,
      CASE WHEN EXISTS(SELECT 1 FROM requirements r JOIN document_sets s ON s.id=r.document_set_id
        WHERE r.id=requirement_id AND s.status<>'ACTIVE') THEN 'SET_INACTIVE' END,
      CASE WHEN NOT EXISTS(SELECT 1 FROM requirement_evidence e JOIN document_versions v ON v.id=e.document_version_id
        JOIN document_blocks b ON b.id=e.document_block_id AND b.document_version_id=v.id
        WHERE e.requirement_id=$1 AND v.approval_status='APPROVED' AND v.parse_status='PARSED'
          AND e.document_set_id=v.document_set_id AND b.content<>'') THEN 'MISSING_APPROVED_EVIDENCE' END,
      CASE WHEN EXISTS(SELECT 1 FROM requirement_conflicts c WHERE c.status='OPEN'
        AND (c.left_requirement_id=$1 OR c.right_requirement_id=$1)) OR
        EXISTS(SELECT 1 FROM open_questions q WHERE q.requirement_id=$1 AND q.status='OPEN') OR
        EXISTS(SELECT 1 FROM requirements r WHERE r.id=$1 AND r.status IN ('CONFLICT','TBD'))
        THEN 'CLARIFICATION_REQUIRED' END
    ],NULL)
$$;

CREATE FUNCTION requirement_review_hash(requirement_id BIGINT) RETURNS TEXT
LANGUAGE SQL STABLE AS $$
 SELECT encode(digest(convert_to(jsonb_build_object(
   'revision',to_jsonb(r)-'created_at'-'updated_at',
   'evidence',COALESCE((SELECT jsonb_agg(jsonb_build_array(e.document_version_id,e.document_block_id,e.source_locator,e.excerpt_hash,v.approval_status) ORDER BY e.id)
     FROM requirement_evidence e JOIN document_versions v ON v.id=e.document_version_id WHERE e.requirement_id=r.id),'[]'::jsonb),
   'steps',COALESCE((SELECT jsonb_agg(jsonb_build_array(s.ordinal,s.action,s.expected_result) ORDER BY s.ordinal) FROM requirement_flow_steps s WHERE s.requirement_id=r.id),'[]'::jsonb),
   'blockers',requirement_review_blockers(r.id)
 )::text,'UTF8'),'sha256'),'hex') FROM requirements r WHERE r.id=requirement_id
$$;

CREATE OR REPLACE FUNCTION protect_reviewed_requirement_proof() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target BIGINT;
BEGIN
 IF TG_TABLE_NAME='requirements' THEN
   IF EXISTS(SELECT 1 FROM requirement_reviews WHERE requirement_id=OLD.id) OR
      EXISTS(SELECT 1 FROM test_case_requirement_links WHERE requirement_id=OLD.id) THEN
     IF (to_jsonb(NEW)-'status'-'source_state'-'updated_at') IS DISTINCT FROM
        (to_jsonb(OLD)-'status'-'source_state'-'updated_at') THEN
       RAISE EXCEPTION 'reviewed requirement proof is immutable; create a revision';
     END IF;
   END IF;
   RETURN NEW;
 END IF;
 IF TG_OP='UPDATE' AND OLD.requirement_id IS DISTINCT FROM NEW.requirement_id THEN
   PERFORM 1 FROM requirements WHERE id=OLD.requirement_id FOR UPDATE;
   IF EXISTS(SELECT 1 FROM requirement_reviews WHERE requirement_id=OLD.requirement_id) OR
      EXISTS(SELECT 1 FROM test_case_requirement_links WHERE requirement_id=OLD.requirement_id) THEN
     RAISE EXCEPTION 'cannot move reviewed requirement evidence/steps';
   END IF;
 END IF;
 target := CASE WHEN TG_OP='DELETE' THEN OLD.requirement_id ELSE NEW.requirement_id END;
 -- Parent lock serializes evidence writes with review; cascaded parent deletion is allowed.
 PERFORM 1 FROM requirements WHERE id=target FOR UPDATE;
 IF FOUND AND (EXISTS(SELECT 1 FROM requirement_reviews WHERE requirement_id=target) OR
   EXISTS(SELECT 1 FROM test_case_requirement_links WHERE requirement_id=target)) THEN
   RAISE EXCEPTION 'reviewed requirement evidence/steps are immutable';
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER requirements_reviewed_proof BEFORE UPDATE ON requirements
 FOR EACH ROW EXECUTE FUNCTION protect_reviewed_requirement_proof();
CREATE TRIGGER requirement_evidence_reviewed_proof BEFORE INSERT OR UPDATE OR DELETE ON requirement_evidence
 FOR EACH ROW EXECUTE FUNCTION protect_reviewed_requirement_proof();
CREATE TRIGGER requirement_steps_reviewed_proof BEFORE INSERT OR UPDATE OR DELETE ON requirement_flow_steps
 FOR EACH ROW EXECUTE FUNCTION protect_reviewed_requirement_proof();

CREATE FUNCTION guard_requirement_clarification_approval() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.status='APPROVED' AND OLD.status<>'APPROVED' AND
   (OLD.status IN ('CONFLICT','TBD') OR EXISTS(SELECT 1 FROM requirement_conflicts c WHERE c.status='OPEN'
     AND (c.left_requirement_id=NEW.id OR c.right_requirement_id=NEW.id)) OR
     EXISTS(SELECT 1 FROM open_questions q WHERE q.requirement_id=NEW.id AND q.status='OPEN')) THEN
   RAISE EXCEPTION 'resolve requirement clarification before approval';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER requirements_clarification_approval BEFORE UPDATE ON requirements
 FOR EACH ROW EXECUTE FUNCTION guard_requirement_clarification_approval();
