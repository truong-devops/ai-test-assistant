// Isolated synthetic migration/restore drill against the DEVELOPMENT compose DB.
// Retains all databases/files; never restores into or deletes an existing target.
const { spawnSync } = require("node:child_process");
const fs = require("node:fs");
const path = require("node:path");
const os = require("node:os");
const crypto = require("node:crypto");
const root = path.resolve(__dirname, "..");
if (process.env.UV09_ALLOW_TEST_WRITES !== "yes")
  throw new Error(
    "Set UV09_ALLOW_TEST_WRITES=yes for isolated synthetic fixtures.",
  );
const suffix = `${Date.now()}_${crypto.randomBytes(3).toString("hex")}`;
const original = `uv09_migration_${suffix}`;
const restored = `uv09_restore_${suffix}`;
const empty = `uv09_empty_${suffix}`;
const artifacts = fs.mkdtempSync(
  path.join(os.tmpdir(), "ai-test-uv09-migration-"),
);
const compose = [
  "compose",
  "-f",
  path.join(root, "infra/compose/docker-compose.yml"),
];
function docker(args, input) {
  const result = spawnSync("docker", [...compose, ...args], {
    cwd: root,
    input,
    maxBuffer: 64 * 1024 * 1024,
  });
  if (result.status !== 0)
    throw new Error(
      result.stderr.toString() || `Docker command failed: ${args[0]}`,
    );
  return result.stdout;
}
function sql(database, query) {
  if (![original, restored, empty].includes(database))
    throw new Error("Unowned database refused");
  return docker(
    [
      "exec",
      "-T",
      "postgres",
      "psql",
      "-X",
      "-qAt",
      "-v",
      "ON_ERROR_STOP=1",
      "-U",
      "postgres",
      "-d",
      database,
    ],
    query,
  )
    .toString()
    .trim();
}
function migrate(...args) {
  migrateDatabase(original, ...args);
}
function migrateDatabase(database, ...args) {
  if (![original, restored, empty].includes(database))
    throw new Error("Unowned migration database refused");
  docker([
    "run",
    "--rm",
    "-T",
    "migrate",
    "-path=/migrations",
    `-database=postgres://postgres:postgres@postgres:5432/${database}?sslmode=disable`,
    ...args,
  ]);
}
function requireTrue(database, name, query) {
  if (sql(database, query) !== "t")
    throw new Error(`${name} failed on ${database}`);
}
docker(["exec", "-T", "postgres", "createdb", "-U", "postgres", original]);
migrate("up", "23");
sql(original, fs.readFileSync(path.join(__dirname, "uv09-legacy-fixture.sql")));
const before = sql(
  original,
  `SELECT jsonb_build_object('ids',array_agg(id ORDER BY id),'expected',array_agg(expected_result_hash ORDER BY id)) FROM test_cases;`,
);
const legacyDump = docker([
  "exec",
  "-T",
  "postgres",
  "pg_dump",
  "-U",
  "postgres",
  "-Fc",
  "--no-owner",
  "--no-privileges",
  original,
]);
fs.writeFileSync(path.join(artifacts, "schema23.dump"), legacyDump, {
  mode: 0o600,
});
migrate("up", "7"); // Stop at schema 30, before the timestamp correction.
requireTrue(
  original,
  "reproduced migration 25 timestamp defect",
  `SELECT count(*)=1 AND bool_and(published_at='2001-01-01T00:00:00Z'::timestamptz) FROM test_suite_releases;`,
);
// A metadata-only USER_PUBLISHED sentinel exercises the origin guard. It is not
// used as evidence of an actual reviewer publication or as a valid demo release.
sql(
  original,
  `INSERT INTO test_suite_releases(test_suite_id,document_set_id,release_number,name,manifest_hash,request_hash,scope_status,published_by,idempotency_key,origin,published_at,created_at)
 SELECT test_suite_id,document_set_id,2,'UV09 user timestamp sentinel',repeat('d',64),repeat('e',64),'PARTIAL','QA','uv09-user-sentinel','USER_PUBLISHED','2002-01-01T00:00:00Z','2002-01-01T00:00:00Z' FROM test_suite_releases;`,
);
const userRelease = sql(
  original,
  `SELECT to_jsonb(r) FROM test_suite_releases r WHERE origin='USER_PUBLISHED';`,
);
const stableProofQuery = `SELECT jsonb_build_object(
 'releases',(SELECT jsonb_agg(to_jsonb(r)-'published_at'-'created_at' ORDER BY id) FROM test_suite_releases r),
 'items',(SELECT jsonb_agg(to_jsonb(i)-'created_at' ORDER BY release_id,ordinal) FROM test_suite_release_items i),
 'bindings',(SELECT jsonb_agg(to_jsonb(b) ORDER BY project_id) FROM project_document_baselines b),
 'runs',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM test_runs r),
 'run_items',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM test_run_items r),
 'exports',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM test_exports e));`;
