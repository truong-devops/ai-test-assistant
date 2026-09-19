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

func (r *IndexRepository) GetStatus(ctx context.Context, setID int64) (IndexStatus, error) {
	const query = `SELECT document_set_id,status,generation,input_fingerprint,version_count,
		skipped_version_count,chunk_count,warning_count,embedding_model,error_message,
		requested_at,started_at,finished_at,updated_at,source_snapshot_id,
		indexed_source_revision,content_warning_count
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
		result = IndexStatus{DocumentSetID: setID, Status: IndexNotIndexed}
		return r.enrichStatus(ctx, result)
	}
	if err != nil {
		return IndexStatus{}, fmt.Errorf("get document index status: %w", err)
	}
	return r.enrichStatus(ctx, result)
}

func (r *IndexRepository) Begin(ctx context.Context, snapshot SourceSnapshot, fingerprint,
	model string,
) (IndexStatus, bool, error) {
	setID := snapshot.DocumentSetID
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
		requested_at,started_at,finished_at,updated_at,source_snapshot_id,
		indexed_source_revision,content_warning_count
		FROM document_index_status WHERE document_set_id=$1 FOR UPDATE`, setID).
		Scan(indexStatusDestinations(&current)...)
	if err == nil && (current.Status == IndexReady || current.Status == IndexIndexing) &&
		current.InputFingerprint == fingerprint && current.EmbeddingModel == model &&
		current.SourceSnapshotID != nil && *current.SourceSnapshotID == snapshot.ID {
		if err := tx.Commit(ctx); err != nil {
			return IndexStatus{}, false, fmt.Errorf("commit unchanged index check: %w", err)
		}
		result, statusErr := r.GetStatus(ctx, setID)
		return result, true, statusErr
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return IndexStatus{}, false, fmt.Errorf("lock document index status: %w", err)
	}
	if err == nil && current.Status == IndexIndexing && current.Generation > 0 {
		if _, updateErr := tx.Exec(ctx, `UPDATE document_index_generations SET status='SUPERSEDED',
			finished_at=NOW(),updated_at=NOW(),error_message='superseded by a newer generation'
			WHERE document_set_id=$1 AND generation=$2 AND status='INDEXING'`, setID,
			current.Generation); updateErr != nil {
			return IndexStatus{}, false, fmt.Errorf("supersede document index generation: %w", updateErr)
		}
	}
	const upsert = `INSERT INTO document_index_status
		(document_set_id,status,generation,input_fingerprint,embedding_model,requested_at,started_at,
		 updated_at,source_snapshot_id,indexed_source_revision,content_warning_count)
		VALUES($1,'INDEXING',1,$2,$3,NOW(),NOW(),NOW(),$4,$5,0)
		ON CONFLICT(document_set_id) DO UPDATE SET status='INDEXING',
		generation=document_index_status.generation+1,input_fingerprint=EXCLUDED.input_fingerprint,
		embedding_model=EXCLUDED.embedding_model,error_message='',requested_at=NOW(),started_at=NOW(),
		finished_at=NULL,updated_at=NOW(),source_snapshot_id=EXCLUDED.source_snapshot_id,
		indexed_source_revision=EXCLUDED.indexed_source_revision,content_warning_count=0
		RETURNING document_set_id,status,generation,input_fingerprint,version_count,
		skipped_version_count,chunk_count,warning_count,embedding_model,error_message,
		requested_at,started_at,finished_at,updated_at,source_snapshot_id,
		indexed_source_revision,content_warning_count`
	var result IndexStatus
	if err := tx.QueryRow(ctx, upsert, setID, fingerprint, model, snapshot.ID,
		snapshot.SourceRevision).Scan(indexStatusDestinations(&result)...); err != nil {
		return IndexStatus{}, false, fmt.Errorf("mark document index started: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO document_index_generations
		(document_set_id,generation,source_snapshot_id,source_revision,input_fingerprint,
		 embedding_model,status,requested_at,started_at)
		VALUES($1,$2,$3,$4,$5,$6,'INDEXING',NOW(),NOW())`, setID, result.Generation,
		snapshot.ID, snapshot.SourceRevision, fingerprint, model); err != nil {
		return IndexStatus{}, false, fmt.Errorf("create document index generation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return IndexStatus{}, false, fmt.Errorf("commit document index start: %w", err)
	}
	result, err = r.GetStatus(ctx, setID)
	return result, false, err
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
		if _, err := tx.Exec(ctx, `INSERT INTO document_index_generation_chunks
			(document_set_id,generation,ordinal,document_chunk_id,document_version_id)
			VALUES($1,$2,$3,$4,$5)`, started.DocumentSetID, started.Generation, index+1,
			chunk.ID, chunk.DocumentVersionID); err != nil {
			return IndexStatus{}, fmt.Errorf("save document index membership: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE document_index_generations SET status='READY',
		version_count=$3,excluded_version_count=$4,chunk_count=$5,content_warning_count=$6,
		error_message='',finished_at=NOW(),updated_at=NOW()
		WHERE document_set_id=$1 AND generation=$2 AND status='INDEXING'`,
		started.DocumentSetID, started.Generation, versionCount, skippedCount, len(chunks),
		warningCount); err != nil {
		return IndexStatus{}, fmt.Errorf("complete document index generation: %w", err)
	}
	const complete = `UPDATE document_index_status SET status='READY',version_count=$3,
		skipped_version_count=$4,chunk_count=$5,warning_count=$6,error_message='',
		content_warning_count=$6,finished_at=NOW(),updated_at=NOW()
		WHERE document_set_id=$1 AND generation=$2 AND status='INDEXING'
		RETURNING generation`
	var completedGeneration int64
	if err := tx.QueryRow(ctx, complete, started.DocumentSetID, started.Generation,
		versionCount, skippedCount, len(chunks), warningCount).Scan(&completedGeneration); err != nil &&
		!errors.Is(err, pgx.ErrNoRows) {
		return IndexStatus{}, fmt.Errorf("complete document index: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return IndexStatus{}, fmt.Errorf("commit document index: %w", err)
	}
	return r.GetStatus(ctx, started.DocumentSetID)
}

func (r *IndexRepository) Fail(ctx context.Context, started IndexStatus, processErr error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin fail document index: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE document_index_generations SET status='FAILED',
		error_message=$3,finished_at=NOW(),updated_at=NOW()
		WHERE document_set_id=$1 AND generation=$2 AND status='INDEXING'`,
		started.DocumentSetID, started.Generation, processErr.Error()); err != nil {
		return fmt.Errorf("fail document index generation: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE document_index_status SET status='FAILED',
		error_message=$3,finished_at=NOW(),updated_at=NOW()
		WHERE document_set_id=$1 AND generation=$2 AND status='INDEXING'`,
		started.DocumentSetID, started.Generation, processErr.Error()); err != nil {
		return fmt.Errorf("fail document index: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *IndexRepository) ListChunks(ctx context.Context, setID, generation int64,
	chunkType, flowType string, limit int,
) ([]SemanticChunk, error) {
	if limit == -1 {
		limit = 2147483647
	} else if limit <= 0 || limit > 500 {
		limit = 200
	}
	if generation == 0 {
		if err := r.pool.QueryRow(ctx, `SELECT generation FROM document_index_status
			WHERE document_set_id=$1`, setID).Scan(&generation); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return []SemanticChunk{}, nil
			}
			return nil, fmt.Errorf("resolve current document index generation: %w", err)
		}
	}
	const query = `SELECT c.id,c.document_set_id,c.document_version_id,v.version_number,
		COALESCE(c.document_block_id,0),
		c.chunk_key,c.parent_chunk_key,c.chunk_type,c.flow_type,c.identifier,c.title,c.content,
		c.raw_content,c.content_hash,c.source_locator,c.embedding_model,c.metadata,v.approval_status,
		c.created_at,c.updated_at
		FROM document_index_generation_chunks m
		JOIN document_chunks c ON c.id=m.document_chunk_id
		JOIN document_versions v ON v.id=c.document_version_id
		WHERE m.document_set_id=$1 AND m.generation=$2
		AND ($3='' OR c.chunk_type=$3) AND ($4='' OR c.flow_type=$4)
		ORDER BY m.ordinal LIMIT $5`
	rows, err := r.pool.Query(ctx, query, setID, generation, chunkType, flowType, limit)
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
	const statement = `WITH scored AS (
		SELECT c.id,c.document_set_id,c.document_version_id,v.version_number,
		COALESCE(c.document_block_id,0) AS document_block_id,
		c.chunk_key,c.parent_chunk_key,c.chunk_type,c.flow_type,c.identifier,c.title,c.content,
		c.raw_content,c.content_hash,c.source_locator,c.embedding_model,c.metadata,v.approval_status,
		c.created_at,c.updated_at,
		CASE WHEN $3<>'' AND lower(c.identifier)=lower($3) THEN 10.0 ELSE 0.0 END AS exact_score,
		ts_rank_cd(c.search_vector,plainto_tsquery('simple',$2))*4.0 AS lexical_score,
		CASE WHEN c.embedding IS NULL OR (c.embedding <=> $9::vector)='NaN'::double precision THEN 0.0
			ELSE GREATEST(0.0,1.0-(c.embedding <=> $9::vector))*2.0 END AS semantic_score,
		CASE v.approval_status WHEN 'APPROVED' THEN 1.0 WHEN 'DRAFT' THEN 0.0 ELSE -1.0 END AS authority_score
		FROM document_index_generation_chunks m
		JOIN document_chunks c ON c.id=m.document_chunk_id
		JOIN document_versions v ON v.id=c.document_version_id
		WHERE m.document_set_id=$1 AND m.generation=$8
		AND ($4='' OR c.chunk_type=$4) AND ($5='' OR c.flow_type=$5)
		AND ($6 IN ('ALL_INDEXED','LATEST') OR ($6='LATEST_APPROVED' AND v.approval_status='APPROVED'))
	)
	SELECT id,document_set_id,document_version_id,version_number,document_block_id,chunk_key,parent_chunk_key,
		chunk_type,flow_type,identifier,title,content,raw_content,content_hash,source_locator,
		embedding_model,metadata,approval_status,created_at,updated_at,
		exact_score,lexical_score,semantic_score,authority_score,
		exact_score+lexical_score+semantic_score+authority_score AS total_score
	FROM scored ORDER BY total_score DESC,document_version_id,id LIMIT $7`
	rows, err := r.pool.Query(ctx, statement, query.DocumentSetID, query.Query, query.Identifier,
		query.ChunkType, query.FlowType, query.VersionPolicy, query.Limit,
		query.IndexGeneration, vector)
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
		 retrieval_config,snapshot_hash,source_snapshot_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id,document_set_id,purpose,query_text,version_policy,index_generation,
		embedding_model,retrieval_config,snapshot_hash,source_snapshot_id,created_at`
	if err := tx.QueryRow(ctx, insertSnapshot, query.DocumentSetID, purpose, query.Query,
		query.VersionPolicy, status.Generation, status.EmbeddingModel, config, snapshotHash,
		status.SourceSnapshotID).
		Scan(&result.ID, &result.DocumentSetID, &result.Purpose, &result.QueryText,
			&result.VersionPolicy, &result.IndexGeneration, &result.EmbeddingModel,
			&result.RetrievalConfig, &result.SnapshotHash, &result.SourceSnapshotID,
			&result.CreatedAt); err != nil {
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

func (r *IndexRepository) enrichStatus(ctx context.Context, result IndexStatus) (IndexStatus, error) {
	if err := r.pool.QueryRow(ctx, `SELECT source_revision FROM document_sets WHERE id=$1`,
		result.DocumentSetID).Scan(&result.SourceRevision); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return IndexStatus{}, ErrNotFound
		}
		return IndexStatus{}, fmt.Errorf("load current source revision: %w", err)
	}
	result.Freshness = IndexFreshnessStale
	if result.SourceSnapshotID != nil && result.IndexedSourceRevision == result.SourceRevision {
		result.Freshness = IndexFreshnessCurrent
	}
	result.Sources = []IndexVersionRef{}
	result.PendingVersions = []IndexVersionRef{}
	result.Generations = []IndexGeneration{}
	approved := true
	if result.SourceSnapshotID != nil {
		rows, err := r.pool.Query(ctx, `SELECT i.document_id,i.document_name,i.document_version_id,
			i.version_number,i.sha256,v.parse_status,v.approval_status,i.included,i.exclusion_reason
			FROM document_source_snapshot_items i
			JOIN document_versions v ON v.id=i.document_version_id
			WHERE i.source_snapshot_id=$1 ORDER BY i.document_id`, *result.SourceSnapshotID)
		if err != nil {
			return IndexStatus{}, fmt.Errorf("list current source snapshot: %w", err)
		}
		for rows.Next() {
			var item IndexVersionRef
			if err := rows.Scan(&item.DocumentID, &item.DocumentName, &item.DocumentVersionID,
				&item.VersionNumber, &item.SHA256, &item.ParseStatus, &item.ApprovalStatus,
				&item.Included, &item.ExclusionReason); err != nil {
				rows.Close()
				return IndexStatus{}, err
			}
			if item.Included && item.ApprovalStatus != ApprovalApproved {
				approved = false
			}
			result.Sources = append(result.Sources, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return IndexStatus{}, err
		}
		rows.Close()
	}
	rows, err := r.pool.Query(ctx, `WITH latest AS (
		SELECT DISTINCT ON (d.id) d.id AS document_id,d.name,v.id AS version_id,
			v.version_number,v.sha256,v.parse_status,v.approval_status
		FROM documents d JOIN document_versions v ON v.document_id=d.id
		WHERE d.document_set_id=$1 ORDER BY d.id,v.version_number DESC
	) SELECT l.document_id,l.name,l.version_id,l.version_number,l.sha256,l.parse_status,
		l.approval_status FROM latest l
	LEFT JOIN document_source_snapshot_items i ON i.source_snapshot_id=$2 AND
		i.document_id=l.document_id AND i.document_version_id=l.version_id
	WHERE i.id IS NULL ORDER BY l.document_id`, result.DocumentSetID, result.SourceSnapshotID)
	if err != nil {
		return IndexStatus{}, fmt.Errorf("list pending source versions: %w", err)
	}
	for rows.Next() {
		var item IndexVersionRef
		if err := rows.Scan(&item.DocumentID, &item.DocumentName, &item.DocumentVersionID,
			&item.VersionNumber, &item.SHA256, &item.ParseStatus, &item.ApprovalStatus); err != nil {
			rows.Close()
			return IndexStatus{}, err
		}
		item.Included = true
		result.PendingVersions = append(result.PendingVersions, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return IndexStatus{}, err
	}
	rows.Close()
	if result.Generation > 0 {
		result.WarningCount, err = r.dynamicWarningCount(ctx, result.DocumentSetID,
			result.Generation, result.ContentWarningCount)
		if err != nil {
			return IndexStatus{}, err
		}
	}
	result.ExtractionReady = result.Status == IndexReady &&
		result.Freshness == IndexFreshnessCurrent && approved
	result.Generations, err = r.ListGenerations(ctx, result.DocumentSetID)
	if err != nil {
		return IndexStatus{}, err
	}
	return result, nil
}

func indexStatusDestinations(item *IndexStatus) []any {
	return []any{&item.DocumentSetID, &item.Status, &item.Generation, &item.InputFingerprint,
		&item.VersionCount, &item.SkippedVersionCount, &item.ChunkCount, &item.WarningCount,
		&item.EmbeddingModel, &item.ErrorMessage, &item.RequestedAt, &item.StartedAt,
		&item.FinishedAt, &item.UpdatedAt, &item.SourceSnapshotID,
		&item.IndexedSourceRevision, &item.ContentWarningCount}
}

func chunkDestinations(item *SemanticChunk) []any {
	return []any{&item.ID, &item.DocumentSetID, &item.DocumentVersionID,
		&item.DocumentVersionNumber, &item.DocumentBlockID,
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
