package requirement

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type SourceChange struct {
	Classification      string  `json:"classification"`
	BeforeIDs           []int64 `json:"before_ids"`
	AfterIDs            []int64 `json:"after_ids"`
	Reason              string  `json:"reason"`
	AffectedTestCaseIDs []int64 `json:"affected_test_case_ids"`
}
type SourceComparison struct {
	ID              int64          `json:"id"`
	FromSnapshotID  *int64         `json:"from_snapshot_id"`
	ToSnapshotID    int64          `json:"to_snapshot_id"`
	IndexGeneration int64          `json:"index_generation"`
	Items           []SourceChange `json:"items"`
	CreatedAt       time.Time      `json:"created_at"`
}
type comparisonRequirement struct {
	ID                int64
	Identity, Content string
}

func compareRequirements(before, after []comparisonRequirement) []SourceChange {
	groups := map[string][2][]comparisonRequirement{}
	for _, row := range before {
		g := groups[row.Identity]
		g[0] = append(g[0], row)
		groups[row.Identity] = g
	}
	for _, row := range after {
		g := groups[row.Identity]
		g[1] = append(g[1], row)
		groups[row.Identity] = g
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := []SourceChange{}
	add := func(kind, reason string, old, new []comparisonRequirement) {
		item := SourceChange{Classification: kind, Reason: reason, BeforeIDs: []int64{}, AfterIDs: []int64{}, AffectedTestCaseIDs: []int64{}}
		for _, v := range old {
			item.BeforeIDs = append(item.BeforeIDs, v.ID)
		}
		for _, v := range new {
			item.AfterIDs = append(item.AfterIDs, v.ID)
		}
		out = append(out, item)
	}
	for _, key := range keys {
		g := groups[key]
		old, new := g[0], g[1]
		if len(before) > 0 && strings.Contains(key, "UNKNOWN:") {
			add("AMBIGUOUS", "Thiếu định danh nghiệp vụ ổn định; không suy đoán ánh xạ từ nội dung tương tự.", old, new)
			continue
		}
		// Only exact, unique content matches can disambiguate repeated identifiers.
		usedOld, usedNew := map[int]bool{}, map[int]bool{}
		for i, left := range old {
			matches := []int{}
			oldCount := 0
			for _, v := range old {
				if v.Content == left.Content {
					oldCount++
				}
			}
			for j, right := range new {
				if left.Content == right.Content {
					matches = append(matches, j)
				}
			}
			if oldCount == 1 && len(matches) == 1 {
				j := matches[0]
				add("UNCHANGED", "Cùng định danh, tài liệu logic, luồng và nội dung; nguồn mới vẫn cần duyệt riêng.", []comparisonRequirement{left}, []comparisonRequirement{new[j]})
				usedOld[i] = true
				usedNew[j] = true
			}
		}
		left, right := []comparisonRequirement{}, []comparisonRequirement{}
		for i, v := range old {
			if !usedOld[i] {
				left = append(left, v)
			}
		}
		for i, v := range new {
			if !usedNew[i] {
				right = append(right, v)
			}
		}
		switch {
		case len(left) == 0 && len(right) == 0:
		case len(left) == 0:
			for _, v := range right {
				add("ADDED", "Không có ứng viên tương ứng trong phạm vi nguồn trước.", nil, []comparisonRequirement{v})
			}
		case len(right) == 0:
			for _, v := range left {
				add("REMOVED", "Không còn trong kết quả trích xuất của phạm vi mới; lịch sử vẫn được giữ.", []comparisonRequirement{v}, nil)
			}
		case len(left) == 1 && len(right) == 1:
			add("CHANGED", "Khớp định danh + tài liệu logic + luồng; nội dung nghiệp vụ đã thay đổi.", left, right)
		default:
			add("AMBIGUOUS", "Nhiều ứng viên cùng định danh/nguồn/luồng; không tự ghép hoặc chuyển quyết định duyệt.", left, right)
		}
	}
	return out
}

// Reconcile only after a complete extraction. A late old-generation worker may
// retain its output but cannot replace the current source inventory.
func (r *Repository) ReconcileSources(ctx context.Context, setID, snapshotID, generation int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var current bool
	if _, err = tx.Exec(ctx, `SELECT id FROM document_sets WHERE id=$1 FOR UPDATE`, setID); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT i.generation=$2 AND i.indexed_source_revision=s.source_revision FROM document_index_status i JOIN document_sets s ON s.id=i.document_set_id WHERE i.document_set_id=$1 FOR SHARE OF i`, setID, generation).Scan(&current); err != nil {
		return err
	}
	if !current {
		return nil
	}
	var previous *int64
	if err = tx.QueryRow(ctx, `SELECT (SELECT source_snapshot_id FROM requirements WHERE document_set_id=$1 AND source_snapshot_id<>$2 AND source_state='CURRENT' ORDER BY id DESC LIMIT 1)`, setID, snapshotID).Scan(&previous); err != nil {
		return err
	}
	if previous == nil {
		if err = tx.QueryRow(ctx, `SELECT (SELECT from_snapshot_id FROM requirement_source_comparisons WHERE document_set_id=$1 AND to_snapshot_id=$2)`, setID, snapshotID).Scan(&previous); err != nil {
			return err
		}
	}
	load := func(snapshot *int64) ([]comparisonRequirement, error) {
		rows, err := tx.Query(ctx, `SELECT r.id,
     jsonb_build_array(CASE WHEN stable_identifier='' THEN 'UNKNOWN:'||r.id ELSE stable_identifier END,flow_type,
       COALESCE((SELECT array_agg(DISTINCT v.document_id ORDER BY v.document_id) FROM requirement_evidence e JOIN document_versions v ON v.id=e.document_version_id WHERE e.requirement_id=r.id),'{}'))::text,
     jsonb_build_object('title',r.title,'statement',r.statement,'type',r.requirement_type,'actor',r.actor,'precondition',r.precondition,'postcondition',r.postcondition,'priority',r.priority,'risk',r.risk,'assumptions',r.assumptions,
       'steps',COALESCE((SELECT jsonb_agg(jsonb_build_array(ordinal,action,expected_result) ORDER BY ordinal) FROM requirement_flow_steps WHERE requirement_id=r.id),'[]'))::text
     FROM requirements r WHERE document_set_id=$1 AND source_snapshot_id=$2 AND NOT EXISTS(SELECT 1 FROM requirements n WHERE n.supersedes_requirement_id=r.id) ORDER BY r.id`, setID, snapshot)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []comparisonRequirement{}
		for rows.Next() {
			var row comparisonRequirement
			if err = rows.Scan(&row.ID, &row.Identity, &row.Content); err != nil {
				return nil, err
			}
			out = append(out, row)
		}
		return out, rows.Err()
	}
	before, err := load(previous)
	if err != nil {
		return err
	}
	after, err := load(&snapshotID)
	if err != nil {
		return err
	}
	changes := compareRequirements(before, after)
	for i := range changes {
		if err = tx.QueryRow(ctx, `SELECT COALESCE(array_agg(DISTINCT test_case_id ORDER BY test_case_id),'{}') FROM test_case_requirement_links WHERE requirement_id=ANY($1)`, changes[i].BeforeIDs).Scan(&changes[i].AffectedTestCaseIDs); err != nil {
			return err
		}
	}
	payload, _ := json.Marshal(changes)
	if _, err = tx.Exec(ctx, `INSERT INTO requirement_source_comparisons(document_set_id,from_snapshot_id,to_snapshot_id,index_generation,items) VALUES($1,$2,$3,$4,$5)
 ON CONFLICT(document_set_id,to_snapshot_id) DO UPDATE SET items=EXCLUDED.items`, setID, previous, snapshotID, generation, payload); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE requirements SET source_state='HISTORICAL',updated_at=NOW() WHERE document_set_id=$1 AND source_snapshot_id IS DISTINCT FROM $2 AND source_state='CURRENT'`, setID, snapshotID); err != nil {
		return err
	}
	for _, item := range changes {
		if item.Classification == "REMOVED" {
			if _, err = tx.Exec(ctx, `UPDATE requirements SET source_state='REMOVED' WHERE id=ANY($1)`, item.BeforeIDs); err != nil {
				return err
			}
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE requirements SET source_state='CURRENT',updated_at=NOW() WHERE document_set_id=$1 AND source_snapshot_id=$2 AND source_state<>'CURRENT'`, setID, snapshotID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) SourceComparisons(ctx context.Context, setID int64) ([]SourceComparison, error) {
	if setID <= 0 {
		return nil, ErrInvalidInput
	}
	rows, err := s.repository.pool.Query(ctx, `SELECT id,from_snapshot_id,to_snapshot_id,index_generation,items,created_at FROM requirement_source_comparisons WHERE document_set_id=$1 ORDER BY index_generation DESC,id DESC LIMIT 10`, setID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SourceComparison{}
	for rows.Next() {
		var item SourceComparison
		var raw []byte
		if err = rows.Scan(&item.ID, &item.FromSnapshotID, &item.ToSnapshotID, &item.IndexGeneration, &raw, &item.CreatedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &item.Items); err != nil {
			return nil, fmt.Errorf("decode source comparison: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