const stableProof = sql(original, stableProofQuery);
const correctionStart = sql(original, `SELECT clock_timestamp();`);
migrate("up");
const correctionEnd = sql(original, `SELECT clock_timestamp();`);
if (sql(original, stableProofQuery) !== stableProof)
  throw new Error(
    "Timestamp correction changed release identity, manifest or historical proof",
  );
if (
  sql(
    original,
    `SELECT to_jsonb(r) FROM test_suite_releases r WHERE origin='USER_PUBLISHED';`,
  ) !== userRelease
)
  throw new Error("Timestamp correction changed a user-published release");
const timestampGraphQuery = `SELECT jsonb_build_object('releases',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM test_suite_releases r),'items',(SELECT jsonb_agg(to_jsonb(i) ORDER BY release_id,ordinal) FROM test_suite_release_items i),'audits',(SELECT jsonb_agg(to_jsonb(a) ORDER BY release_id) FROM test_suite_release_timestamp_audits a));`;
const correctedGraph = sql(original, timestampGraphQuery);
migrate("up"); // At latest version, re-running must be a no-op.
if (sql(original, timestampGraphQuery) !== correctedGraph)
  throw new Error("Repeat migrate changed timestamp correction/audit");
requireTrue(
  original,
  "stable legacy IDs/expected",
  `SELECT jsonb_build_object('ids',array_agg(id ORDER BY id),'expected',array_agg(expected_result_hash ORDER BY id)) = '${before.replaceAll("'", "''")}'::jsonb FROM test_cases;`,
);
const checks = [
  [
    "mixed scenario flagged",
    `SELECT count(*)=1 FROM test_case_families WHERE legacy_key='TC-LEGACY' AND needs_identity_review;`,
  ],
  [
    "migration issue retained",
    `SELECT count(*)>0 FROM test_case_identity_migration_issues;`,
  ],
  [
    "exact old binding, no hidden-v1 resurrection",
    `SELECT count(*)=1 AND bool_and(t.test_case_key='TC-INDEPENDENT') FROM test_suite_release_items i JOIN test_cases t ON t.id=i.test_case_id;`,
  ],
  [
    "migrated origin explicit",
    `SELECT count(*)=1 FROM test_suite_releases WHERE origin='MIGRATED_CURRENT_STATE';`,
  ],
  [
    "migration timestamp correction with original audit",
    `SELECT count(*)=1 AND bool_and(a.migration_version=31 AND a.previous_published_at='2001-01-01T00:00:00Z'::timestamptz AND a.previous_created_at=a.previous_published_at
      AND r.published_at=a.corrected_at AND r.created_at=a.corrected_at
      AND a.corrected_at BETWEEN '${correctionStart}'::timestamptz AND '${correctionEnd}'::timestamptz
      AND jsonb_array_length(a.previous_item_created_at)=1
      AND (a.previous_item_created_at->0->>'created_at')::timestamptz=a.previous_created_at
      AND NOT EXISTS(SELECT 1 FROM test_suite_release_items i WHERE i.release_id=r.id AND i.created_at<>a.corrected_at))
      FROM test_suite_release_timestamp_audits a JOIN test_suite_releases r ON r.id=a.release_id;`,
  ],
  [
    "user publication timestamp untouched",
    `SELECT count(*)=1 AND bool_and(published_at='2002-01-01T00:00:00Z'::timestamptz AND created_at=published_at) FROM test_suite_releases WHERE origin='USER_PUBLISHED';`,
  ],
  [
    "immutable protections enabled",
    `SELECT count(*)=3 AND bool_and(tgenabled='O') FROM pg_trigger WHERE tgname IN ('test_suite_releases_immutable','test_suite_release_items_immutable','test_suite_release_timestamp_audits_immutable');`,
  ],
  [
    "legacy run not fabricated into release",
    `SELECT count(*)=1 AND bool_and(suite_release_id IS NULL) FROM test_runs;`,
  ],
  [
    "legacy pass and expected retained",
    `SELECT count(*)=1 AND bool_and(i.status='PASSED' AND i.expected_result_snapshot='Expected v1' AND t.version_number=1) FROM test_run_items i JOIN test_cases t ON t.id=i.test_case_id;`,
  ],
  [
    "content backfill checksum",
    `SELECT bool_and(content_hash=test_case_revision_content_hash(id)) FROM test_cases;`,
  ],
  [
    "export bytes checksum",
    `SELECT count(*)=1 AND bool_and(content_hash=encode(sha256(content),'hex')) FROM test_exports;`,
  ],
  [
    "all FK constraints validated",
    `SELECT NOT EXISTS(SELECT 1 FROM pg_constraint WHERE contype='f' AND NOT convalidated);`,
  ],
];
for (const [name, query] of checks) requireTrue(original, name, query);
sql(
  original,
  fs.readFileSync(path.join(__dirname, "uv09-migration-assertions.sql")),
);
sql(
  original,
  `DO $$ DECLARE blocked boolean; BEGIN
 blocked:=FALSE;
 BEGIN UPDATE test_suite_releases SET published_at=NOW(); EXCEPTION WHEN raise_exception THEN blocked:=TRUE; END;
 IF NOT blocked THEN RAISE EXCEPTION 'release metadata was mutable after correction'; END IF;
 blocked:=FALSE;
 BEGIN UPDATE test_suite_release_items SET created_at=NOW(); EXCEPTION WHEN raise_exception THEN blocked:=TRUE; END;
 IF NOT blocked THEN RAISE EXCEPTION 'release items were mutable after correction'; END IF;
 blocked:=FALSE;
 BEGIN UPDATE test_suite_release_timestamp_audits SET reason='tampered'; EXCEPTION WHEN raise_exception THEN blocked:=TRUE; END;
 IF NOT blocked THEN RAISE EXCEPTION 'timestamp audit was mutable'; END IF;
 END $$;`,
);
try {
  sql(
    original,
    fs.readFileSync(
      path.join(
        root,
        "backend/migrations/000031_migrated_release_timestamp_audit.down.sql",
      ),
    ),
  );
  throw new Error("Populated downgrade unexpectedly succeeded");
} catch (error) {
  if (!error.message.includes("cannot downgrade migration 31")) throw error;
}
for (const [name, query] of checks) requireTrue(original, name, query);
// Add schema-30 proposal/checkpoint/receipt records to exercise new graph backup.
sql(
  original,
  `DO $$ DECLARE jid bigint;uid bigint;pid bigint;sid bigint;tid bigint; BEGIN
 SELECT id INTO sid FROM document_sets; SELECT id INTO tid FROM test_suites;
 INSERT INTO document_workflow_jobs(document_set_id,operation,input_snapshot,input_hash,requested_by,idempotency_key,status) VALUES(sid,'GENERATE_TESTCASES','{}',repeat('a',64),'fixture','fixture','SUCCEEDED') RETURNING id INTO jid;
 INSERT INTO document_workflow_job_units(workflow_job_id,unit_key,input_hash,status) VALUES(jid,'retire',repeat('a',64),'SUCCEEDED') RETURNING id INTO uid;
 INSERT INTO test_case_generation_proposals(document_set_id,test_suite_id,workflow_job_id,workflow_unit_id,proposal_key,classification,reason,content,content_hash,generation,candidates,source_revision,status,decision,decision_reason,decided_by,decided_at)
 VALUES(sid,tid,jid,uid,'fixture','NEW_CASE','restore fixture','{}',repeat('b',64),'{}','[]',1,'DISMISSED','DISMISS','restore fixture','fixture',NOW()) RETURNING id INTO pid;
 INSERT INTO test_case_proposal_commands(document_set_id,idempotency_key,proposal_id,request_hash,result,actor) VALUES(sid,'fixture',pid,repeat('c',64),'{}','fixture'); END $$;`,
);
const graphQuery = `SELECT jsonb_build_object('cases',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM test_cases t),'release',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM test_suite_releases t),'items',(SELECT jsonb_agg(to_jsonb(t) ORDER BY release_id,ordinal) FROM test_suite_release_items t),'run',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM test_run_items t),'export',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM test_exports t),'proposals',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM test_case_generation_proposals t),'receipts',(SELECT jsonb_agg(to_jsonb(t) ORDER BY idempotency_key) FROM test_case_proposal_commands t),'units',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM document_workflow_job_units t));`;
const fullGraphQuery = `SELECT jsonb_build_object('graph',(${graphQuery.replace(/;$/, "")}), 'timestamp_audits',(SELECT jsonb_agg(to_jsonb(a) ORDER BY release_id) FROM test_suite_release_timestamp_audits a));`;
const graph = sql(original, fullGraphQuery);
const dump = docker([
  "exec",
  "-T",
  "postgres",
  "pg_dump",
  "-U",
  "postgres",
  "-Fc",
  "--no-owner",
  "--no-privileges",
  original,
]);
fs.writeFileSync(path.join(artifacts, "schema31.dump"), dump, { mode: 0o600 });
// The fixture file is deliberately passive and matches the stored version hash.
const files = path.join(artifacts, "documents");
fs.mkdirSync(files);
fs.writeFileSync(path.join(files, "legacy.md"), "# Legacy source\n");
const tar = spawnSync("tar", [
  "-czf",
  path.join(artifacts, "documents.tar.gz"),
  "-C",
  files,
  ".",
]);
if (tar.status !== 0) throw new Error("Document archive failed");
docker(["exec", "-T", "postgres", "createdb", "-U", "postgres", restored]);
docker(
  [
    "exec",
    "-T",
    "postgres",
    "pg_restore",
    "-U",
    "postgres",
    "--no-owner",
    "--no-privileges",
    "--exit-on-error",
    "--single-transaction",
    "-d",
    restored,
  ],
  dump,
);
if (sql(restored, fullGraphQuery) !== graph)
  throw new Error("Restored graph differs from backed up graph");
