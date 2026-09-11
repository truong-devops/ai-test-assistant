package document

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxDOCXEntries           = 2048
	maxDOCXUncompressedBytes = 64 << 20
	maxDOCXXMLBytes          = 32 << 20
)

var (
	markdownHeading     = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)
	markdownList        = regexp.MustCompile(`^\s*(?:[-+*]|\d+[.)])\s+`)
	wordNumberedHeading = regexp.MustCompile(`^\s*(\d+(?:\.\d+){0,5})\.?\s+\S`)
)

type Parser interface {
	Parse(context.Context, []byte, string, string) ([]ParsedBlock, error)
}

type StructuredParser struct{}

func NewStructuredParser() StructuredParser { return StructuredParser{} }

func (StructuredParser) Parse(ctx context.Context, data []byte, mediaType, filename string) ([]ParsedBlock, error) {
	switch mediaType {
	case MediaTypeMarkdown:
		return parseMarkdown(ctx, data)
	case MediaTypeDOCX:
		return parseDOCX(ctx, data)
	default:
		return nil, fmt.Errorf("%w: %s (%s)", ErrUnsupported, filename, mediaType)
	}
}

func parseMarkdown(ctx context.Context, data []byte) ([]ParsedBlock, error) {
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, fmt.Errorf("%w: Markdown must be UTF-8 text without NUL bytes", ErrInvalidInput)
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	blocks := make([]ParsedBlock, 0)
	headingPath := make([]string, 0, 6)
	for index := 0; index < len(lines); {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line := lines[index]
		if strings.TrimSpace(line) == "" {
			index++
			continue
		}
		if match := markdownHeading.FindStringSubmatch(line); len(match) == 3 {
			level := len(match[1])
			title := strings.TrimSpace(match[2])
			headingPath = updateHeadingPath(headingPath, level, title)
			blocks = append(blocks, ParsedBlock{BlockType: BlockHeading, HeadingLevel: level,
				Content: title, SourceLocator: lineLocator(index+1, index+1),
				Metadata: map[string]any{"heading_path": append([]string(nil), headingPath...)}})
			index++
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			start := index
			index++
			for index < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[index]), "```") {
				index++
			}
			if index < len(lines) {
				index++
			}
			content := strings.TrimSpace(strings.Join(lines[start:index], "\n"))
			blocks = append(blocks, textBlock(BlockCode, content, start+1, index, headingPath))
			continue
		}
		blockType := BlockParagraph
		if markdownList.MatchString(line) {
			blockType = BlockList
		} else if looksLikeMarkdownTableLine(line) {
			blockType = BlockTable
		}
		start := index
		index++
		for index < len(lines) && strings.TrimSpace(lines[index]) != "" {
			if markdownHeading.MatchString(lines[index]) || strings.HasPrefix(strings.TrimSpace(lines[index]), "```") {
				break
			}
			currentType := BlockParagraph
			if markdownList.MatchString(lines[index]) {
				currentType = BlockList
			} else if looksLikeMarkdownTableLine(lines[index]) {
				currentType = BlockTable
			}
			if currentType != blockType {
				break
			}
			index++
		}
		content := strings.TrimSpace(strings.Join(lines[start:index], "\n"))
		if content != "" {
			blocks = append(blocks, textBlock(blockType, content, start+1, index, headingPath))
		}
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("%w: Markdown contains no readable content", ErrInvalidInput)
	}
	return blocks, nil
}

func textBlock(blockType, content string, startLine, endLine int, headingPath []string) ParsedBlock {
	return ParsedBlock{BlockType: blockType, Content: content,
		SourceLocator: lineLocator(startLine, endLine),
		Metadata:      map[string]any{"heading_path": append([]string(nil), headingPath...)}}
}

func lineLocator(start, end int) string {
	if start == end {
		return "line:" + strconv.Itoa(start)
	}
	return fmt.Sprintf("lines:%d-%d", start, end)
}

func looksLikeMarkdownTableLine(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.Count(trimmed, "|") >= 2 && (strings.HasPrefix(trimmed, "|") || strings.HasSuffix(trimmed, "|"))
}

