package report

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureSnapshot() Snapshot {
	return Snapshot{DocumentSetName: "Đặt hàng", ProductName: "Sàn TMĐT", Scope: "UC-B08",
		TestSuiteName: "UC-B08", GeneratedAt: time.Date(2026, 9, 11, 1, 2, 3, 0, time.UTC),
		GeneratedBy: "QA", Reviewer: "Lan", DocumentVersions: []DocumentVersionRef{{DocumentName: "PTYC", VersionNumber: 1, SHA256: strings.Repeat("a", 64), ApprovalStatus: "APPROVED"}},
		Rows: []ExecutionRow{{TestCaseKey: "TC-001", RequirementTrace: "UC-B08", Objective: "Đặt hàng thành công", Steps: []string{"Chọn sản phẩm", "Xác nhận\nđơn hàng"}, ExpectedResult: "Đơn hàng được tạo", Status: "P", ActualResult: "Thành công", AutomationStatus: "MANUAL", SourceStatus: "APPROVED"}}}
}

func TestRenderXLSXContainsRequiredSheetsColumnsAndExactSummaryRange(t *testing.T) {
	content, err := RenderXLSX(fixtureSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("xlsx is not a readable zip: %v", err)
	}
	parts := map[string]string{}
	for _, file := range reader.File {
		r, openErr := file.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		data, readErr := io.ReadAll(r)
		r.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		parts[file.Name] = string(data)
	}
	for _, sheet := range []string{"Test Cases", "Run History", "Summary", "Metadata"} {
		if !strings.Contains(parts["xl/workbook.xml"], `name="`+sheet+`"`) {
			t.Errorf("missing sheet %q", sheet)
		}
	}
	for _, header := range testCaseHeaders {
		if !strings.Contains(parts["xl/worksheets/sheet1.xml"], ">"+header+"<") {
			t.Errorf("missing column %q", header)
		}
	}
	if !strings.Contains(parts["xl/worksheets/sheet3.xml"], "L2:L2") {
		t.Errorf("summary does not use exact data range: %s", parts["xl/worksheets/sheet3.xml"])
	}
	if !strings.Contains(parts["xl/worksheets/sheet3.xml"], `COUNTIF(&#39;Test Cases&#39;!L2:L2,&#34;P&#34;)`) {
		t.Errorf("summary does not count the P management status")
	}
	if !strings.Contains(parts["xl/worksheets/sheet1.xml"], "Đặt hàng thành công") || !strings.Contains(parts["xl/worksheets/sheet1.xml"], "Xác nhận&#xA;đơn hàng") {
		t.Error("unicode or multiline content was lost")
	}
}

func TestSanitizeCellPreventsFormulaInjectionAndInvalidXML(t *testing.T) {
	for _, value := range []string{"=CMD()", "+1", "-1", "@SUM(A1)", "  =CMD()"} {
		if got := sanitizeCell(value); !strings.HasPrefix(got, "'") {
			t.Errorf("sanitizeCell(%q)=%q", value, got)
		}
	}
	if got := sanitizeCell("a\x00b"); got != "ab" {
		t.Fatalf("invalid XML rune remained: %q", got)
	}
	if got := utf8RuneCount(sanitizeCell(strings.Repeat("a", maxExcelCellRunes+10))); got != maxExcelCellRunes {
		t.Fatalf("cell length=%d", got)
	}
}

func TestRenderMarkdownEscapesTableInput(t *testing.T) {
	snapshot := fixtureSnapshot()
	snapshot.Rows[0].Objective = "A|B\nC"
	text := string(RenderMarkdown(snapshot))
	if !strings.Contains(text, `A\|B<br>C`) {
		t.Fatalf("markdown input was not escaped: %s", text)
	}
}

func TestRenderXLSXOpensWithLibreOffice(t *testing.T) {
	soffice, err := exec.LookPath("soffice")
	if err != nil {
		t.Skip("LibreOffice is not installed")
	}
	if output, err := exec.Command(soffice, "--version").CombinedOutput(); err != nil {
		t.Skipf("LibreOffice launcher is unavailable: %v (%s)", err, output)
	}
	content, err := RenderXLSX(fixtureSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	inputDirectory, outputDirectory := t.TempDir(), t.TempDir()
	input := filepath.Join(inputDirectory, "report.xlsx")
	if err := os.WriteFile(input, content, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(soffice, "--headless", "--convert-to", "xlsx", "--outdir", outputDirectory, input)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("LibreOffice could not open the workbook: %v\n%s", err, output)
	}
	converted, err := os.Stat(filepath.Join(outputDirectory, "report.xlsx"))
	if err != nil || converted.Size() == 0 {
		t.Fatalf("LibreOffice did not produce a readable workbook: %v", err)
	}
}

func utf8RuneCount(value string) int { return len([]rune(value)) }
