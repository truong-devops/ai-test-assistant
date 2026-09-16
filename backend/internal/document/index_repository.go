package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type IndexRepository struct {
	pool *pgxpool.Pool
}

func NewIndexRepository(pool *pgxpool.Pool) *IndexRepository { return &IndexRepository{pool: pool} }

func (r *IndexRepository) LoadSources(ctx context.Context, setID int64) ([]IndexSource, int, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM document_sets WHERE id=$1)`, setID).Scan(&exists); err != nil {
		return nil, 0, fmt.Errorf("check document set: %w", err)
	}
	if !exists {
		return nil, 0, ErrNotFound
	}
	const versionsQuery = `WITH latest AS (
		SELECT DISTINCT ON (d.id) d.id AS document_id_value, d.document_set_id, d.name, d.document_type,
			d.created_at, d.updated_at,
			v.id AS version_id_value, v.document_id AS version_document_id, v.document_set_id AS version_document_set_id, v.version_number, v.original_filename,
			v.media_type, v.size_bytes, v.sha256, v.storage_key, v.approval_status,
			v.parse_status, v.parse_error, v.block_count, v.attempt_count,
			v.next_attempt_at, v.lease_expires_at, v.uploaded_at, v.started_at, v.parsed_at
		FROM documents d JOIN document_versions v ON v.document_id=d.id
		WHERE d.document_set_id=$1
		ORDER BY d.id, v.version_number DESC
	)
	SELECT * FROM latest ORDER BY document_id_value`
	rows, err := r.pool.Query(ctx, versionsQuery, setID)
	if err != nil {
		return nil, 0, fmt.Errorf("list latest document versions for indexing: %w", err)
	}
	defer rows.Close()
	sources := make([]IndexSource, 0)
	skipped := 0
	for rows.Next() {
		var source IndexSource
		destinations := []any{&source.Document.ID, &source.Document.DocumentSetID,
			&source.Document.Name, &source.Document.DocumentType, &source.Document.CreatedAt,
			&source.Document.UpdatedAt}
		destinations = append(destinations, versionDestinations(&source.Version)...)
		if err := rows.Scan(destinations...); err != nil {
			return nil, 0, fmt.Errorf("scan index source: %w", err)
		}
		if source.Version.ParseStatus != ParseParsed {
			skipped++
			continue
		}
		source.Blocks, err = r.listBlocks(ctx, source.Version.ID)
		if err != nil {
			return nil, 0, err
		}
		if len(source.Blocks) == 0 {
			skipped++
			continue
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate index sources: %w", err)
	}
	return sources, skipped, nil
}

func (r *IndexRepository) listBlocks(ctx context.Context, versionID int64) ([]Block, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, document_version_id, ordinal, block_type,
		heading_level, content, source_locator, metadata, created_at
		FROM document_blocks WHERE document_version_id=$1 ORDER BY ordinal`, versionID)
	if err != nil {
		return nil, fmt.Errorf("load blocks for indexing: %w", err)
	}
	defer rows.Close()
	blocks := make([]Block, 0)
	for rows.Next() {
		var block Block
		if err := rows.Scan(&block.ID, &block.DocumentVersionID, &block.Ordinal,
			&block.BlockType, &block.HeadingLevel, &block.Content, &block.SourceLocator,
			&block.Metadata, &block.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan index block: %w", err)
		}
		blocks = append(blocks, block)
	}
	return blocks, rows.Err()
}

