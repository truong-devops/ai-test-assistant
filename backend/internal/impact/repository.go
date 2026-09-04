package impact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/job"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) SaveAnalysis(ctx context.Context, analysisID, projectID int64,
	expectedAttempt int, symbols []job.ChangedSymbol, graph Result,
) error {
	if err := validateResult(graph); err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save impact analysis: %w", err)
	}
	defer tx.Rollback(ctx)
	var owned bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM analysis_jobs
		WHERE id=$1 AND project_id=$2 AND source_sha=$3 AND status=$4 AND attempt_count=$5)`,
		analysisID, projectID, graph.SourceSHA, job.StatusAnalyzingChange, expectedAttempt).Scan(&owned); err != nil {
		return fmt.Errorf("validate impact analysis ownership: %w", err)
	}
	if !owned {
		return job.ErrLeaseLost
	}
	if _, err := tx.Exec(ctx, `DELETE FROM impact_analysis_runs WHERE analysis_job_id=$1`, analysisID); err != nil {
		return fmt.Errorf("clear previous impact analysis: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM changed_symbols WHERE changed_file_id IN
		(SELECT id FROM changed_files WHERE analysis_job_id=$1)`, analysisID); err != nil {
		return fmt.Errorf("clear changed symbols: %w", err)
	}
	const insertSymbol = `INSERT INTO changed_symbols
		(changed_file_id, symbol_name, symbol_kind, receiver_name, package_name,
		 start_line, end_line, change_type, change_summary)
		SELECT id,$3,$4,$5,$6,$7,$8,$9,$10 FROM changed_files
		WHERE id=$1 AND analysis_job_id=$2`
	for _, symbol := range symbols {
		result, err := tx.Exec(ctx, insertSymbol, symbol.ChangedFileID, analysisID,
			symbol.SymbolName, symbol.SymbolKind, symbol.ReceiverName, symbol.PackageName,
			symbol.StartLine, symbol.EndLine, symbol.ChangeType, symbol.ChangeSummary)
		if err != nil {
			return fmt.Errorf("insert changed symbol %q: %w", symbol.SymbolName, err)
		}
		if result.RowsAffected() != 1 {
			return fmt.Errorf("changed file %d does not belong to analysis %d", symbol.ChangedFileID, analysisID)
		}
	}
	var runID int64
	if err := tx.QueryRow(ctx, `INSERT INTO impact_analysis_runs
		(analysis_job_id, project_id, source_sha, mode, algorithm, max_depth,
		 max_nodes, package_count, fallback_reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, analysisID, projectID,
		graph.SourceSHA, graph.Mode, graph.Algorithm, graph.MaxDepth, graph.MaxNodes,
		graph.PackageCount, graph.FallbackReason).Scan(&runID); err != nil {
		return fmt.Errorf("insert impact analysis run: %w", err)
	}
	nodeIDs := make(map[string]int64, len(graph.Nodes))
	const insertNode = `INSERT INTO impact_nodes
		(impact_run_id, project_id, stable_key, package_path, package_name, symbol_name,
		 receiver_name, symbol_kind, file_path, start_line, end_line, direct_change,
		 existing_test, depth, score, reason_codes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		RETURNING id`
	for _, node := range graph.Nodes {
		reasons, err := json.Marshal(node.ReasonCodes)
		if err != nil {
			return fmt.Errorf("encode impact reasons for %q: %w", node.Key, err)
		}
		var id int64
		if err := tx.QueryRow(ctx, insertNode, runID, projectID, node.Key,
			node.PackagePath, node.PackageName, node.SymbolName, node.ReceiverName,
			node.SymbolKind, node.FilePath, node.StartLine, node.EndLine,
			node.DirectChange, node.ExistingTest, node.Depth, node.Score, reasons).Scan(&id); err != nil {
			return fmt.Errorf("insert impact node %q: %w", node.Key, err)
		}
		nodeIDs[node.Key] = id
	}
	const insertEdge = `INSERT INTO impact_edges
		(impact_run_id, from_node_id, to_node_id, relation, reason_code, depth, score)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`
	for _, edge := range graph.Edges {
		fromID, fromOK := nodeIDs[edge.FromKey]
		toID, toOK := nodeIDs[edge.ToKey]
		if !fromOK || !toOK {
			return fmt.Errorf("impact edge references an unknown node")
		}
		if _, err := tx.Exec(ctx, insertEdge, runID, fromID, toID, edge.Relation,
			edge.ReasonCode, edge.Depth, edge.Score); err != nil {
			return fmt.Errorf("insert impact edge: %w", err)
		}
	}
	result, err := tx.Exec(ctx, `UPDATE analysis_jobs SET status=$2, error_message=NULL,
		lease_expires_at=NULL, next_attempt_at=NOW(), attempt_count=0
		WHERE id=$1 AND status=$3 AND attempt_count=$4`, analysisID,
		job.StatusRetrievingContext, job.StatusAnalyzingChange, expectedAttempt)
	if err != nil {
		return fmt.Errorf("complete impact analysis: %w", err)
	}
	if result.RowsAffected() != 1 {
		return job.ErrLeaseLost
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit impact analysis: %w", err)
	}
	return nil
}

func validateResult(result Result) error {
	if result.SourceSHA == "" || (result.Mode != ModeSSA && result.Mode != ModeASTFallback) ||
		result.Algorithm == "" || result.MaxDepth < 1 || result.MaxDepth > 20 ||
		result.MaxNodes < 1 || result.MaxNodes > 10_000 || result.PackageCount < 0 {
		return fmt.Errorf("invalid impact analysis result")
	}
	seen := make(map[string]struct{}, len(result.Nodes))
	for _, node := range result.Nodes {
		if node.Key == "" || node.SymbolName == "" || node.Depth < 0 ||
			node.Score < 0 || node.Score > 1 || len(node.ReasonCodes) == 0 {
			return fmt.Errorf("invalid impact node %q", node.Key)
		}
		if _, duplicate := seen[node.Key]; duplicate {
			return fmt.Errorf("duplicate impact node %q", node.Key)
		}
		seen[node.Key] = struct{}{}
	}
	for _, edge := range result.Edges {
		_, fromOK := seen[edge.FromKey]
		_, toOK := seen[edge.ToKey]
		validRelation := edge.Relation == RelationCalls || edge.Relation == RelationImplements || edge.Relation == RelationUsesType
		validReason := edge.ReasonCode == ReasonCaller || edge.ReasonCode == ReasonCallee ||
			edge.ReasonCode == ReasonInterfaceImplementation || edge.ReasonCode == ReasonTypeUsage ||
			edge.ReasonCode == ReasonExistingTest
		if !fromOK || !toOK || !validRelation || !validReason || edge.Depth < 1 ||
			edge.Depth > result.MaxDepth || edge.Score < 0 || edge.Score > 1 {
			return fmt.Errorf("invalid impact edge from %q to %q", edge.FromKey, edge.ToKey)
		}
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, analysisID int64) (Bundle, error) {
	const runQuery = `SELECT id, analysis_job_id, project_id, source_sha, mode,
		algorithm, max_depth, max_nodes, package_count, fallback_reason, created_at
		FROM impact_analysis_runs WHERE analysis_job_id=$1`
	bundle := Bundle{
		Nodes: make([]Node, 0),
		Edges: make([]Edge, 0),
	}
	if err := r.pool.QueryRow(ctx, runQuery, analysisID).Scan(&bundle.Run.ID,
		&bundle.Run.AnalysisJobID, &bundle.Run.ProjectID, &bundle.Run.SourceSHA,
		&bundle.Run.Mode, &bundle.Run.Algorithm, &bundle.Run.MaxDepth,
		&bundle.Run.MaxNodes, &bundle.Run.PackageCount, &bundle.Run.FallbackReason,
		&bundle.Run.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Bundle{}, ErrNotFound
		}
		return Bundle{}, fmt.Errorf("get impact analysis: %w", err)
	}
	rows, err := r.pool.Query(ctx, `SELECT id, stable_key, package_path, package_name,
		symbol_name, receiver_name, symbol_kind, file_path, start_line, end_line,
		direct_change, existing_test, depth, score, reason_codes
		FROM impact_nodes WHERE impact_run_id=$1
		ORDER BY direct_change DESC, score DESC, depth, stable_key`, bundle.Run.ID)
	if err != nil {
		return Bundle{}, fmt.Errorf("list impact nodes: %w", err)
	}
	for rows.Next() {
		var node Node
		var reasons []byte
		if err := rows.Scan(&node.ID, &node.Key, &node.PackagePath, &node.PackageName,
			&node.SymbolName, &node.ReceiverName, &node.SymbolKind, &node.FilePath,
			&node.StartLine, &node.EndLine, &node.DirectChange, &node.ExistingTest,
			&node.Depth, &node.Score, &reasons); err != nil {
			rows.Close()
			return Bundle{}, fmt.Errorf("scan impact node: %w", err)
		}
		if err := json.Unmarshal(reasons, &node.ReasonCodes); err != nil {
			rows.Close()
			return Bundle{}, fmt.Errorf("decode impact reasons: %w", err)
		}
		bundle.Nodes = append(bundle.Nodes, node)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Bundle{}, fmt.Errorf("iterate impact nodes: %w", err)
	}
	rows.Close()
	edgeRows, err := r.pool.Query(ctx, `SELECT id, from_node_id, to_node_id,
		relation, reason_code, depth, score FROM impact_edges
		WHERE impact_run_id=$1 ORDER BY depth, id`, bundle.Run.ID)
	if err != nil {
		return Bundle{}, fmt.Errorf("list impact edges: %w", err)
	}
	defer edgeRows.Close()
	for edgeRows.Next() {
		var edge Edge
		if err := edgeRows.Scan(&edge.ID, &edge.FromNodeID, &edge.ToNodeID,
			&edge.Relation, &edge.ReasonCode, &edge.Depth, &edge.Score); err != nil {
			return Bundle{}, fmt.Errorf("scan impact edge: %w", err)
		}
		bundle.Edges = append(bundle.Edges, edge)
	}
	if err := edgeRows.Err(); err != nil {
		return Bundle{}, fmt.Errorf("iterate impact edges: %w", err)
	}
	return bundle, nil
}
