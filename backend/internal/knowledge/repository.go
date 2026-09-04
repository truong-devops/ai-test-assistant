package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrIndexNotFound  = errors.New("project index not found")
	ErrIndexLeaseLost = errors.New("project index lease lost")
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) RequestIndex(ctx context.Context, projectID int64, ref string) (IndexJob, error) {
	const query = `
		INSERT INTO project_indexes (project_id, ref, status)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id) DO UPDATE SET
			ref=EXCLUDED.ref, status=EXCLUDED.status,
			generation=project_indexes.generation + 1, attempt_count=0,
			error_message=NULL, next_attempt_at=NOW(), lease_expires_at=NULL,
			requested_at=NOW(), started_at=NULL, finished_at=NULL, updated_at=NOW()
		RETURNING project_id, ref, status, generation, attempt_count, file_count,
			skipped_file_count, chunk_count, embedding_model, COALESCE(error_message, ''),
			requested_at, started_at, finished_at, updated_at`
	var result IndexJob
	if err := r.pool.QueryRow(ctx, query, projectID, ref, IndexStatusPending).Scan(indexJobDestinations(&result)...); err != nil {
		return IndexJob{}, fmt.Errorf("request project index: %w", err)
	}
	return result, nil
}

func (r *Repository) GetIndex(ctx context.Context, projectID int64) (IndexJob, error) {
	const query = `
		SELECT project_id, ref, status, generation, attempt_count, file_count,
			skipped_file_count, chunk_count, embedding_model, COALESCE(error_message, ''),
			requested_at, started_at, finished_at, updated_at
		FROM project_indexes WHERE project_id=$1`
	var result IndexJob
	if err := r.pool.QueryRow(ctx, query, projectID).Scan(indexJobDestinations(&result)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return IndexJob{}, ErrIndexNotFound
		}
		return IndexJob{}, fmt.Errorf("get project index: %w", err)
	}
	return result, nil
}

func (r *Repository) ClaimNext(ctx context.Context, leaseDuration time.Duration) (IndexJob, error) {
	const query = `
		WITH next_index AS (
			SELECT project_id FROM project_indexes
			WHERE (status=$1 AND next_attempt_at <= NOW() AND lease_expires_at IS NULL)
			   OR (status=$2 AND lease_expires_at <= NOW())
			ORDER BY requested_at, project_id
			FOR UPDATE SKIP LOCKED LIMIT 1
		)
		UPDATE project_indexes AS indexes SET status=$2, error_message=NULL,
			attempt_count=CASE
				WHEN indexes.lease_expires_at IS NULL AND indexes.error_message IS NULL THEN 1
				ELSE indexes.attempt_count + 1
			END,
			lease_expires_at=NOW() + $3::interval,
			started_at=COALESCE(started_at, NOW()), updated_at=NOW()
		FROM next_index WHERE indexes.project_id=next_index.project_id
		RETURNING indexes.project_id, indexes.ref, indexes.status, indexes.generation,
			indexes.attempt_count, indexes.file_count, indexes.skipped_file_count,
			indexes.chunk_count, indexes.embedding_model, COALESCE(indexes.error_message, ''),
			indexes.requested_at, indexes.started_at, indexes.finished_at, indexes.updated_at`
	var result IndexJob
	if err := r.pool.QueryRow(ctx, query, IndexStatusPending, IndexStatusIndexing,
		leaseDuration.String()).Scan(indexJobDestinations(&result)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return IndexJob{}, ErrIndexNotFound
		}
		return IndexJob{}, fmt.Errorf("claim project index: %w", err)
	}
	return result, nil
}