for (const [name, query] of checks) requireTrue(restored, name, query);
sql(
  restored,
  fs.readFileSync(path.join(__dirname, "uv09-migration-assertions.sql")),
);
const recovered = path.join(artifacts, "restored-documents");
fs.mkdirSync(recovered);
if (
  spawnSync("tar", [
    "-xzf",
    path.join(artifacts, "documents.tar.gz"),
    "-C",
    recovered,
  ]).status !== 0
)
  throw new Error("Document restore failed");
const fileHash = crypto
  .createHash("sha256")
  .update(fs.readFileSync(path.join(recovered, "legacy.md")))
  .digest("hex");
requireTrue(
  restored,
  "restored source bytes",
  `SELECT bool_and(sha256='${fileHash}') FROM document_versions;`,
);
const result = {
  status: "PASS",
  original,
  restored,
  empty,
  artifacts,
  checks: [
    ...checks.map(([name]) => name),
    "non-timestamp graph unchanged",
    "repeat migrate no-op",
    "immutable writes refused",
    "populated downgrade refused",
    "empty schema31 down/up",
    "restored graph/audit/source bytes",
  ],
  scope:
    "synthetic schema23-to-30-to-31 and empty-target restore, not production rollback",
};
// No correction audit exists on an empty install, so its down/up is reversible.
docker(["exec", "-T", "postgres", "createdb", "-U", "postgres", empty]);
migrateDatabase(empty, "up");
migrateDatabase(empty, "down", "1");
requireTrue(
  empty,
  "empty downgrade removes only schema31 audit",
  `SELECT to_regclass('test_suite_release_timestamp_audits') IS NULL AND to_regclass('test_suite_releases') IS NOT NULL;`,
);
migrateDatabase(empty, "up");
requireTrue(
  empty,
  "empty up restores schema31 audit",
  `SELECT (SELECT count(*) FROM test_suite_release_timestamp_audits)=0 AND (SELECT version=31 AND NOT dirty FROM schema_migrations);`,
);
fs.writeFileSync(
  path.join(artifacts, "result.json"),
  JSON.stringify(result, null, 2),
);
console.log(JSON.stringify(result, null, 2));
