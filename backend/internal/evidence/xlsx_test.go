package evidence

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/report"
)

func workbookFixture() report.Snapshot {
	run, release := int64(3), int64(4)
	return report.Snapshot{DocumentSetID: 1, TestSuiteID: 2, TestRunID: &run, SuiteReleaseID: &release,
		DocumentSetName: "Tài liệu", ReleaseName: "Baseline", ReleaseNumber: 1, ReleaseManifestHash: "manifest",
		GeneratedAt: time.Date(2026, 9, 25, 1, 2, 3, 0, time.UTC), GeneratedBy: "QA",
		RevisionManifest: []report.RevisionRef{{FamilyID: 5, TestCaseID: 6, TestCaseKey: "TC-1", RevisionNumber: 1, ContentHash: "content"}},
		DocumentVersions: []report.DocumentVersionRef{{DocumentName: "Nguồn", VersionNumber: 1, SHA256: "sourcehash", ApprovalStatus: "APPROVED"}},
		Rows:             []report.ExecutionRow{{TestCaseID: 6, TestCaseKey: "TC-1", ExpectedResult: "expected", ActualResult: "actual", Status: "F", Steps: []string{"Bước 1", "Bước 2\nXác nhận"}, Notes: "=SUM(A1)", Evidence: []string{"local://proof"}}},
		RunHistory:       []report.RunHistoryRow{{TestCaseKey: "TC-1", RunID: 3, Attempt: 1, ActualResult: "history-actual", Status: "F", CreatedAt: time.Date(2026, 9, 25, 1, 1, 1, 0, time.UTC)}},
	}
}

func rewriteWorkbook(t *testing.T, original []byte, mutate func(map[string]string), duplicate string) []byte {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{}
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		parts[file.Name] = string(data)
	}
	mutate(parts)
	var result bytes.Buffer
	writer := zip.NewWriter(&result)
	write := func(name, content string) {
		part, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.WriteString(part, content); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range parts {
		write(name, content)
	}
	if duplicate != "" {
		write(duplicate, parts[duplicate])
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	return result.Bytes()
}

func TestWorkbookProofRejectsRechecksummedCellAndPackageTampering(t *testing.T) {
	snapshot := workbookFixture()
	content, err := report.RenderXLSX(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(snapshot)
	items := []manifestItem{{FamilyID: 5, TestCaseID: 6, ContentHash: "content", ExpectedResultHash: digest([]byte("expected"))}}
	valid := func(data []byte) bool {
		// Intentionally recompute the byte checksum: the cells must independently
		// agree with the original immutable snapshot, not just a matching digest.
		return validateExport(data, raw, digest(data), digest(raw), 1, 1, 2, 3, 4, "manifest", items)
	}
	if !valid(content) {
		t.Fatal("renderer output rejected")
	}
	replace := func(part, before, after string) func(map[string]string) {
		return func(parts map[string]string) {
			if !strings.Contains(parts[part], before) {
				t.Fatalf("fixture missing %q", before)
			}
			parts[part] = strings.Replace(parts[part], before, after, 1)
		}
	}
	for _, tc := range []struct {
		name      string
		mutate    func(map[string]string)
		duplicate string
	}{
		{"expected", replace("xl/worksheets/sheet1.xml", ">expected<", ">changed<"), ""},
		{"actual", replace("xl/worksheets/sheet1.xml", ">actual<", ">changed<"), ""},
		{"false pass", replace("xl/worksheets/sheet1.xml", ">F<", ">P<"), ""},
		{"history", replace("xl/worksheets/sheet2.xml", ">history-actual<", ">changed<"), ""},
		{"release hash", replace("xl/worksheets/sheet4.xml", ">manifest<", ">wrong<"), ""},
		{"revision", replace("xl/worksheets/sheet4.xml", "row 6 | family 5", "row 99 | family 5"), ""},
		{"duplicate coordinate", replace("xl/worksheets/sheet1.xml", `r="H2"`, `r="A2"`), ""},
		{"missing row", replace("xl/worksheets/sheet1.xml", `<row r="2">`, `<row r="3">`), ""},
		{"hidden row", replace("xl/worksheets/sheet1.xml", `<row r="2">`, `<row r="2" hidden="1">`), ""},
		{"formula injection", replace("xl/worksheets/sheet1.xml", `<is><t xml:space="preserve">actual</t></is>`, `<f>1+1</f><is><t xml:space="preserve">actual</t></is>`), ""},
		{"summary range", replace("xl/worksheets/sheet3.xml", "L2:L2", "L2:L99"), ""},
		{"summary cached value", replace("xl/worksheets/sheet3.xml", "<v>0</v>", "<v>999</v>"), ""},
		{"missing metadata", func(parts map[string]string) { delete(parts, "xl/worksheets/sheet4.xml") }, ""},
		{"malformed xml", func(parts map[string]string) { parts["xl/worksheets/sheet1.xml"] = "<worksheet" }, ""},
		{"trailing root", func(parts map[string]string) { parts["xl/worksheets/sheet1.xml"] += "<extra/>" }, ""},
		{"duplicate attribute", replace("xl/worksheets/sheet1.xml", `r="H2"`, `r="H2" r="A2"`), ""},
		{"foreign namespace", replace("xl/worksheets/sheet1.xml", `<sheetData>`, `<sheetData xmlns="urn:wrong">`), ""},
		{"sheet routing", replace("xl/_rels/workbook.xml.rels", "worksheets/sheet1.xml", "worksheets/sheet2.xml"), ""},
		{"external relationship", replace("xl/_rels/workbook.xml.rels", `Id="rId1"`, `Id="rId1" TargetMode="External"`), ""},
		{"root routing", replace("_rels/.rels", `Target="xl/workbook.xml"`, `Target="other.xml"`), ""},
		{"hidden sheet", replace("xl/workbook.xml", `name="Test Cases"`, `name="Test Cases" state="hidden"`), ""},
		{"duplicate zip entry", func(map[string]string) {}, "xl/worksheets/sheet1.xml"},
		{"path traversal", func(parts map[string]string) { parts["../escape"] = "data" }, ""},
		{"too many parts", func(parts map[string]string) {
			for i := range 33 {
				parts[strings.Repeat("x", i+1)] = "data"
			}
		}, ""},
		{"oversized part", func(parts map[string]string) {
			parts["xl/worksheets/sheet1.xml"] = strings.Repeat("x", maxXLSXPartBytes+1)
		}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := rewriteWorkbook(t, content, tc.mutate, tc.duplicate)
			if valid(changed) {
				t.Fatal("rechecksummed workbook tampering accepted")
			}
		})
	}
	if valid([]byte("PKfixture")) {
		t.Fatal("magic header accepted instead of workbook")
	}
}

func TestWorkbookCellEncodingContract(t *testing.T) {
	for _, value := range []string{"Tiếng Việt\nline\t2 & <xml>", "=SUM(A1)", "  +1", "-1", "@SUM(A1)", "a\x00b", "bad\xffutf8", strings.Repeat("đ", 32770)} {
		snapshot := workbookFixture()
		snapshot.Rows[0].ActualResult = value
		content, err := report.RenderXLSX(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if !validateWorkbook(content, snapshot) {
			t.Fatalf("cell contract rejected (%d bytes)", len(value))
		}
	}
}