func updateHeadingPath(current []string, level int, title string) []string {
	if level < 1 {
		return current
	}
	if len(current) >= level {
		current = current[:level-1]
	}
	for len(current) < level-1 {
		current = append(current, "")
	}
	return append(current, title)
}

func parseDOCX(ctx context.Context, data []byte) ([]ParsedBlock, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid DOCX archive", ErrInvalidInput)
	}
	if len(archive.File) == 0 || len(archive.File) > maxDOCXEntries {
		return nil, fmt.Errorf("%w: DOCX archive entry count is invalid", ErrUnsafeDocument)
	}
	var documentXML *zip.File
	var total uint64
	for _, entry := range archive.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cleaned := path.Clean(entry.Name)
		if cleaned != entry.Name || strings.HasPrefix(cleaned, "/") || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			return nil, fmt.Errorf("%w: invalid DOCX entry path", ErrUnsafeDocument)
		}
		lower := strings.ToLower(cleaned)
		if strings.HasSuffix(lower, "vbaproject.bin") || strings.HasSuffix(lower, ".exe") || strings.HasSuffix(lower, ".dll") {
			return nil, fmt.Errorf("%w: executable content is not allowed", ErrUnsafeDocument)
		}
		total += entry.UncompressedSize64
		if total > maxDOCXUncompressedBytes || entry.UncompressedSize64 > maxDOCXXMLBytes {
			return nil, fmt.Errorf("%w: DOCX expanded content exceeds the safety limit", ErrUnsafeDocument)
		}
		if cleaned == "word/document.xml" {
			if documentXML != nil {
				return nil, fmt.Errorf("%w: duplicate DOCX document.xml entry", ErrUnsafeDocument)
			}
			documentXML = entry
		}
	}
	if documentXML == nil {
		return nil, fmt.Errorf("%w: DOCX document.xml is missing", ErrInvalidInput)
	}
	reader, err := documentXML.Open()
	if err != nil {
		return nil, fmt.Errorf("open DOCX document XML: %w", err)
	}
	defer reader.Close()
	xmlBytes, err := io.ReadAll(io.LimitReader(reader, maxDOCXXMLBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read DOCX document XML: %w", err)
	}
	if len(xmlBytes) > maxDOCXXMLBytes {
		return nil, fmt.Errorf("%w: DOCX XML exceeds the safety limit", ErrUnsafeDocument)
	}
	return parseWordDocument(ctx, xmlBytes)
}

func parseWordDocument(ctx context.Context, data []byte) ([]ParsedBlock, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	blocks := make([]ParsedBlock, 0)
	headingPath := make([]string, 0, 6)
	paragraphNumber, tableNumber := 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode DOCX XML: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "p":
			paragraphNumber++
			text, style, list, bold, err := parseWordParagraph(ctx, decoder, start)
			if err != nil {
				return nil, err
			}
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			level := wordHeadingLevel(style)
			if level == 0 && !list {
				level = wordDirectHeadingLevel(text, bold)
			}
			blockType := BlockParagraph
			if level > 0 {
				blockType = BlockHeading
				headingPath = updateHeadingPath(headingPath, level, text)
			} else if list {
				blockType = BlockList
			}
			blocks = append(blocks, ParsedBlock{BlockType: blockType, HeadingLevel: level,
				Content: text, SourceLocator: fmt.Sprintf("word/body/p[%d]", paragraphNumber),
				Metadata: map[string]any{"style": style, "heading_path": append([]string(nil), headingPath...)}})
		case "tbl":
			tableNumber++
			rows, err := parseWordTable(ctx, decoder, start)
			if err != nil {
				return nil, err
			}
			if len(rows) == 0 {
				continue
			}
			formattedRows := make([]string, 0, len(rows))
			for _, row := range rows {
				formattedRows = append(formattedRows, "| "+strings.Join(row, " | ")+" |")
			}
			blocks = append(blocks, ParsedBlock{BlockType: BlockTable,
				Content:       strings.Join(formattedRows, "\n"),
				SourceLocator: fmt.Sprintf("word/body/table[%d]", tableNumber),
				Metadata:      map[string]any{"row_count": len(rows), "heading_path": append([]string(nil), headingPath...)}})
		}
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("%w: DOCX contains no readable paragraphs or tables", ErrInvalidInput)
	}
	return blocks, nil
}

