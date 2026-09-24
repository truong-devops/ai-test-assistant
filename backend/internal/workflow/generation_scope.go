package workflow

import (
	"context"
	"encoding/json"
	"slices"
)

type GenerationScope struct {
	DefaultScope           string          `json:"default_scope"`
	AffectedRequirementIDs []int64         `json:"affected_requirement_ids"`
	AffectedFamilyIDs      []int64         `json:"affected_family_ids"`
	RemovedFamilyIDs       []int64         `json:"removed_family_ids"`
	BlockedRequirements    int             `json:"blocked_requirements"`
	Changes                json.RawMessage `json:"changes"`
}

func (r *Repository) GenerationScope(ctx context.Context, setID int64) (GenerationScope, error) {
	out := GenerationScope{DefaultScope: "ALL", AffectedRequirementIDs: []int64{}, AffectedFamilyIDs: []int64{}, RemovedFamilyIDs: []int64{}, Changes: json.RawMessage(`[]`)}
	var families int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM test_case_families WHERE document_set_id=$1 AND NOT archived`, setID).Scan(&families); err != nil {
		return out, err
	}
	if families > 0 {
		out.DefaultScope = "AFFECTED"
	}
	rows, err := r.pool.Query(ctx, `SELECT r.id FROM requirements r WHERE r.document_set_id=$1 AND r.status='APPROVED' AND cardinality(requirement_review_blockers(r.id))=0 AND requirement_is_current(r.id)
 AND (NOT EXISTS(SELECT 1 FROM test_case_families f JOIN test_case_requirement_links l ON l.test_case_id=f.head_revision_id WHERE f.document_set_id=$1 AND NOT f.archived AND l.requirement_id=r.id)
 OR EXISTS(SELECT 1 FROM test_case_families f JOIN test_case_requirement_links l ON l.test_case_id=f.head_revision_id WHERE f.document_set_id=$1 AND NOT f.archived AND l.requirement_id=r.id AND EXISTS(SELECT 1 FROM test_case_requirement_links old WHERE old.test_case_id=f.head_revision_id AND NOT requirement_is_current(old.requirement_id)))) ORDER BY r.id`, setID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		out.AffectedRequirementIDs = append(out.AffectedRequirementIDs, id)
	}
	rows.Close()
	if rows.Err() != nil {
		return out, rows.Err()
	}
	rows, err = r.pool.Query(ctx, `SELECT f.id,NOT EXISTS(SELECT 1 FROM test_case_requirement_links l JOIN requirements r ON r.id=l.requirement_id WHERE l.test_case_id=f.head_revision_id AND r.source_state<>'REMOVED') FROM test_case_families f
 WHERE f.document_set_id=$1 AND NOT f.archived AND EXISTS(SELECT 1 FROM test_case_requirement_links l WHERE l.test_case_id=f.head_revision_id AND NOT requirement_is_current(l.requirement_id)) ORDER BY f.id`, setID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id int64
		var removed bool
		if err = rows.Scan(&id, &removed); err != nil {
			rows.Close()
			return out, err
		}
		out.AffectedFamilyIDs = append(out.AffectedFamilyIDs, id)
		if removed {
			out.RemovedFamilyIDs = append(out.RemovedFamilyIDs, id)
		}
	}
	rows.Close()
	if rows.Err() != nil {
		return out, rows.Err()
	}
	err = r.pool.QueryRow(ctx, `SELECT count(*) FROM requirements WHERE document_set_id=$1 AND requirement_is_current(id) AND (status<>'APPROVED' OR cardinality(requirement_review_blockers(id))>0)`, setID).Scan(&out.BlockedRequirements)
	if err != nil {
		return out, err
	}
	err = r.pool.QueryRow(ctx, `SELECT COALESCE((SELECT c.items FROM requirement_source_comparisons c JOIN document_source_snapshots s ON s.id=c.to_snapshot_id JOIN document_sets d ON d.id=c.document_set_id WHERE d.id=$1 AND s.source_revision=d.source_revision ORDER BY c.id DESC LIMIT 1),'[]'::jsonb)`, setID).Scan(&out.Changes)
	return out, err
}

func (r *Repository) buildProposalSnapshot(ctx context.Context, setID int64, input OperationInput) (json.RawMessage, string, error) {
	preview, err := r.GenerationScope(ctx, setID)
	if err != nil {
		return nil, "", err
	}
	scope := input.GenerationScope
	if scope == "" {
		if len(input.RequirementIDs) > 0 {
			scope = "SELECTED"
		} else {
			scope = preview.DefaultScope
		}
	}
	if !slices.Contains([]string{"ALL", "SELECTED", "AFFECTED"}, scope) || (scope == "SELECTED" && len(input.RequirementIDs) == 0) || (scope != "SELECTED" && len(input.RequirementIDs) > 0) {
		return nil, "", ErrInvalidInput
	}
	raw, _, err := r.buildGenerateSnapshot(ctx, setID, input.RequirementIDs, true)
	if err != nil {
		return nil, "", err
	}
	var snapshot generateInputSnapshot
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		return nil, "", err
	}
	if scope == "AFFECTED" {
		filtered := []generateRequirementSnapshot{}
		for _, req := range snapshot.Requirements {
			if slices.Contains(preview.AffectedRequirementIDs, req.ID) {
				filtered = append(filtered, req)
			}
		}
		snapshot.Requirements = filtered
	}
	if len(snapshot.Requirements) == 0 && (scope == "SELECTED" || len(preview.RemovedFamilyIDs) == 0) {
		return nil, "", &BlockedError{Code: "NO_GENERATION_CHANGES", Message: "No eligible requirements or confirmed removed identities in this scope", NextAction: "REVIEW_REQUIREMENTS"}
	}
	snapshot.Scope, snapshot.RequestedScope, snapshot.PerRequirementUnits = scope, input.GenerationScope, true
	raw, _, err = hashJSON(snapshot)
	if err != nil {
		return nil, "", err
	}
	return r.PinProposalTargets(ctx, setID, raw)
}
