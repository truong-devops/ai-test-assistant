package provenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidRecord = errors.New("invalid AI provenance record")

type Repository struct {
	pool   *pgxpool.Pool
	config RuntimeConfig
}

func NewRepository(pool *pgxpool.Pool, config RuntimeConfig) *Repository {
	config.Provider = strings.TrimSpace(config.Provider)
	config.Model = strings.TrimSpace(config.Model)
	return &Repository{pool: pool, config: config}
}

func (r *Repository) Record(ctx context.Context, input RecordInput) (Call, error) {
	if input.AttemptNumber <= 0 {
		input.AttemptNumber = 1
	}
	if err := validateRecordInput(input); err != nil {
		return Call{}, err
	}
	requestSchema, err := json.Marshal(input.Request.Schema)
	if err != nil {
		return Call{}, fmt.Errorf("encode provenance request schema: %w", err)
	}
	retrievalQuery, err := json.Marshal(map[string]any{
		"query": input.RetrievalQuery.Query, "package_name": input.RetrievalQuery.PackageName,
		"symbol_name": input.RetrievalQuery.SymbolName, "file_path": input.RetrievalQuery.FilePath,
		"prefer_tests": input.RetrievalQuery.PreferTests, "limit": input.RetrievalQuery.Limit,
	})
	if err != nil {
		return Call{}, fmt.Errorf("encode provenance retrieval query: %w", err)
	}
	model := strings.TrimSpace(input.Response.Model)
	if model == "" {
		model = r.config.Model
	}
	provider := r.config.Provider
	if provider == "" {
		provider = "unknown"
	}
	configuration := map[string]any{
		"provider": provider, "model": model, "prompt_version": input.PromptVersion,
		"schema_name": input.Request.SchemaName, "request_schema": input.Request.Schema,
		"max_output_tokens": input.Request.MaxOutputTokens,
		"retrieval_query":   json.RawMessage(retrievalQuery),
	}
	configurationHash, err := hashJSON(configuration)
	if err != nil {
		return Call{}, fmt.Errorf("hash provenance configuration: %w", err)
	}
	promptHash := HashText(input.Request.Instructions + "\x00" + input.Request.Input + "\x00" + string(requestSchema))
	cost := estimateCost(input.Response.Usage.InputTokens, input.Response.Usage.OutputTokens,
		r.config.InputCostPerMillionUSD, r.config.OutputCostPerMillionUSD)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Call{}, fmt.Errorf("begin AI provenance record: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := validateOwnership(ctx, tx, input); err != nil {
		return Call{}, err
	}

	changedSymbolID, recommendationID, generatedTestID := subjectColumns(input.Phase, input.SubjectID)
	const insertCall = `INSERT INTO llm_calls
		(analysis_job_id, project_id, phase, changed_symbol_id, recommendation_id,
		 generated_test_id, attempt_number, source_sha, target_sha, provider, model_name,
		 prompt_version, prompt_hash, configuration_hash, instructions, prompt_text,
		 schema_name, request_schema, provider_response_id, response_text, status,
		 error_message, input_tokens, output_tokens, total_tokens, latency_ms,
		 estimated_cost_usd)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,
			$18,$19,$20,$21,$22,$23,$24,$25,$26,$27)
		RETURNING id, created_at`
	result := Call{
		AnalysisJobID: input.Analysis.ID, ProjectID: input.Analysis.ProjectID,
		Phase: input.Phase, ChangedSymbolID: changedSymbolID, RecommendationID: recommendationID,
		GeneratedTestID: generatedTestID, AttemptNumber: input.AttemptNumber,
		SourceSHA: input.Analysis.SourceSHA, TargetSHA: input.Analysis.TargetSHA,
		Provider: provider, ModelName: model, PromptVersion: input.PromptVersion,
		PromptHash: promptHash, ConfigurationHash: configurationHash,
		Instructions: input.Request.Instructions, PromptText: input.Request.Input,
		SchemaName: input.Request.SchemaName, RequestSchema: requestSchema,
		ProviderResponseID: input.Response.ID, ResponseText: input.Response.Output,
		Status: input.Status, ErrorMessage: truncateBytes(input.ErrorMessage, 16_384),
		InputTokens: input.Response.Usage.InputTokens, OutputTokens: input.Response.Usage.OutputTokens,
		TotalTokens: input.Response.Usage.TotalTokens, LatencyMS: max(0, input.Latency.Milliseconds()),
		EstimatedCostUSD: cost,
	}
	if err := tx.QueryRow(ctx, insertCall, result.AnalysisJobID, result.ProjectID, result.Phase,
		result.ChangedSymbolID, result.RecommendationID, result.GeneratedTestID, result.AttemptNumber,
		result.SourceSHA, result.TargetSHA, result.Provider, result.ModelName, result.PromptVersion,
		result.PromptHash, result.ConfigurationHash, result.Instructions, result.PromptText,
		result.SchemaName, result.RequestSchema, result.ProviderResponseID, result.ResponseText,
		result.Status, result.ErrorMessage, result.InputTokens, result.OutputTokens,
		result.TotalTokens, result.LatencyMS, result.EstimatedCostUSD).
		Scan(&result.ID, &result.CreatedAt); err != nil {
		return Call{}, fmt.Errorf("insert LLM call provenance: %w", err)
	}

	snapshot, err := insertSnapshot(ctx, tx, result, input, retrievalQuery)
	if err != nil {
		return Call{}, err
	}
	result.Context = &snapshot
	if err := tx.Commit(ctx); err != nil {
		return Call{}, fmt.Errorf("commit AI provenance record: %w", err)
	}
	return result, nil
}