func (r *IndexRepository) GetStatus(ctx context.Context, setID int64) (IndexStatus, error) {
	const query = `SELECT document_set_id,status,generation,input_fingerprint,version_count,
		skipped_version_count,chunk_count,warning_count,embedding_model,error_message,
		requested_at,started_at,finished_at,updated_at
		FROM document_index_status WHERE document_set_id=$1`
	var result IndexStatus
	err := r.pool.QueryRow(ctx, query, setID).Scan(indexStatusDestinations(&result)...)
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		if checkErr := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM document_sets WHERE id=$1)`, setID).Scan(&exists); checkErr != nil {
			return IndexStatus{}, fmt.Errorf("check document set index owner: %w", checkErr)
		}
		if !exists {
			return IndexStatus{}, ErrNotFound
		}
		return IndexStatus{DocumentSetID: setID, Status: IndexNotIndexed}, nil
	}
	if err != nil {
		return IndexStatus{}, fmt.Errorf("get document index status: %w", err)
	}
	return result, nil
}

func (r *IndexRepository) Begin(ctx context.Context, setID int64, fingerprint, model string) (IndexStatus, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return IndexStatus{}, false, fmt.Errorf("begin document index: %w", err)
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM document_sets WHERE id=$1)`, setID).Scan(&exists); err != nil {
		return IndexStatus{}, false, fmt.Errorf("check document set: %w", err)
	}
	if !exists {
		return IndexStatus{}, false, ErrNotFound
	}
	var current IndexStatus
	err = tx.QueryRow(ctx, `SELECT document_set_id,status,generation,input_fingerprint,version_count,
		skipped_version_count,chunk_count,warning_count,embedding_model,error_message,
		requested_at,started_at,finished_at,updated_at
		FROM document_index_status WHERE document_set_id=$1 FOR UPDATE`, setID).
		Scan(indexStatusDestinations(&current)...)
	if err == nil && current.Status == IndexReady && current.InputFingerprint == fingerprint && current.EmbeddingModel == model {
		if err := tx.Commit(ctx); err != nil {
			return IndexStatus{}, false, fmt.Errorf("commit unchanged index check: %w", err)
		}
		return current, true, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return IndexStatus{}, false, fmt.Errorf("lock document index status: %w", err)
	}
	const upsert = `INSERT INTO document_index_status
		(document_set_id,status,generation,input_fingerprint,embedding_model,requested_at,started_at,updated_at)
		VALUES($1,'INDEXING',1,$2,$3,NOW(),NOW(),NOW())
		ON CONFLICT(document_set_id) DO UPDATE SET status='INDEXING',
		generation=document_index_status.generation+1,input_fingerprint=EXCLUDED.input_fingerprint,
		embedding_model=EXCLUDED.embedding_model,error_message='',requested_at=NOW(),started_at=NOW(),
		finished_at=NULL,updated_at=NOW()
		RETURNING document_set_id,status,generation,input_fingerprint,version_count,
		skipped_version_count,chunk_count,warning_count,embedding_model,error_message,
		requested_at,started_at,finished_at,updated_at`
	var result IndexStatus
	if err := tx.QueryRow(ctx, upsert, setID, fingerprint, model).Scan(indexStatusDestinations(&result)...); err != nil {
		return IndexStatus{}, false, fmt.Errorf("mark document index started: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return IndexStatus{}, false, fmt.Errorf("commit document index start: %w", err)
	}
	return result, false, nil
}

func (r *IndexRepository) Complete(ctx context.Context, started IndexStatus, chunks []SemanticChunk,
	versionCount, skippedCount, warningCount int,
) (IndexStatus, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return IndexStatus{}, fmt.Errorf("begin save document index: %w", err)
	}
	defer tx.Rollback(ctx)
	const insertChunk = `INSERT INTO document_chunks
		(document_set_id,document_version_id,document_block_id,chunk_key,parent_chunk_key,
		 chunk_type,flow_type,identifier,title,content,raw_content,content_hash,source_locator,
		 embedding_model,embedding,metadata)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::vector,$16)
		ON CONFLICT(document_version_id,chunk_key) DO UPDATE SET
		document_block_id=EXCLUDED.document_block_id,parent_chunk_key=EXCLUDED.parent_chunk_key,
		chunk_type=EXCLUDED.chunk_type,flow_type=EXCLUDED.flow_type,identifier=EXCLUDED.identifier,
		title=EXCLUDED.title,content=EXCLUDED.content,raw_content=EXCLUDED.raw_content,
		content_hash=EXCLUDED.content_hash,source_locator=EXCLUDED.source_locator,
		embedding_model=EXCLUDED.embedding_model,embedding=EXCLUDED.embedding,
		metadata=EXCLUDED.metadata,updated_at=NOW() RETURNING id`
	for index := range chunks {
		chunk := &chunks[index]
		vector, err := encodeDocumentVector(chunk.Embedding)
		if err != nil {
			return IndexStatus{}, err
		}
		if err := tx.QueryRow(ctx, insertChunk, chunk.DocumentSetID, chunk.DocumentVersionID,
			chunk.DocumentBlockID, chunk.ChunkKey, chunk.ParentChunkKey, chunk.ChunkType,
			chunk.FlowType, chunk.Identifier, chunk.Title, chunk.Content, chunk.RawContent,
			chunk.ContentHash, chunk.SourceLocator, chunk.EmbeddingModel, vector, chunk.Metadata).
			Scan(&chunk.ID); err != nil {
			return IndexStatus{}, fmt.Errorf("save semantic chunk %d: %w", index+1, err)
		}
		for sourceIndex, blockID := range chunk.SourceBlockIDs {
			_, err := tx.Exec(ctx, `INSERT INTO document_chunk_source_blocks
				(document_chunk_id,document_version_id,document_block_id,ordinal)
				VALUES($1,$2,$3,$4) ON CONFLICT(document_chunk_id,document_block_id) DO NOTHING`,
				chunk.ID, chunk.DocumentVersionID, blockID, sourceIndex+1)
			if err != nil {
				return IndexStatus{}, fmt.Errorf("save chunk source block: %w", err)
			}
		}
	}
	const complete = `UPDATE document_index_status SET status='READY',version_count=$3,
		skipped_version_count=$4,chunk_count=$5,warning_count=$6,error_message='',
		finished_at=NOW(),updated_at=NOW()
		WHERE document_set_id=$1 AND generation=$2 AND status='INDEXING'
		RETURNING document_set_id,status,generation,input_fingerprint,version_count,
		skipped_version_count,chunk_count,warning_count,embedding_model,error_message,
		requested_at,started_at,finished_at,updated_at`
	var result IndexStatus
	if err := tx.QueryRow(ctx, complete, started.DocumentSetID, started.Generation,
		versionCount, skippedCount, len(chunks), warningCount).Scan(indexStatusDestinations(&result)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return IndexStatus{}, ErrLeaseLost
		}
		return IndexStatus{}, fmt.Errorf("complete document index: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return IndexStatus{}, fmt.Errorf("commit document index: %w", err)
	}
	return result, nil
}

