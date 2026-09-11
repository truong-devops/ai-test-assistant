package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

type ParseResultSaver interface {
	SaveParsed(context.Context, Version, []ParsedBlock) error
}

type Processor struct {
	files   FileStore
	parser  Parser
	results ParseResultSaver
}

func NewProcessor(files FileStore, parser Parser, results ParseResultSaver) *Processor {
	return &Processor{files: files, parser: parser, results: results}
}

func (p *Processor) Process(ctx context.Context, claimed Version) error {
	if claimed.ID <= 0 || claimed.DocumentID <= 0 || claimed.SizeBytes <= 0 || claimed.StorageKey == "" {
		return fmt.Errorf("claimed document version is incomplete")
	}
	file, err := p.files.Open(ctx, claimed.StorageKey)
	if err != nil {
		return fmt.Errorf("open document version %d: %w", claimed.ID, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, claimed.SizeBytes+1))
	if err != nil {
		return fmt.Errorf("read document version %d: %w", claimed.ID, err)
	}
	if int64(len(data)) != claimed.SizeBytes {
		return fmt.Errorf("document version %d size does not match stored metadata", claimed.ID)
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != claimed.SHA256 {
		return fmt.Errorf("document version %d checksum does not match stored metadata", claimed.ID)
	}
	blocks, err := p.parser.Parse(ctx, data, claimed.MediaType, claimed.OriginalFilename)
	if err != nil {
		return fmt.Errorf("parse document version %d: %w", claimed.ID, err)
	}
	if len(blocks) == 0 {
		return fmt.Errorf("parse document version %d: no content blocks", claimed.ID)
	}
	return p.results.SaveParsed(ctx, claimed, blocks)
}
