-- Synthetic schema-23 fixture. Never execute on application/production databases.
DO $$
DECLARE set_id bigint; suite_id bigint; project_id bigint; doc_id bigint;
        old_id bigint; new_id bigint; independent_id bigint; run_id bigint;
BEGIN
  INSERT INTO document_sets(name) VALUES('UV09 legacy migration drill') RETURNING id INTO set_id;
  INSERT INTO documents(document_set_id,name,document_type) VALUES(set_id,'Legacy source','REQUIREMENTS') RETURNING id INTO doc_id;
  INSERT INTO document_versions(document_id,document_set_id,version_number,original_filename,media_type,size_bytes,sha256,storage_key,approval_status,parse_status)
    VALUES(doc_id,set_id,1,'legacy.md','text/markdown',16,encode(sha256(convert_to('# Legacy source' || chr(10),'UTF8')),'hex'),'legacy.md','APPROVED','PARSED');
  INSERT INTO test_suites(document_set_id,name) VALUES(set_id,'Legacy suite') RETURNING id INTO suite_id;
  INSERT INTO test_cases(test_suite_id,document_set_id,test_case_key,title,test_type,expected_result,expected_result_hash,status)
    VALUES(suite_id,set_id,'TC-LEGACY','First scenario','NEGATIVE','Expected v1',encode(sha256(convert_to('Expected v1','UTF8')),'hex'),'APPROVED') RETURNING id INTO old_id;
  INSERT INTO test_cases(test_suite_id,document_set_id,test_case_key,version_number,title,test_type,expected_result,expected_result_hash,status,supersedes_test_case_id)
    VALUES(suite_id,set_id,'TC-LEGACY',2,'Suspicious different scenario','NEGATIVE','Expected v2',encode(sha256(convert_to('Expected v2','UTF8')),'hex'),'DRAFT',old_id) RETURNING id INTO new_id;
  INSERT INTO test_cases(test_suite_id,document_set_id,test_case_key,title,test_type,expected_result,expected_result_hash,status)
    VALUES(suite_id,set_id,'TC-INDEPENDENT','Independent approved','HAPPY','Independent result',encode(sha256(convert_to('Independent result','UTF8')),'hex'),'APPROVED') RETURNING id INTO independent_id;
  INSERT INTO projects(name,provider,provider_project_id,repository_url) VALUES('UV09 migration','gitlab',909090,'https://example.invalid/fixture.git') RETURNING id INTO project_id;
  INSERT INTO project_document_baselines(project_id,document_set_id,test_suite_id,selected_by,updated_at)
    VALUES(project_id,set_id,suite_id,'Legacy QA','2001-01-01T00:00:00Z');
  INSERT INTO test_runs(test_suite_id,status) VALUES(suite_id,'COMPLETED') RETURNING id INTO run_id;
  INSERT INTO test_run_items(test_run_id,test_case_id,status,expected_result_snapshot,expected_result_hash,actual_result)
    VALUES(run_id,old_id,'PASSED','Expected v1',encode(sha256(convert_to('Expected v1','UTF8')),'hex'),'Legacy run fixture');
  INSERT INTO test_exports(document_set_id,test_suite_id,test_run_id,format,filename,content_type,content,content_hash,snapshot,snapshot_hash,row_count,generated_by)
    VALUES(set_id,suite_id,run_id,'MARKDOWN','legacy.md','text/markdown',convert_to('Legacy export fixture','UTF8'),encode(sha256(convert_to('Legacy export fixture','UTF8')),'hex'),'{}',encode(sha256(convert_to('{}','UTF8')),'hex'),1,'Legacy QA');
END $$;