func (r *Repository) RetryOrFail(ctx context.Context, claimed IndexJob, processErr error,
	maxAttempts int, retryDelay time.Duration,
) error {
	const query = `
		UPDATE project_indexes SET
			status=CASE WHEN attempt_count < $4 THEN $5 ELSE $6 END,
			error_message=$2,
			next_attempt_at=CASE WHEN attempt_count < $4 THEN NOW() + $7::interval ELSE next_attempt_at END,
			lease_expires_at=NULL,
			finished_at=CASE WHEN attempt_count < $4 THEN NULL ELSE NOW() END,
			updated_at=NOW()
		WHERE project_id=$1 AND generation=$3 AND status=$8 AND attempt_count=$9`
	result, err := r.pool.Exec(ctx, query, claimed.ProjectID, processErr.Error(), claimed.Generation,
		maxAttempts, IndexStatusPending, IndexStatusFailed, retryDelay.String(),
		IndexStatusIndexing, claimed.AttemptCount)
	if err != nil {
		return fmt.Errorf("retry or fail project index: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrIndexLeaseLost
	}
	return nil
}

func (r *Repository) RenewLease(ctx context.Context, claimed IndexJob, leaseDuration time.Duration) error {
	result, err := r.pool.Exec(ctx, `UPDATE project_indexes SET
		lease_expires_at=NOW() + $4::interval, updated_at=NOW()
		WHERE project_id=$1 AND generation=$2 AND status=$5 AND attempt_count=$3`,
		claimed.ProjectID, claimed.Generation, claimed.AttemptCount, leaseDuration.String(), IndexStatusIndexing)
	if err != nil {
		return fmt.Errorf("renew project index lease: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrIndexLeaseLost
	}
	return nil
}

func (r *Repository) ContentFingerprints(ctx context.Context, projectID int64) (map[string]ChunkFingerprint, error) {
	rows, err := r.pool.Query(ctx, `SELECT chunk_key, content_hash, embedding_model
		FROM knowledge_chunks WHERE project_id=$1`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query knowledge fingerprints: %w", err)
	}
	defer rows.Close()
	results := make(map[string]ChunkFingerprint)
	for rows.Next() {
		var key string
		var fingerprint ChunkFingerprint
		if err := rows.Scan(&key, &fingerprint.ContentHash, &fingerprint.EmbeddingModel); err != nil {
			return nil, fmt.Errorf("scan knowledge fingerprint: %w", err)
		}
		results[key] = fingerprint
	}
	return results, rows.Err()
}

func (r *Repository) SaveIndex(ctx context.Context, claimed IndexJob, chunks []KnowledgeChunk,
	fileCount, skippedFileCount int, embeddingModel string,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save project index: %w", err)
	}
	defer tx.Rollback(ctx)

	var status string
	var generation int64
	var attempt int
	if err := tx.QueryRow(ctx, `SELECT status, generation, attempt_count FROM project_indexes
		WHERE project_id=$1 FOR UPDATE`, claimed.ProjectID).Scan(&status, &generation, &attempt); err != nil {
		return fmt.Errorf("lock project index: %w", err)
	}
	if status != IndexStatusIndexing || generation != claimed.Generation || attempt != claimed.AttemptCount {
		return ErrIndexLeaseLost
	}

	keys := make([]string, 0, len(chunks))
	seen := make(map[string]struct{}, len(chunks))
	for _, chunk := range chunks {
		if chunk.ProjectID != claimed.ProjectID || chunk.ChunkKey == "" || chunk.ContentHash == "" {
			return fmt.Errorf("invalid knowledge chunk %q", chunk.SymbolName)
		}
		if _, duplicate := seen[chunk.ChunkKey]; duplicate {
			return fmt.Errorf("duplicate knowledge chunk key %q", chunk.ChunkKey)
		}
		seen[chunk.ChunkKey] = struct{}{}
		keys = append(keys, chunk.ChunkKey)
		metadata, err := json.Marshal(chunk.Metadata)
		if err != nil {
			return fmt.Errorf("encode metadata for %q: %w", chunk.SymbolName, err)
		}
		if len(chunk.Embedding) == 0 {
			result, err := tx.Exec(ctx, `UPDATE knowledge_chunks SET file_path=$3, package_name=$4,
				symbol_name=$5, chunk_type=$6, content=$7, start_line=$8, end_line=$9,
				metadata=$10, updated_at=CASE WHEN start_line<>$8 OR end_line<>$9 OR metadata<>$10
					THEN NOW() ELSE updated_at END
				WHERE project_id=$1 AND chunk_key=$2 AND content_hash=$11 AND embedding_model=$12`,
				chunk.ProjectID, chunk.ChunkKey, chunk.FilePath, chunk.PackageName, chunk.SymbolName,
				chunk.ChunkType, chunk.Content, chunk.StartLine, chunk.EndLine, metadata,
				chunk.ContentHash, chunk.EmbeddingModel)
			if err != nil {
				return fmt.Errorf("update unchanged knowledge chunk %q: %w", chunk.SymbolName, err)
			}
			if result.RowsAffected() != 1 {
				return fmt.Errorf("unchanged knowledge chunk %q was not found", chunk.SymbolName)
			}
			continue
		}
		vector, err := encodeVector(chunk.Embedding, EmbeddingDimensions)
		if err != nil {
			return fmt.Errorf("encode embedding for %q: %w", chunk.SymbolName, err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO knowledge_chunks
			(project_id, chunk_key, file_path, package_name, symbol_name, chunk_type, content,
			 content_hash, start_line, end_line, embedding_model, embedding, metadata)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::vector,$13)
			ON CONFLICT (project_id, chunk_key) DO UPDATE SET
				file_path=EXCLUDED.file_path, package_name=EXCLUDED.package_name,
				symbol_name=EXCLUDED.symbol_name, chunk_type=EXCLUDED.chunk_type,
				content=EXCLUDED.content, content_hash=EXCLUDED.content_hash,
				start_line=EXCLUDED.start_line, end_line=EXCLUDED.end_line,
				embedding_model=EXCLUDED.embedding_model, embedding=EXCLUDED.embedding,
				metadata=EXCLUDED.metadata, updated_at=NOW()`,
			chunk.ProjectID, chunk.ChunkKey, chunk.FilePath, chunk.PackageName, chunk.SymbolName,
			chunk.ChunkType, chunk.Content, chunk.ContentHash, chunk.StartLine, chunk.EndLine,
			chunk.EmbeddingModel, vector, metadata)
		if err != nil {
			return fmt.Errorf("upsert knowledge chunk %q: %w", chunk.SymbolName, err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM knowledge_chunks
		WHERE project_id=$1 AND NOT (chunk_key = ANY($2::text[]))`, claimed.ProjectID, keys); err != nil {
		return fmt.Errorf("delete stale knowledge chunks: %w", err)
	}
	result, err := tx.Exec(ctx, `UPDATE project_indexes SET status=$2, file_count=$3,
		skipped_file_count=$4, chunk_count=$5, embedding_model=$6, error_message=NULL,
		lease_expires_at=NULL, finished_at=NOW(), updated_at=NOW(), attempt_count=0
		WHERE project_id=$1 AND generation=$7 AND status=$8 AND attempt_count=$9`,
		claimed.ProjectID, IndexStatusReady, fileCount, skippedFileCount, len(chunks), embeddingModel,
		claimed.Generation, IndexStatusIndexing, claimed.AttemptCount)
	if err != nil {
		return fmt.Errorf("complete project index: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrIndexLeaseLost
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit project index: %w", err)
	}
	return nil
}

func (r *Repository) Retrieve(ctx context.Context, query RetrievalQuery, embedding []float32) ([]KnowledgeChunk, error) {
	vector, err := encodeVector(embedding, EmbeddingDimensions)
	if err != nil {
		return nil, err
	}
	directory := path.Dir(query.FilePath)
	if directory == "." {
		directory = ""
	}
	const statement = `
		WITH scored AS (
			SELECT knowledge_chunks.id, knowledge_chunks.project_id, chunk_key, file_path,
				package_name, symbol_name, chunk_type, content, content_hash, start_line,
				end_line, knowledge_chunks.embedding_model,
				COALESCE(indexes.ref, '') AS index_ref,
				COALESCE(indexes.generation, 0) AS index_generation,
				metadata, created_at, knowledge_chunks.updated_at,
				CASE WHEN $3<>'' AND lower(symbol_name)=lower($3) THEN 5.0 ELSE 0.0 END +
				CASE WHEN $4<>'' AND lower(package_name)=lower($4) THEN 3.0 ELSE 0.0 END +
				CASE WHEN $5<>'' AND file_path=$5 THEN 1.0 ELSE 0.0 END +
				CASE WHEN $6<>'' AND file_path LIKE $6 || '/%' THEN 0.5 ELSE 0.0 END +
				CASE WHEN $7 AND chunk_type IN ('test','test_helper','mock') THEN 4.0 ELSE 0.0 END +
				CASE WHEN chunk_type='mock' THEN 1.5 ELSE 0.0 END AS structural_score,
				ts_rank_cd(search_vector, plainto_tsquery('simple', $2)) * 4.0 AS lexical_score,
				CASE WHEN (embedding <=> $8::vector) = 'NaN'::double precision THEN 0.0
					ELSE GREATEST(0.0, 1.0 - (embedding <=> $8::vector)) * 2.0
				END AS semantic_score
			FROM knowledge_chunks
			LEFT JOIN project_indexes AS indexes ON indexes.project_id=knowledge_chunks.project_id
			WHERE knowledge_chunks.project_id=$1
		)
		SELECT id, project_id, chunk_key, file_path, package_name, symbol_name,
			chunk_type, content, content_hash, start_line, end_line, embedding_model,
			index_ref, index_generation, metadata,
			structural_score + lexical_score + semantic_score AS score,
			created_at, updated_at
		FROM scored
		ORDER BY score DESC, file_path, start_line, id
		LIMIT $9`
	rows, err := r.pool.Query(ctx, statement, query.ProjectID, query.Query, query.SymbolName,
		query.PackageName, query.FilePath, directory, query.PreferTests, vector, query.Limit)
	if err != nil {
		return nil, fmt.Errorf("retrieve knowledge chunks: %w", err)
	}
	defer rows.Close()
	results := make([]KnowledgeChunk, 0)
	for rows.Next() {
		var item KnowledgeChunk
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.ChunkKey, &item.FilePath,
			&item.PackageName, &item.SymbolName, &item.ChunkType, &item.Content,
			&item.ContentHash, &item.StartLine, &item.EndLine, &item.EmbeddingModel,
			&item.IndexRef, &item.IndexGeneration, &item.Metadata, &item.Score,
			&item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan retrieved knowledge chunk: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate retrieved knowledge chunks: %w", err)
	}
	return results, nil
}

func ContentHash(content string) string {
	digest := sha256.Sum256([]byte(content))
	return hex.EncodeToString(digest[:])
}

func encodeVector(values []float32, dimensions int) (string, error) {
	if len(values) != dimensions {
		return "", fmt.Errorf("embedding has %d dimensions, want %d", len(values), dimensions)
	}
	var builder strings.Builder
	builder.WriteByte('[')
	for index, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return "", fmt.Errorf("embedding contains a non-finite value")
		}
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	builder.WriteByte(']')
	return builder.String(), nil
}

func indexJobDestinations(item *IndexJob) []any {
	return []any{&item.ProjectID, &item.Ref, &item.Status, &item.Generation,
		&item.AttemptCount, &item.FileCount, &item.SkippedFileCount, &item.ChunkCount,
		&item.EmbeddingModel, &item.ErrorMessage, &item.RequestedAt, &item.StartedAt,
		&item.FinishedAt, &item.UpdatedAt}
}