func parseWordParagraph(ctx context.Context, decoder *xml.Decoder, start xml.StartElement) (string, string, bool, bool, error) {
	var text strings.Builder
	style := ""
	list := false
	bold := false
	depth := 1
	for depth > 0 {
		if err := ctx.Err(); err != nil {
			return "", "", false, false, err
		}
		token, err := decoder.Token()
		if err != nil {
			return "", "", false, false, fmt.Errorf("decode DOCX paragraph: %w", err)
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if typed.Name.Local == "t" {
				var value string
				if err := decoder.DecodeElement(&value, &typed); err != nil {
					return "", "", false, false, fmt.Errorf("decode DOCX text: %w", err)
				}
				text.WriteString(value)
				continue
			}
			if typed.Name.Local == "pStyle" {
				style = wordAttribute(typed.Attr, "val")
			}
			if typed.Name.Local == "numPr" {
				list = true
			}
			if typed.Name.Local == "b" && wordOnOff(typed.Attr) {
				bold = true
			}
			if typed.Name.Local == "tab" {
				text.WriteByte('\t')
			} else if typed.Name.Local == "br" || typed.Name.Local == "cr" {
				text.WriteByte('\n')
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return text.String(), style, list, bold, nil
}

func parseWordTable(ctx context.Context, decoder *xml.Decoder, start xml.StartElement) ([][]string, error) {
	rows := make([][]string, 0)
	depth := 1
	for depth > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("decode DOCX table: %w", err)
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if typed.Name.Local == "tr" {
				row, err := parseWordRow(ctx, decoder, typed)
				if err != nil {
					return nil, err
				}
				if len(row) > 0 {
					rows = append(rows, row)
				}
				continue
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return rows, nil
}

func parseWordRow(ctx context.Context, decoder *xml.Decoder, start xml.StartElement) ([]string, error) {
	row := make([]string, 0)
	depth := 1
	for depth > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("decode DOCX table row: %w", err)
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if typed.Name.Local == "tc" {
				cell, err := parseWordCell(ctx, decoder, typed)
				if err != nil {
					return nil, err
				}
				row = append(row, strings.TrimSpace(cell))
				continue
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return row, nil
}

func parseWordCell(ctx context.Context, decoder *xml.Decoder, start xml.StartElement) (string, error) {
	paragraphs := make([]string, 0)
	depth := 1
	for depth > 0 {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		token, err := decoder.Token()
		if err != nil {
			return "", fmt.Errorf("decode DOCX table cell: %w", err)
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if typed.Name.Local == "p" {
				paragraph, _, _, _, err := parseWordParagraph(ctx, decoder, typed)
				if err != nil {
					return "", err
				}
				if paragraph = strings.TrimSpace(paragraph); paragraph != "" {
					paragraphs = append(paragraphs, paragraph)
				}
				continue
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return strings.Join(paragraphs, "\n"), nil
}

func wordHeadingLevel(style string) int {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(style), " ", ""))
	for _, prefix := range []string{"heading", "tieude"} {
		if strings.HasPrefix(normalized, prefix) {
			value := strings.TrimPrefix(normalized, prefix)
			if level, err := strconv.Atoi(value); err == nil && level >= 1 && level <= 6 {
				return level
			}
		}
	}
	return 0
}

func wordDirectHeadingLevel(text string, bold bool) int {
	if !bold {
		return 0
	}
	match := wordNumberedHeading.FindStringSubmatch(text)
	if len(match) != 2 {
		return 0
	}
	level := strings.Count(match[1], ".") + 1
	if level > 6 {
		return 6
	}
	return level
}

func wordOnOff(attributes []xml.Attr) bool {
	value := strings.ToLower(strings.TrimSpace(wordAttribute(attributes, "val")))
	return value != "0" && value != "false" && value != "off" && value != "no"
}

func wordAttribute(attributes []xml.Attr, localName string) string {
	for _, attribute := range attributes {
		if attribute.Name.Local == localName {
			return attribute.Value
		}
	}
	return ""
}
