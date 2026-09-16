package document

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var semanticIdentifierPattern = regexp.MustCompile(`(?i)\b(?:UC|FR|BR|AC|PR|NFR|YC|REQ)[-_.]?[A-Z0-9]+(?:[-_.][A-Z0-9]+)*\b`)

type SemanticChunker struct {
	maxRunes int
}

func NewSemanticChunker() *SemanticChunker { return &SemanticChunker{maxRunes: 1800} }

func (c *SemanticChunker) Chunk(source IndexSource) []SemanticChunk {
	chunks := make([]SemanticChunk, 0, len(source.Blocks))
	headingPath := make([]string, 0, 6)
	parentIdentifier := ""
	parentChunkKey := ""
	flowType := FlowNone
	for _, block := range source.Blocks {
		headingIdentifier := ""
		if block.BlockType == BlockHeading {
			headingPath = updateSemanticHeadingPath(headingPath, block.HeadingLevel, block.Content)
			headingIdentifier = extractSemanticIdentifier(block.Content)
			if detected := detectFlowType(block.Content); detected != FlowNone {
				flowType = detected
			} else if block.HeadingLevel <= 2 && headingIdentifier != "" {
				flowType = FlowNone
			}
		}
		blockChunks := make([]SemanticChunk, 0, 1)
		if block.BlockType == BlockTable {
			rows := tableRows(block.Content)
			if len(rows) > 1 {
				header := rows[0]
				for rowIndex, row := range rows[1:] {
					raw := header + "\n" + row
					blockChunks = append(blockChunks, c.makeChunk(source, block, raw, "TABLE_ROW", flowType,
						parentIdentifier, parentChunkKey, headingPath, fmt.Sprintf("row-%d", rowIndex+1)))
				}
				chunks = append(chunks, blockChunks...)
				continue
			}
		}
		parts := splitSemanticContent(block.Content, c.maxRunes)
		for index, part := range parts {
			typeName := semanticChunkType(block, part)
			suffix := ""
			if len(parts) > 1 {
				suffix = fmt.Sprintf("part-%d", index+1)
			}
			blockChunks = append(blockChunks, c.makeChunk(source, block, part, typeName, flowType,
				parentIdentifier, parentChunkKey, headingPath, suffix))
		}
		chunks = append(chunks, blockChunks...)
		if headingIdentifier != "" && len(blockChunks) > 0 {
			parentIdentifier = headingIdentifier
			parentChunkKey = blockChunks[0].ChunkKey
		}
	}
	return chunks
}

func (c *SemanticChunker) makeChunk(source IndexSource, block Block, raw, chunkType, flowType,
	parentIdentifier, parentChunkKey string, headingPath []string, suffix string,
) SemanticChunk {
	normalized := normalizeSemanticText(raw)
	identifier := extractSemanticIdentifier(raw)
	if identifier == "" && flowType != FlowNone {
		identifier = parentIdentifier
	}
	title := ""
	if len(headingPath) > 0 {
		title = headingPath[len(headingPath)-1]
	}
	keySeed := fmt.Sprintf("%d:%d:%s:%s", source.Version.ID, block.Ordinal, chunkType, suffix)
	keyHash := sha256.Sum256([]byte(keySeed))
	metadata := map[string]any{
		"heading_path":         append([]string(nil), headingPath...),
		"document_name":        source.Document.Name,
		"document_type":        source.Document.DocumentType,
		"version_number":       source.Version.VersionNumber,
		"source_block_ordinal": block.Ordinal,
		"content_is_untrusted": true,
		"prompt_injection":     ContainsPromptInjection(raw),
		"chunker_version":      SemanticChunkerVersion,
		"actor":                extractSemanticLabel(raw, "actor", "tác nhân", "vai trò", "vai liên quan"),
		"precondition":         extractSemanticLabel(raw, "precondition", "tiền điều kiện"),
		"postcondition":        extractSemanticLabel(raw, "postcondition", "post-condition", "hậu điều kiện"),
	}
	encoded, _ := json.Marshal(metadata)
	contentHash := sha256.Sum256([]byte(normalized))
	return SemanticChunk{
		DocumentSetID: source.Version.DocumentSetID, DocumentVersionID: source.Version.ID,
		DocumentBlockID: block.ID, SourceBlockIDs: []int64{block.ID},
		ChunkKey: hex.EncodeToString(keyHash[:16]), ParentChunkKey: parentChunkKey,
		ChunkType: chunkType, FlowType: flowType, Identifier: strings.ToUpper(identifier),
		Title: title, Content: normalized, RawContent: raw,
		ContentHash: hex.EncodeToString(contentHash[:]), SourceLocator: block.SourceLocator,
		Metadata: encoded,
	}
}

func extractSemanticLabel(input string, labels ...string) string {
	for _, line := range strings.Split(input, "\n") {
		for _, separator := range []string{":", " - "} {
			index := strings.Index(line, separator)
			if index < 0 {
				continue
			}
			key := strings.Trim(strings.TrimSpace(line[:index]), "|*- ")
			for _, label := range labels {
				if foldVietnamese(key) == foldVietnamese(label) {
					return strings.Trim(strings.TrimSpace(line[index+len(separator):]), "| ")
				}
			}
		}
	}
	return ""
}

