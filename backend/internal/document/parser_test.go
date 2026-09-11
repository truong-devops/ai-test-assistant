package document

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestStructuredParserParsesMarkdownWithStableLocators(t *testing.T) {
	input := []byte("# Đặt hàng\n\nKhách hàng đã đăng nhập.\n\n- Chọn sản phẩm\n- Xác nhận\n\n| Mã | Kết quả |\n| --- | --- |\n| A1 | Thành công |\n\n```json\n{\"quantity\": 1}\n```\n\n## Ngoại lệ\nHết hàng.")
	blocks, err := NewStructuredParser().Parse(context.Background(), input, MediaTypeMarkdown, "requirements.md")
	if err != nil {
		t.Fatal(err)
	}
	wantTypes := []string{BlockHeading, BlockParagraph, BlockList, BlockTable, BlockCode, BlockHeading, BlockParagraph}
	if len(blocks) != len(wantTypes) {
		t.Fatalf("block count=%d, want %d: %#v", len(blocks), len(wantTypes), blocks)
	}
	for index, want := range wantTypes {
		if blocks[index].BlockType != want || blocks[index].SourceLocator == "" {
			t.Fatalf("block %d=%+v, want type=%s and locator", index, blocks[index], want)
		}
	}
	if blocks[0].SourceLocator != "line:1" || blocks[3].SourceLocator != "lines:8-10" {
		t.Fatalf("unexpected locators: heading=%q table=%q", blocks[0].SourceLocator, blocks[3].SourceLocator)
	}
	path, ok := blocks[6].Metadata["heading_path"].([]string)
	if !ok || len(path) != 2 || path[0] != "Đặt hàng" || path[1] != "Ngoại lệ" {
		t.Fatalf("heading path=%#v", blocks[6].Metadata["heading_path"])
	}
}

func TestStructuredParserRejectsInvalidMarkdown(t *testing.T) {
	parser := NewStructuredParser()
	for name, input := range map[string][]byte{
		"empty":    []byte(" \n\t"),
		"not-utf8": {0xff, 0xfe},
		"nul":      []byte("valid\x00invalid"),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parser.Parse(context.Background(), input, MediaTypeMarkdown, "input.md")
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error=%v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestMarkdownPreservesMainAlternateAndExceptionFlowSections(t *testing.T) {
	input := []byte("# UC-B08\n## Luồng chính\n1. Xác nhận đơn\n## Luồng thay thế\n1. Đổi địa chỉ\n## Luồng ngoại lệ\n1. Sản phẩm hết hàng")
	blocks, err := NewStructuredParser().Parse(context.Background(), input, MediaTypeMarkdown, "uc-b08.md")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"UC-B08", "Luồng chính", "1. Xác nhận đơn", "Luồng thay thế", "1. Đổi địa chỉ", "Luồng ngoại lệ", "1. Sản phẩm hết hàng"}
	if len(blocks) != len(want) {
		t.Fatalf("blocks=%+v", blocks)
	}
	for index := range want {
		if blocks[index].Content != want[index] || blocks[index].SourceLocator == "" {
			t.Fatalf("block %d=%+v, want content %q", index, blocks[index], want[index])
		}
	}
}

func TestStructuredParserParsesDOCXParagraphsHeadingsAndTables(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Đặt hàng</w:t></w:r></w:p>
<w:p><w:r><w:t>Người mua xác nhận</w:t></w:r><w:r><w:tab/><w:t>đơn hàng</w:t></w:r></w:p>
<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr><w:r><w:t>Chọn sản phẩm</w:t></w:r></w:p>
<w:tbl><w:tr><w:tc><w:p><w:r><w:t>Trạng thái</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Kết quả</w:t></w:r></w:p></w:tc></w:tr>
<w:tr><w:tc><w:p><w:r><w:t>PAID</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Thành công</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
</w:body></w:document>`
	data := makeDOCX(t, map[string]string{"[Content_Types].xml": "<Types/>", "word/document.xml": documentXML})
	blocks, err := NewStructuredParser().Parse(context.Background(), data, MediaTypeDOCX, "requirements.docx")
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 4 {
		t.Fatalf("blocks=%#v, want 4", blocks)
	}
	if blocks[0].BlockType != BlockHeading || blocks[0].HeadingLevel != 1 || blocks[0].Content != "Đặt hàng" {
		t.Fatalf("heading=%+v", blocks[0])
	}
	if blocks[1].Content != "Người mua xác nhận\tđơn hàng" || blocks[1].SourceLocator != "word/body/p[2]" {
		t.Fatalf("paragraph=%+v", blocks[1])
	}
	if blocks[2].BlockType != BlockList || blocks[2].Content != "Chọn sản phẩm" {
		t.Fatalf("list=%+v", blocks[2])
	}
	if blocks[3].BlockType != BlockTable || blocks[3].SourceLocator != "word/body/table[1]" ||
		blocks[3].Content != "| Trạng thái | Kết quả |\n| PAID | Thành công |" {
		t.Fatalf("table=%+v", blocks[3])
	}
}

func TestStructuredParserRejectsUnsafeDOCX(t *testing.T) {
	parser := NewStructuredParser()
	for name, entries := range map[string]map[string]string{
		"traversal": {"../word/document.xml": "<document/>"},
		"macro":     {"word/document.xml": "<document/>", "word/vbaProject.bin": "macro"},
		"missing":   {"[Content_Types].xml": "<Types/>"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parser.Parse(context.Background(), makeDOCX(t, entries), MediaTypeDOCX, "unsafe.docx")
			if err == nil {
				t.Fatal("error=nil, want unsafe or invalid DOCX error")
			}
		})
	}
}

func TestStructuredParserHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewStructuredParser().Parse(ctx, []byte("# title"), MediaTypeMarkdown, "input.md")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want context.Canceled", err)
	}
}

func TestStructuredParserExternalFixturesAreDeterministic(t *testing.T) {
	fixtures := []struct {
		environment string
		mediaType   string
	}{
		{"DOCUMENT_FIXTURE_DOCX", MediaTypeDOCX},
		{"DOCUMENT_FIXTURE_MARKDOWN", MediaTypeMarkdown},
	}
	parser := NewStructuredParser()
	checked := 0
	for _, fixture := range fixtures {
		path := os.Getenv(fixture.environment)
		if path == "" {
			continue
		}
		checked++
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", fixture.environment, err)
		}
		first, err := parser.Parse(context.Background(), data, fixture.mediaType, path)
		if err != nil {
			t.Fatalf("parse %s: %v", fixture.environment, err)
		}
		second, err := parser.Parse(context.Background(), data, fixture.mediaType, path)
		if err != nil {
			t.Fatalf("parse %s again: %v", fixture.environment, err)
		}
		if len(first) == 0 || !reflect.DeepEqual(first, second) {
			t.Fatalf("fixture %s produced empty or non-deterministic output", fixture.environment)
		}
		counts := map[string]int{}
		for index, block := range first {
			counts[block.BlockType]++
			if block.Content == "" || block.SourceLocator == "" {
				t.Fatalf("fixture %s block %d lacks content or locator: %+v", fixture.environment, index, block)
			}
		}
		if counts[BlockHeading] == 0 || counts[BlockTable] == 0 {
			t.Fatalf("fixture %s lacks headings or tables: %v", fixture.environment, counts)
		}
		t.Logf("%s: %d deterministic blocks (%v)", fixture.environment, len(first), counts)
	}
	if checked == 0 {
		t.Skip("DOCUMENT_FIXTURE_DOCX and DOCUMENT_FIXTURE_MARKDOWN are not set")
	}
}

func makeDOCX(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, contents := range entries {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