func (r *IndexRepository) Fail(ctx context.Context, started IndexStatus, processErr error) error {
	result, err := r.pool.Exec(ctx, `UPDATE document_index_status SET status='FAILED',
		error_message=$3,finished_at=NOW(),updated_at=NOW()
		WHERE document_set_id=$1 AND generation=$2 AND status='INDEXING'`,
		started.DocumentSetID, started.Generation, processErr.Error())
	if err != nil {
		return fmt.Errorf("fail document index: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (r *IndexRepository) ListChunks(ctx context.Context, setID int64, chunkType, flowType string,
	limit int,
) ([]SemanticChunk, error) {
	if limit == -1 {
		limit = 2147483647
	} else if limit <= 0 || limit > 500 {
		limit = 200
	}
	const query = `SELECT c.id,c.document_set_id,c.document_version_id,COALESCE(c.document_block_id,0),
		c.chunk_key,c.parent_chunk_key,c.chunk_type,c.flow_type,c.identifier,c.title,c.content,
		c.raw_content,c.content_hash,c.source_locator,c.embedding_model,c.metadata,v.approval_status,
		c.created_at,c.updated_at
		FROM document_chunks c JOIN document_versions v ON v.id=c.document_version_id
		WHERE c.document_set_id=$1 AND ($2='' OR c.chunk_type=$2) AND ($3='' OR c.flow_type=$3)
		ORDER BY c.document_version_id,c.id LIMIT $4`
	rows, err := r.pool.Query(ctx, query, setID, chunkType, flowType, limit)
	if err != nil {
		return nil, fmt.Errorf("list semantic chunks: %w", err)
	}
	defer rows.Close()
	results := make([]SemanticChunk, 0)
	for rows.Next() {
		var item SemanticChunk
		if err := rows.Scan(chunkDestinations(&item)...); err != nil {
			return nil, fmt.Errorf("scan semantic chunk: %w", err)
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (r *IndexRepository) Retrieve(ctx context.Context, query RetrievalQuery,
	embedding []float32,
) ([]SemanticChunk, error) {
	vector, err := encodeDocumentVector(embedding)
	if err != nil {
		return nil, err
	}
	const statement = `WITH latest AS (
		SELECT DISTINCT ON (document_id) id FROM document_versions
		WHERE document_set_id=$1 ORDER BY document_id,version_number DESC
	), latest_approved AS (
		SELECT DISTINCT ON (document_id) id FROM document_versions
		WHERE document_set_id=$1 AND approval_status='APPROVED'
		ORDER BY document_id,version_number DESC
	), scored AS (
		SELECT c.id,c.document_set_id,c.document_version_id,COALESCE(c.document_block_id,0) AS document_block_id,
		c.chunk_key,c.parent_chunk_key,c.chunk_type,c.flow_type,c.identifier,c.title,c.content,
		c.raw_content,c.content_hash,c.source_locator,c.embedding_model,c.metadata,v.approval_status,
		c.created_at,c.updated_at,
		CASE WHEN $3<>'' AND lower(c.identifier)=lower($3) THEN 10.0 ELSE 0.0 END AS exact_score,
		ts_rank_cd(c.search_vector,plainto_tsquery('simple',$2))*4.0 AS lexical_score,
		CASE WHEN c.embedding IS NULL OR (c.embedding <=> $8::vector)='NaN'::double precision THEN 0.0
			ELSE GREATEST(0.0,1.0-(c.embedding <=> $8::vector))*2.0 END AS semantic_score,
		CASE v.approval_status WHEN 'APPROVED' THEN 1.0 WHEN 'DRAFT' THEN 0.0 ELSE -1.0 END AS authority_score
		FROM document_chunks c JOIN document_versions v ON v.id=c.document_version_id
		WHERE c.document_set_id=$1
		AND ($4='' OR c.chunk_type=$4) AND ($5='' OR c.flow_type=$5)
		AND ($6='ALL_INDEXED' OR ($6='LATEST' AND c.document_version_id IN (SELECT id FROM latest))
			OR ($6='LATEST_APPROVED' AND c.document_version_id IN (SELECT id FROM latest_approved)))
	)
	SELECT id,document_set_id,document_version_id,document_block_id,chunk_key,parent_chunk_key,
		chunk_type,flow_type,identifier,title,content,raw_content,content_hash,source_locator,
		embedding_model,metadata,approval_status,created_at,updated_at,
		exact_score,lexical_score,semantic_score,authority_score,
		exact_score+lexical_score+semantic_score+authority_score AS total_score
	FROM scored ORDER BY total_score DESC,document_version_id,id LIMIT $7`
	rows, err := r.pool.Query(ctx, statement, query.DocumentSetID, query.Query, query.Identifier,
		query.ChunkType, query.FlowType, query.VersionPolicy, query.Limit, vector)
	if err != nil {
		return nil, fmt.Errorf("retrieve document chunks: %w", err)
	}
	defer rows.Close()
	results := make([]SemanticChunk, 0)
	for rows.Next() {
		var item SemanticChunk
		destinations := chunkDestinations(&item)
		destinations = append(destinations, &item.ExactScore, &item.LexicalScore,
			&item.SemanticScore, &item.AuthorityScore, &item.Score)
		if err := rows.Scan(destinations...); err != nil {
			return nil, fmt.Errorf("scan retrieved document chunk: %w", err)
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (r *IndexRepository) SaveSnapshot(ctx context.Context, purpose string,
	query RetrievalQuery, status IndexStatus, items []SemanticChunk,
) (ContextSnapshot, error) {
	config, err := json.Marshal(query)
	if err != nil {
		return ContextSnapshot{}, fmt.Errorf("encode retrieval snapshot config: %w", err)
	}
	hasher := sha256.New()
	for _, item := range items {
		_, _ = fmt.Fprintf(hasher, "%d\x00%s\x00%s\x00%.8f\n", item.ID, item.ContentHash,
			item.ApprovalStatus, item.Score)
	}
	snapshotHash := hex.EncodeToString(hasher.Sum(nil))
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ContextSnapshot{}, fmt.Errorf("begin context snapshot: %w", err)
	}
	defer tx.Rollback(ctx)
	var result ContextSnapshot
	const insertSnapshot = `INSERT INTO document_context_snapshots
		(document_set_id,purpose,query_text,version_policy,index_generation,embedding_model,
		 retrieval_config,snapshot_hash)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id,document_set_id,purpose,query_text,version_policy,index_generation,
		embedding_model,retrieval_config,snapshot_hash,created_at`
	if err := tx.QueryRow(ctx, insertSnapshot, query.DocumentSetID, purpose, query.Query,
		query.VersionPolicy, status.Generation, status.EmbeddingModel, config, snapshotHash).
		Scan(&result.ID, &result.DocumentSetID, &result.Purpose, &result.QueryText,
			&result.VersionPolicy, &result.IndexGeneration, &result.EmbeddingModel,
			&result.RetrievalConfig, &result.SnapshotHash, &result.CreatedAt); err != nil {
		return ContextSnapshot{}, fmt.Errorf("insert context snapshot: %w", err)
	}
	const insertItem = `INSERT INTO document_context_snapshot_items
		(context_snapshot_id,document_set_id,ordinal,document_chunk_id,document_version_id,
		 chunk_key,chunk_type,identifier,title,content,content_hash,source_locator,approval_status,
		 exact_score,lexical_score,semantic_score,authority_score,total_score,metadata)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`
	for index, item := range items {
		if _, err := tx.Exec(ctx, insertItem, result.ID, result.DocumentSetID, index+1,
			item.ID, item.DocumentVersionID, item.ChunkKey, item.ChunkType, item.Identifier,
			item.Title, item.Content, item.ContentHash, item.SourceLocator, item.ApprovalStatus,
			item.ExactScore, item.LexicalScore, item.SemanticScore, item.AuthorityScore,
			item.Score, item.Metadata); err != nil {
			return ContextSnapshot{}, fmt.Errorf("insert context snapshot item: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ContextSnapshot{}, fmt.Errorf("commit context snapshot: %w", err)
	}
	result.Items = append([]SemanticChunk(nil), items...)
	return result, nil
}

func (r *IndexRepository) ReviewVersion(ctx context.Context, versionID int64,
	input VersionReviewInput,
) (VersionReview, error) {
	input.ReviewerName = strings.TrimSpace(input.ReviewerName)
	input.Decision = strings.ToUpper(strings.TrimSpace(input.Decision))
	input.Comment = strings.TrimSpace(input.Comment)
	if input.ReviewerName == "" || input.Decision != ApprovalApproved && input.Decision != ApprovalRejected {
		return VersionReview{}, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return VersionReview{}, fmt.Errorf("begin document review: %w", err)
	}
	defer tx.Rollback(ctx)
	var current, parseStatus string
	if err := tx.QueryRow(ctx, `SELECT approval_status,parse_status FROM document_versions WHERE id=$1 FOR UPDATE`, versionID).Scan(&current, &parseStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return VersionReview{}, ErrNotFound
		}
		return VersionReview{}, fmt.Errorf("lock document version: %w", err)
	}
	if input.Decision == ApprovalApproved && parseStatus != ParseParsed {
		return VersionReview{}, ErrSourceReviewBlocked
	}
	if current == ApprovalApproved && input.Decision == ApprovalRejected {
		var approvedDependents int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM requirement_evidence e
			JOIN requirements r ON r.id=e.requirement_id
			WHERE e.document_version_id=$1 AND r.status='APPROVED'`, versionID).Scan(&approvedDependents); err != nil {
			return VersionReview{}, fmt.Errorf("check approved source dependents: %w", err)
		}
		if approvedDependents > 0 {
			return VersionReview{}, ErrSourceReviewBlocked
		}
	}
	var review VersionReview
	if err := tx.QueryRow(ctx, `INSERT INTO document_version_reviews
		(document_version_id,reviewer_name,decision,comment) VALUES($1,$2,$3,$4)
		RETURNING id,document_version_id,reviewer_name,decision,comment,created_at`,
		versionID, input.ReviewerName, input.Decision, input.Comment).
		Scan(&review.ID, &review.DocumentVersionID, &review.ReviewerName, &review.Decision,
			&review.Comment, &review.CreatedAt); err != nil {
		return VersionReview{}, fmt.Errorf("save document version review: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE document_versions SET approval_status=$2 WHERE id=$1`,
		versionID, input.Decision); err != nil {
		return VersionReview{}, fmt.Errorf("apply document version review: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return VersionReview{}, fmt.Errorf("commit document review: %w", err)
	}
	return review, nil
}

func indexStatusDestinations(item *IndexStatus) []any {
	return []any{&item.DocumentSetID, &item.Status, &item.Generation, &item.InputFingerprint,
		&item.VersionCount, &item.SkippedVersionCount, &item.ChunkCount, &item.WarningCount,
		&item.EmbeddingModel, &item.ErrorMessage, &item.RequestedAt, &item.StartedAt,
		&item.FinishedAt, &item.UpdatedAt}
}

func chunkDestinations(item *SemanticChunk) []any {
	return []any{&item.ID, &item.DocumentSetID, &item.DocumentVersionID, &item.DocumentBlockID,
		&item.ChunkKey, &item.ParentChunkKey, &item.ChunkType, &item.FlowType,
		&item.Identifier, &item.Title, &item.Content, &item.RawContent, &item.ContentHash,
		&item.SourceLocator, &item.EmbeddingModel, &item.Metadata, &item.ApprovalStatus,
		&item.CreatedAt, &item.UpdatedAt}
}

func encodeDocumentVector(values []float32) (string, error) {
	if len(values) != 384 {
		return "", fmt.Errorf("document embedding has %d dimensions, want 384", len(values))
	}
	var builder strings.Builder
	builder.WriteByte('[')
	for index, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return "", fmt.Errorf("document embedding contains a non-finite value")
		}
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	builder.WriteByte(']')
	return builder.String(), nil
}

func indexFingerprint(sources []IndexSource, model string) string {
	parts := make([]string, 0, len(sources))
	for _, source := range sources {
		parts = append(parts, fmt.Sprintf("%d:%s", source.Version.ID, source.Version.SHA256))
	}
	sort.Strings(parts)
	digest := sha256.Sum256([]byte(SemanticChunkerVersion + "\x00" + model + "\x00" + strings.Join(parts, "\n")))
	return hex.EncodeToString(digest[:])
}