func semanticChunkType(block Block, content string) string {
	identifier := strings.ToUpper(extractSemanticIdentifier(content))
	switch {
	case strings.HasPrefix(identifier, "UC"):
		return "USE_CASE"
	case strings.HasPrefix(identifier, "BR"):
		return "BUSINESS_RULE"
	case strings.HasPrefix(identifier, "AC"):
		return "ACCEPTANCE_CRITERION"
	case strings.HasPrefix(identifier, "NFR"):
		return "NON_FUNCTIONAL"
	case strings.HasPrefix(identifier, "FR"), strings.HasPrefix(identifier, "PR"):
		return "REQUIREMENT"
	case block.BlockType == BlockHeading:
		return "SECTION"
	case block.BlockType == BlockList:
		return "FLOW_STEP"
	case block.BlockType == BlockCode:
		return "CODE_REFERENCE"
	default:
		return "PROSE"
	}
}

func extractSemanticIdentifier(input string) string {
	return semanticIdentifierPattern.FindString(strings.ToUpper(input))
}

func detectFlowType(input string) string {
	value := foldVietnamese(input)
	switch {
	case strings.Contains(value, "ngoai le"), strings.Contains(value, "exception"),
		strings.Contains(value, "error flow"):
		return FlowException
	case strings.Contains(value, "thay the"), strings.Contains(value, "alternate"),
		strings.Contains(value, "alternative"):
		return FlowAlternate
	case strings.Contains(value, "luong chinh"), strings.Contains(value, "main flow"),
		strings.Contains(value, "basic flow"):
		return FlowMain
	default:
		return FlowNone
	}
}

func normalizeSemanticText(input string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(input, "\u00a0", " ")), " ")
}

func splitSemanticContent(input string, maxRunes int) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	runes := []rune(input)
	if len(runes) <= maxRunes {
		return []string{input}
	}
	parts := make([]string, 0, len(runes)/maxRunes+1)
	for len(runes) > 0 {
		end := min(maxRunes, len(runes))
		if end < len(runes) {
			for candidate := end; candidate > maxRunes/2; candidate-- {
				if unicode.IsSpace(runes[candidate-1]) || strings.ContainsRune(".;!?", runes[candidate-1]) {
					end = candidate
					break
				}
			}
		}
		parts = append(parts, strings.TrimSpace(string(runes[:end])))
		runes = runes[end:]
	}
	return parts
}

func tableRows(input string) []string {
	lines := strings.Split(input, "\n")
	rows := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || isTableSeparator(line) {
			continue
		}
		rows = append(rows, line)
	}
	return rows
}

func isTableSeparator(input string) bool {
	trimmed := strings.Trim(input, "| :-\t")
	return trimmed == ""
}

func updateSemanticHeadingPath(current []string, level int, value string) []string {
	if level < 1 {
		level = 1
	}
	if len(current) >= level {
		current = current[:level-1]
	}
	for len(current) < level-1 {
		current = append(current, "")
	}
	return append(current, value)
}

// ContainsPromptInjection flags likely instructions embedded in source content.
// The source remains searchable, but downstream prompts label it as untrusted evidence.
func ContainsPromptInjection(input string) bool {
	value := strings.ToLower(input)
	patterns := []string{
		"ignore previous instructions", "ignore all instructions", "system prompt",
		"developer message", "reveal your prompt", "exfiltrate", "bỏ qua hướng dẫn",
		"bỏ qua chỉ dẫn", "tiết lộ prompt", "làm theo lệnh sau",
	}
	for _, pattern := range patterns {
		if strings.Contains(value, pattern) {
			return true
		}
	}
	return false
}

func foldVietnamese(input string) string {
	replacer := strings.NewReplacer(
		"à", "a", "á", "a", "ạ", "a", "ả", "a", "ã", "a", "â", "a", "ầ", "a", "ấ", "a", "ậ", "a", "ẩ", "a", "ẫ", "a", "ă", "a", "ằ", "a", "ắ", "a", "ặ", "a", "ẳ", "a", "ẵ", "a",
		"è", "e", "é", "e", "ẹ", "e", "ẻ", "e", "ẽ", "e", "ê", "e", "ề", "e", "ế", "e", "ệ", "e", "ể", "e", "ễ", "e",
		"ì", "i", "í", "i", "ị", "i", "ỉ", "i", "ĩ", "i",
		"ò", "o", "ó", "o", "ọ", "o", "ỏ", "o", "õ", "o", "ô", "o", "ồ", "o", "ố", "o", "ộ", "o", "ổ", "o", "ỗ", "o", "ơ", "o", "ờ", "o", "ớ", "o", "ợ", "o", "ở", "o", "ỡ", "o",
		"ù", "u", "ú", "u", "ụ", "u", "ủ", "u", "ũ", "u", "ư", "u", "ừ", "u", "ứ", "u", "ự", "u", "ử", "u", "ữ", "u",
		"ỳ", "y", "ý", "y", "ỵ", "y", "ỷ", "y", "ỹ", "y", "đ", "d",
	)
	return replacer.Replace(strings.ToLower(input))
}
