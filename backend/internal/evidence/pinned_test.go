package evidence

import (
	"encoding/json"
	"testing"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/report"
)

func TestExportProofRejectsTamperingAndWrongRevision(t *testing.T) {
	runID, releaseID := int64(3), int64(4)
	items := []manifestItem{{FamilyID: 5, TestCaseID: 6, ContentHash: "content", ExpectedResultHash: digest([]byte("expected"))}}
	snapshot := report.Snapshot{DocumentSetID: 1, TestSuiteID: 2, TestRunID: &runID, SuiteReleaseID: &releaseID, ReleaseManifestHash: "manifest",
		RevisionManifest: []report.RevisionRef{{FamilyID: 5, TestCaseID: 6, ContentHash: "content"}}, Rows: []report.ExecutionRow{{TestCaseID: 6, ExpectedResult: "expected"}}}
	bytes, err := report.RenderXLSX(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	valid := func(s report.Snapshot, data []byte, contentHash, snapshotHash string, count int) bool {
		raw, _ := json.Marshal(s)
		return validateExport(data, raw, contentHash, snapshotHash, count, 1, 2, 3, 4, "manifest", items)
	}
	raw, _ := json.Marshal(snapshot)
	contentHash, snapshotHash := digest(bytes), digest(raw)
	if !valid(snapshot, bytes, contentHash, snapshotHash, 1) {
		t.Fatal("valid pinned fixture rejected")
	}
	if valid(snapshot, []byte("PKtampered"), contentHash, snapshotHash, 1) {
		t.Fatal("changed export bytes accepted")
	}
	if valid(snapshot, bytes, contentHash, "wrong", 1) {
		t.Fatal("changed snapshot checksum accepted")
	}
	if valid(snapshot, bytes, contentHash, snapshotHash, 2) {
		t.Fatal("missing row accepted")
	}
	for _, mutate := range []func(*report.Snapshot){
		func(s *report.Snapshot) { v := int64(99); s.SuiteReleaseID = &v },
		func(s *report.Snapshot) { s.RevisionManifest[0].TestCaseID = 99 },
		func(s *report.Snapshot) { s.Rows[0].ExpectedResult = "new expected" },
		func(s *report.Snapshot) {
			s.Rows = append(s.Rows, s.Rows[0])
			s.RevisionManifest = append(s.RevisionManifest, s.RevisionManifest[0])
		},
	} {
		var changed report.Snapshot
		_ = json.Unmarshal(raw, &changed)
		mutate(&changed)
		encoded, _ := json.Marshal(changed)
		if valid(changed, bytes, contentHash, digest(encoded), len(changed.Rows)) {
			t.Fatal("internally checksummed but mismatched proof accepted")
		}
	}
}
