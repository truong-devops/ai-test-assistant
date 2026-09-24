ALTER TABLE test_case_generation_proposals ADD COLUMN workflow_unit_id BIGINT
    REFERENCES document_workflow_job_units(id) ON DELETE RESTRICT;
CREATE INDEX testcase_proposal_unit_idx ON test_case_generation_proposals(workflow_unit_id);