func insertSnapshot(ctx context.Context, tx pgx.Tx, call Call, input RecordInput,
	retrievalQuery []byte,
) (ContextSnapshot, error) {
	indexRef := ""
	var indexGeneration int64
	embeddingModel := ""
	if len(input.Contexts) > 0 {
		indexRef = input.Contexts[0].IndexRef
		indexGeneration = input.Contexts[0].IndexGeneration
		embeddingModel = input.Contexts[0].EmbeddingModel
	}
	if indexGeneration == 0 {
		err := tx.QueryRow(ctx, `SELECT ref, generation, embedding_model FROM project_indexes
			WHERE project_id=$1`, input.Analysis.ProjectID).
			Scan(&indexRef, &indexGeneration, &embeddingModel)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return ContextSnapshot{}, fmt.Errorf("read provenance index generation: %w", err)
		}
	}
	retrievalConfig, err := json.Marshal(map[string]any{
		"result_count": len(input.Contexts), "score_version": "hybrid-v1",
		"snapshot_content": true,
	})
	if err != nil {
		return ContextSnapshot{}, fmt.Errorf("encode provenance retrieval configuration: %w", err)
	}
	result := ContextSnapshot{
		LLMCallID: call.ID, ProjectID: input.Analysis.ProjectID,
		QueryText: strings.TrimSpace(input.RetrievalQuery.Query), RetrievalQuery: retrievalQuery,
		RetrievalConfig: retrievalConfig, IndexRef: indexRef, IndexGeneration: indexGeneration,
		EmbeddingModel: embeddingModel, Items: make([]ContextSnapshotItem, 0, len(input.Contexts)),
	}
	if result.QueryText == "" {
		result.QueryText = strings.Join([]string{input.RetrievalQuery.SymbolName,
			input.RetrievalQuery.PackageName, input.RetrievalQuery.FilePath}, " ")
	}
	const insert = `INSERT INTO context_snapshots
		(llm_call_id, project_id, query_text, retrieval_query, retrieval_config,
		 index_ref, index_generation, embedding_model)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, created_at`
	if err := tx.QueryRow(ctx, insert, result.LLMCallID, result.ProjectID, result.QueryText,
		result.RetrievalQuery, result.RetrievalConfig, result.IndexRef, result.IndexGeneration,
		result.EmbeddingModel).Scan(&result.ID, &result.CreatedAt); err != nil {
		return ContextSnapshot{}, fmt.Errorf("insert context snapshot: %w", err)
	}
	const insertItem = `INSERT INTO context_snapshot_items
		(context_snapshot_id, project_id, ordinal, knowledge_chunk_id, chunk_key,
		 file_path, package_name, symbol_name, chunk_type, content, content_hash,
		 start_line, end_line, embedding_model, score, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		RETURNING id, created_at`
	for index, chunk := range input.Contexts {
		if chunk.ProjectID != input.Analysis.ProjectID {
			return ContextSnapshot{}, fmt.Errorf("%w: context chunk %d belongs to project %d",
				ErrInvalidRecord, chunk.ID, chunk.ProjectID)
		}
		metadata := chunk.Metadata
		if len(metadata) == 0 {
			metadata = json.RawMessage(`{}`)
		}
		item := ContextSnapshotItem{
			ContextSnapshotID: result.ID, ProjectID: chunk.ProjectID, Ordinal: index + 1,
			KnowledgeChunkID: chunk.ID, ChunkKey: chunk.ChunkKey, FilePath: chunk.FilePath,
			PackageName: chunk.PackageName, SymbolName: chunk.SymbolName, ChunkType: chunk.ChunkType,
			Content: chunk.Content, ContentHash: chunk.ContentHash, StartLine: chunk.StartLine,
			EndLine: chunk.EndLine, EmbeddingModel: chunk.EmbeddingModel, Score: chunk.Score,
			Metadata: metadata,
		}
		if err := tx.QueryRow(ctx, insertItem, item.ContextSnapshotID, item.ProjectID,
			item.Ordinal, item.KnowledgeChunkID, item.ChunkKey, item.FilePath, item.PackageName,
			item.SymbolName, item.ChunkType, item.Content, item.ContentHash, item.StartLine,
			item.EndLine, item.EmbeddingModel, item.Score, item.Metadata).
			Scan(&item.ID, &item.CreatedAt); err != nil {
			return ContextSnapshot{}, fmt.Errorf("insert context snapshot item %d: %w", index+1, err)
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func validateOwnership(ctx context.Context, tx pgx.Tx, input RecordInput) error {
	var owned bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM analysis_jobs
		WHERE id=$1 AND project_id=$2 AND source_sha=$3 AND target_sha=$4)`,
		input.Analysis.ID, input.Analysis.ProjectID, input.Analysis.SourceSHA,
		input.Analysis.TargetSHA).Scan(&owned); err != nil {
		return fmt.Errorf("validate provenance analysis ownership: %w", err)
	}
	if !owned {
		return fmt.Errorf("%w: analysis identity does not match persisted source", ErrInvalidRecord)
	}
	queries := map[string]string{
		PhaseRecommendation: `SELECT EXISTS (SELECT 1 FROM changed_symbols AS symbols
			JOIN changed_files AS files ON files.id=symbols.changed_file_id
			WHERE symbols.id=$1 AND files.analysis_job_id=$2)`,
		PhaseGeneration: `SELECT EXISTS (SELECT 1 FROM test_recommendations
			WHERE id=$1 AND analysis_job_id=$2)`,
		PhaseRepair: `SELECT EXISTS (SELECT 1 FROM generated_tests
			WHERE id=$1 AND analysis_job_id=$2)`,
	}
	if err := tx.QueryRow(ctx, queries[input.Phase], input.SubjectID, input.Analysis.ID).Scan(&owned); err != nil {
		return fmt.Errorf("validate provenance subject ownership: %w", err)
	}
	if !owned {
		return fmt.Errorf("%w: %s subject %d does not belong to analysis %d",
			ErrInvalidRecord, input.Phase, input.SubjectID, input.Analysis.ID)
	}
	return nil
}

func validateRecordInput(input RecordInput) error {
	if input.Analysis.ID <= 0 || input.Analysis.ProjectID <= 0 || input.SubjectID <= 0 ||
		strings.TrimSpace(input.Analysis.SourceSHA) == "" || strings.TrimSpace(input.Analysis.TargetSHA) == "" ||
		strings.TrimSpace(input.PromptVersion) == "" || strings.TrimSpace(input.Request.Instructions) == "" ||
		strings.TrimSpace(input.Request.Input) == "" || strings.TrimSpace(input.Request.SchemaName) == "" ||
		len(input.Request.Schema) == 0 || input.Latency < 0 {
		return fmt.Errorf("%w: required call metadata is missing", ErrInvalidRecord)
	}
	if input.Phase != PhaseRecommendation && input.Phase != PhaseGeneration && input.Phase != PhaseRepair {
		return fmt.Errorf("%w: unsupported phase %q", ErrInvalidRecord, input.Phase)
	}
	if input.Status != StatusCompleted && input.Status != StatusFailed && input.Status != StatusInvalidOutput {
		return fmt.Errorf("%w: unsupported status %q", ErrInvalidRecord, input.Status)
	}
	for _, chunk := range input.Contexts {
		if chunk.ID <= 0 || chunk.ProjectID <= 0 || strings.TrimSpace(chunk.Content) == "" ||
			len(chunk.ContentHash) != 64 || chunk.StartLine <= 0 || chunk.EndLine < chunk.StartLine {
			return fmt.Errorf("%w: context chunk snapshot is incomplete", ErrInvalidRecord)
		}
	}
	return nil
}

func subjectColumns(phase string, subjectID int64) (*int64, *int64, *int64) {
	switch phase {
	case PhaseRecommendation:
		return &subjectID, nil, nil
	case PhaseGeneration:
		return nil, &subjectID, nil
	default:
		return nil, nil, &subjectID
	}
}

func HashText(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func hashJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return HashText(string(encoded)), nil
}

func estimateCost(inputTokens, outputTokens int, inputRate, outputRate float64) float64 {
	if inputTokens < 0 || outputTokens < 0 || inputRate < 0 || outputRate < 0 ||
		math.IsNaN(inputRate) || math.IsNaN(outputRate) ||
		math.IsInf(inputRate, 0) || math.IsInf(outputRate, 0) {
		return 0
	}
	return float64(inputTokens)/1_000_000*inputRate + float64(outputTokens)/1_000_000*outputRate
}

func truncateBytes(value string, limit int) string {
	value = strings.ToValidUTF8(value, "�")
	if len(value) <= limit {
		return value
	}
	for limit > 0 && !utf8.ValidString(value[:limit]) {
		limit--
	}
	return value[:limit]
}

func (r *Repository) List(ctx context.Context, analysisID int64) ([]Call, error) {
	const query = `SELECT id, analysis_job_id, project_id, phase, changed_symbol_id,
		recommendation_id, generated_test_id, attempt_number, source_sha, target_sha,
		provider, model_name, prompt_version, prompt_hash, configuration_hash,
		instructions, prompt_text, schema_name, request_schema, provider_response_id,
		response_text, status, error_message, input_tokens, output_tokens, total_tokens,
		latency_ms, estimated_cost_usd, created_at
		FROM llm_calls WHERE analysis_job_id=$1 ORDER BY created_at, id`
	rows, err := r.pool.Query(ctx, query, analysisID)
	if err != nil {
		return nil, fmt.Errorf("list LLM call provenance: %w", err)
	}
	defer rows.Close()
	results := make([]Call, 0)
	for rows.Next() {
		var item Call
		if err := rows.Scan(&item.ID, &item.AnalysisJobID, &item.ProjectID, &item.Phase,
			&item.ChangedSymbolID, &item.RecommendationID, &item.GeneratedTestID,
			&item.AttemptNumber, &item.SourceSHA, &item.TargetSHA, &item.Provider,
			&item.ModelName, &item.PromptVersion, &item.PromptHash, &item.ConfigurationHash,
			&item.Instructions, &item.PromptText, &item.SchemaName, &item.RequestSchema,
			&item.ProviderResponseID, &item.ResponseText, &item.Status, &item.ErrorMessage,
			&item.InputTokens, &item.OutputTokens, &item.TotalTokens, &item.LatencyMS,
			&item.EstimatedCostUSD, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan LLM call provenance: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate LLM call provenance: %w", err)
	}
	if err := r.loadSnapshots(ctx, results); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *Repository) ListSummary(ctx context.Context, analysisID int64) ([]CallSummary, error) {
	const query = `SELECT calls.id, calls.phase, calls.attempt_number, calls.source_sha,
		calls.target_sha, calls.provider, calls.model_name, calls.prompt_version,
		calls.prompt_hash, calls.configuration_hash, calls.provider_response_id,
		calls.status, calls.error_message, calls.input_tokens, calls.output_tokens,
		calls.total_tokens, calls.latency_ms, calls.estimated_cost_usd, calls.created_at,
		snapshots.id, snapshots.query_text, snapshots.index_ref,
		snapshots.index_generation, snapshots.embedding_model, COUNT(items.id)
		FROM llm_calls AS calls
		LEFT JOIN context_snapshots AS snapshots ON snapshots.llm_call_id=calls.id
		LEFT JOIN context_snapshot_items AS items ON items.context_snapshot_id=snapshots.id
		WHERE calls.analysis_job_id=$1
		GROUP BY calls.id, snapshots.id
		ORDER BY calls.created_at, calls.id`
	rows, err := r.pool.Query(ctx, query, analysisID)
	if err != nil {
		return nil, fmt.Errorf("list LLM provenance summaries: %w", err)
	}
	defer rows.Close()
	results := make([]CallSummary, 0)
	for rows.Next() {
		var item CallSummary
		var contextID *int64
		var queryText, indexRef, embeddingModel *string
		var indexGeneration *int64
		var itemCount int
		if err := rows.Scan(&item.ID, &item.Phase, &item.AttemptNumber, &item.SourceSHA,
			&item.TargetSHA, &item.Provider, &item.ModelName, &item.PromptVersion,
			&item.PromptHash, &item.ConfigurationHash, &item.ProviderResponseID,
			&item.Status, &item.ErrorMessage, &item.InputTokens, &item.OutputTokens,
			&item.TotalTokens, &item.LatencyMS, &item.EstimatedCostUSD, &item.CreatedAt,
			&contextID, &queryText, &indexRef, &indexGeneration, &embeddingModel,
			&itemCount); err != nil {
			return nil, fmt.Errorf("scan LLM provenance summary: %w", err)
		}
		if contextID != nil {
			item.Context = &ContextSummary{ID: *contextID, ItemCount: itemCount}
			if queryText != nil {
				item.Context.QueryText = *queryText
			}
			if indexRef != nil {
				item.Context.IndexRef = *indexRef
			}
			if indexGeneration != nil {
				item.Context.IndexGeneration = *indexGeneration
			}
			if embeddingModel != nil {
				item.Context.EmbeddingModel = *embeddingModel
			}
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate LLM provenance summaries: %w", err)
	}
	return results, nil
}

func (r *Repository) loadSnapshots(ctx context.Context, calls []Call) error {
	if len(calls) == 0 {
		return nil
	}
	callIDs := make([]int64, len(calls))
	callIndex := make(map[int64]int, len(calls))
	for index := range calls {
		callIDs[index] = calls[index].ID
		callIndex[calls[index].ID] = index
	}
	const snapshotsQuery = `SELECT id, llm_call_id, project_id, query_text,
		retrieval_query, retrieval_config, index_ref, index_generation, embedding_model,
		created_at FROM context_snapshots WHERE llm_call_id=ANY($1::bigint[])
		ORDER BY llm_call_id`
	rows, err := r.pool.Query(ctx, snapshotsQuery, callIDs)
	if err != nil {
		return fmt.Errorf("list provenance context snapshots: %w", err)
	}
	snapshotToCall := make(map[int64]int, len(calls))
	for rows.Next() {
		var snapshot ContextSnapshot
		if err := rows.Scan(&snapshot.ID, &snapshot.LLMCallID, &snapshot.ProjectID,
			&snapshot.QueryText, &snapshot.RetrievalQuery, &snapshot.RetrievalConfig,
			&snapshot.IndexRef, &snapshot.IndexGeneration, &snapshot.EmbeddingModel,
			&snapshot.CreatedAt); err != nil {
			rows.Close()
			return fmt.Errorf("scan provenance context snapshot: %w", err)
		}
		snapshot.Items = make([]ContextSnapshotItem, 0)
		index, exists := callIndex[snapshot.LLMCallID]
		if !exists {
			rows.Close()
			return fmt.Errorf("context snapshot references an unknown LLM call")
		}
		calls[index].Context = &snapshot
		snapshotToCall[snapshot.ID] = index
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate provenance context snapshots: %w", err)
	}
	rows.Close()
	if len(snapshotToCall) == 0 {
		return nil
	}
	snapshotIDs := make([]int64, 0, len(snapshotToCall))
	for id := range snapshotToCall {
		snapshotIDs = append(snapshotIDs, id)
	}
	const itemsQuery = `SELECT id, context_snapshot_id, project_id, ordinal,
		knowledge_chunk_id, chunk_key, file_path, package_name, symbol_name, chunk_type,
		content, content_hash, start_line, end_line, embedding_model, score, metadata,
		created_at FROM context_snapshot_items
		WHERE context_snapshot_id=ANY($1::bigint[])
		ORDER BY context_snapshot_id, ordinal`
	itemRows, err := r.pool.Query(ctx, itemsQuery, snapshotIDs)
	if err != nil {
		return fmt.Errorf("list provenance context snapshot items: %w", err)
	}
	defer itemRows.Close()
	for itemRows.Next() {
		var item ContextSnapshotItem
		if err := itemRows.Scan(&item.ID, &item.ContextSnapshotID, &item.ProjectID,
			&item.Ordinal, &item.KnowledgeChunkID, &item.ChunkKey, &item.FilePath,
			&item.PackageName, &item.SymbolName, &item.ChunkType, &item.Content,
			&item.ContentHash, &item.StartLine, &item.EndLine, &item.EmbeddingModel,
			&item.Score, &item.Metadata, &item.CreatedAt); err != nil {
			return fmt.Errorf("scan provenance context snapshot item: %w", err)
		}
		callPosition, exists := snapshotToCall[item.ContextSnapshotID]
		if !exists || calls[callPosition].Context == nil {
			return fmt.Errorf("context item references an unknown snapshot")
		}
		calls[callPosition].Context.Items = append(calls[callPosition].Context.Items, item)
	}
	if err := itemRows.Err(); err != nil {
		return fmt.Errorf("iterate provenance context snapshot items: %w", err)
	}
	return nil
}
