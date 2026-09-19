package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (r *IndexRepository) CreateSourceSnapshot(ctx context.Context, setID int64,
	options IndexOptions,
) (SourceSnapshot, []IndexSource, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return SourceSnapshot{}, nil, fmt.Errorf("begin source snapshot: %w", err)
	}
	defer tx.Rollback(ctx)

	var sourceRevision int64
	if err := tx.QueryRow(ctx, `SELECT source_revision FROM document_sets WHERE id=$1 FOR UPDATE`, setID).
		Scan(&sourceRevision); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SourceSnapshot{}, nil, ErrNotFound
		}
		return SourceSnapshot{}, nil, fmt.Errorf("lock document set source revision: %w", err)
	}

	excluded := make(map[int64]struct{}, len(options.ExcludedVersionIDs))
	for _, versionID := range options.ExcludedVersionIDs {
		if versionID <= 0 {
			return SourceSnapshot{}, nil, ErrInvalidInput
		}
		excluded[versionID] = struct{}{}
	}

	const versionsQuery = `WITH latest AS (
		SELECT DISTINCT ON (d.id) d.id AS document_id_value,d.document_set_id,d.name,d.document_type,
			d.created_at,d.updated_at,
			v.id AS version_id_value,v.document_id AS version_document_id,
			v.document_set_id AS version_document_set_id,v.version_number,v.original_filename,
			v.media_type,v.size_bytes,v.sha256,v.storage_key,v.approval_status,
			v.parse_status,v.parse_error,v.block_count,v.attempt_count,v.next_attempt_at,
			v.lease_expires_at,v.uploaded_at,v.started_at,v.parsed_at
		FROM documents d JOIN document_versions v ON v.document_id=d.id
		WHERE d.document_set_id=$1 ORDER BY d.id,v.version_number DESC
	) SELECT * FROM latest ORDER BY document_id_value`
	rows, err := tx.Query(ctx, versionsQuery, setID)
	if err != nil {
		return SourceSnapshot{}, nil, fmt.Errorf("list source snapshot versions: %w", err)
	}
	sources := make([]IndexSource, 0)
	candidates := make([]IndexSource, 0)
	items := make([]IndexVersionRef, 0)
	issues := make([]IndexVersionRef, 0)
	seenExcluded := make(map[int64]struct{}, len(excluded))
	for rows.Next() {
		var source IndexSource
		destinations := []any{&source.Document.ID, &source.Document.DocumentSetID,
			&source.Document.Name, &source.Document.DocumentType, &source.Document.CreatedAt,
			&source.Document.UpdatedAt}
		destinations = append(destinations, versionDestinations(&source.Version)...)
		if err := rows.Scan(destinations...); err != nil {
			rows.Close()
			return SourceSnapshot{}, nil, fmt.Errorf("scan source snapshot version: %w", err)
		}
		item := IndexVersionRef{DocumentID: source.Document.ID, DocumentName: source.Document.Name,
			DocumentVersionID: source.Version.ID, VersionNumber: source.Version.VersionNumber,
			SHA256: source.Version.SHA256, ParseStatus: source.Version.ParseStatus,
			ApprovalStatus: source.Version.ApprovalStatus, Included: true}
		if _, ok := excluded[source.Version.ID]; ok {
			item.Included = false
			item.ExclusionReason = "USER_EXCLUDED"
			seenExcluded[source.Version.ID] = struct{}{}
			items = append(items, item)
			continue
		}
		if source.Version.ParseStatus != ParseParsed || source.Version.BlockCount == 0 {
			issues = append(issues, item)
			items = append(items, item)
			continue
		}
		candidates = append(candidates, source)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return SourceSnapshot{}, nil, fmt.Errorf("iterate source snapshot versions: %w", err)
	}
	rows.Close()
	for _, source := range candidates {
		blockRows, blockErr := tx.Query(ctx, `SELECT id,document_version_id,ordinal,block_type,
			heading_level,content,source_locator,metadata,created_at
			FROM document_blocks WHERE document_version_id=$1 ORDER BY ordinal`, source.Version.ID)
		if blockErr != nil {
			return SourceSnapshot{}, nil, fmt.Errorf("load source snapshot blocks: %w", blockErr)
		}
		for blockRows.Next() {
			var block Block
			if err := blockRows.Scan(&block.ID, &block.DocumentVersionID, &block.Ordinal,
				&block.BlockType, &block.HeadingLevel, &block.Content, &block.SourceLocator,
				&block.Metadata, &block.CreatedAt); err != nil {
				blockRows.Close()
				return SourceSnapshot{}, nil, fmt.Errorf("scan source snapshot block: %w", err)
			}
			source.Blocks = append(source.Blocks, block)
		}
		if err := blockRows.Err(); err != nil {
			blockRows.Close()
			return SourceSnapshot{}, nil, err
		}
		blockRows.Close()
		if len(source.Blocks) == 0 {
			for _, item := range items {
				if item.DocumentVersionID == source.Version.ID {
					issues = append(issues, item)
					break
				}
			}
			continue
		}
		sources = append(sources, source)
	}
	if len(items) == 0 {
		return SourceSnapshot{}, nil, ErrNoParsedDocuments
	}
	if len(seenExcluded) != len(excluded) {
		return SourceSnapshot{}, nil, fmt.Errorf("%w: excluded version must be the latest version in this document set", ErrInvalidInput)
	}
	if len(issues) > 0 {
		return SourceSnapshot{}, nil, &SourceSnapshotBlockedError{Issues: issues}
	}
	if len(sources) == 0 {
		return SourceSnapshot{}, nil, fmt.Errorf("%w: all latest document versions were excluded", ErrNoParsedDocuments)
	}

	fingerprint := sourceSnapshotFingerprint(items)
	var snapshot SourceSnapshot
	err = tx.QueryRow(ctx, `SELECT id,document_set_id,source_revision,fingerprint,
		included_count,excluded_count,created_at FROM document_source_snapshots
		WHERE document_set_id=$1 AND source_revision=$2 AND fingerprint=$3`,
		setID, sourceRevision, fingerprint).Scan(&snapshot.ID, &snapshot.DocumentSetID,
		&snapshot.SourceRevision, &snapshot.Fingerprint, &snapshot.IncludedCount,
		&snapshot.ExcludedCount, &snapshot.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		excludedCount := len(items) - len(sources)
		err = tx.QueryRow(ctx, `INSERT INTO document_source_snapshots
			(document_set_id,source_revision,fingerprint,included_count,excluded_count)
			VALUES($1,$2,$3,$4,$5) RETURNING id,document_set_id,source_revision,fingerprint,
			included_count,excluded_count,created_at`, setID, sourceRevision, fingerprint,
			len(sources), excludedCount).Scan(&snapshot.ID, &snapshot.DocumentSetID,
			&snapshot.SourceRevision, &snapshot.Fingerprint, &snapshot.IncludedCount,
			&snapshot.ExcludedCount, &snapshot.CreatedAt)
		if err == nil {
			for _, item := range items {
				if _, err := tx.Exec(ctx, `INSERT INTO document_source_snapshot_items
					(source_snapshot_id,document_set_id,document_id,document_name,document_version_id,
					 version_number,sha256,parse_status,approval_status,included,exclusion_reason)
					VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, snapshot.ID, setID,
					item.DocumentID, item.DocumentName, item.DocumentVersionID, item.VersionNumber,
					item.SHA256, item.ParseStatus, item.ApprovalStatus, item.Included,
					item.ExclusionReason); err != nil {
					return SourceSnapshot{}, nil, fmt.Errorf("save source snapshot item: %w", err)
				}
			}
		}
	}
	if err != nil {
		return SourceSnapshot{}, nil, fmt.Errorf("save source snapshot: %w", err)
	}
	snapshot.Items = items
	if err := tx.Commit(ctx); err != nil {
		return SourceSnapshot{}, nil, fmt.Errorf("commit source snapshot: %w", err)
	}
	return snapshot, sources, nil
}

func sourceSnapshotFingerprint(items []IndexVersionRef) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%d:%d:%s:%t:%s", item.DocumentID,
			item.DocumentVersionID, item.SHA256, item.Included, item.ExclusionReason))
	}
	sort.Strings(parts)
	digest := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(digest[:])
}

func (r *IndexRepository) GetGeneration(ctx context.Context, setID, generation int64) (IndexGeneration, error) {
	var result IndexGeneration
	err := r.pool.QueryRow(ctx, `SELECT document_set_id,generation,source_snapshot_id,
		source_revision,input_fingerprint,embedding_model,status,version_count,
		excluded_version_count,chunk_count,content_warning_count,error_message,
		requested_at,started_at,finished_at,updated_at
		FROM document_index_generations WHERE document_set_id=$1 AND generation=$2`,
		setID, generation).Scan(indexGenerationDestinations(&result)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return IndexGeneration{}, ErrNotFound
	}
	if err != nil {
		return IndexGeneration{}, fmt.Errorf("get document index generation: %w", err)
	}
	result.WarningCount, err = r.dynamicWarningCount(ctx, result.DocumentSetID,
		result.Generation, result.ContentWarningCount)
	return result, err
}

func (r *IndexRepository) ListGenerations(ctx context.Context, setID int64) ([]IndexGeneration, error) {
	rows, err := r.pool.Query(ctx, `SELECT document_set_id,generation,source_snapshot_id,
		source_revision,input_fingerprint,embedding_model,status,version_count,
		excluded_version_count,chunk_count,content_warning_count,error_message,
		requested_at,started_at,finished_at,updated_at
		FROM document_index_generations WHERE document_set_id=$1
		ORDER BY generation DESC LIMIT 25`, setID)
	if err != nil {
		return nil, fmt.Errorf("list document index generations: %w", err)
	}
	defer rows.Close()
	results := make([]IndexGeneration, 0)
	for rows.Next() {
		var item IndexGeneration
		if err := rows.Scan(indexGenerationDestinations(&item)...); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for index := range results {
		results[index].WarningCount, err = r.dynamicWarningCount(ctx, setID,
			results[index].Generation, results[index].ContentWarningCount)
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

func indexGenerationDestinations(item *IndexGeneration) []any {
	return []any{&item.DocumentSetID, &item.Generation, &item.SourceSnapshotID,
		&item.SourceRevision, &item.InputFingerprint, &item.EmbeddingModel, &item.Status,
		&item.VersionCount, &item.ExcludedVersionCount, &item.ChunkCount,
		&item.ContentWarningCount, &item.ErrorMessage, &item.RequestedAt, &item.StartedAt,
		&item.FinishedAt, &item.UpdatedAt}
}

func (r *IndexRepository) dynamicWarningCount(ctx context.Context, setID, generation int64,
	contentWarnings int,
) (int, error) {
	var authorityWarnings int
	err := r.pool.QueryRow(ctx, `SELECT count(DISTINCT c.document_version_id)
		FROM document_index_generation_chunks m
		JOIN document_chunks c ON c.id=m.document_chunk_id
		JOIN document_versions v ON v.id=c.document_version_id
		WHERE m.document_set_id=$1 AND m.generation=$2 AND v.approval_status<>'APPROVED'`,
		setID, generation).Scan(&authorityWarnings)
	if err != nil {
		return 0, fmt.Errorf("count document index warnings: %w", err)
	}
	return contentWarnings + authorityWarnings, nil
}
