package testcase

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

type ReleaseRef struct {
	ID     int64 `json:"id"`
	Number int   `json:"number"`
}
type HistoryEntry struct {
	Revision TestCase     `json:"revision"`
	Releases []ReleaseRef `json:"releases"`
}
type HistoryPage struct {
	Entries    []HistoryEntry `json:"entries"`
	NextBefore int            `json:"next_before,omitempty"`
}

func (s *Service) History(ctx context.Context, familyID int64, before, limit int) (HistoryPage, error) {
	out := HistoryPage{Entries: []HistoryEntry{}}
	if familyID <= 0 || before < 0 || limit < 1 || limit > 50 {
		return out, ErrInvalidInput
	}
	if _, err := s.repository.GetFamily(ctx, familyID); err != nil {
		return out, err
	}
	rows, err := s.repository.pool.Query(ctx, `SELECT `+testCaseColumnList+` FROM test_cases WHERE family_id=$1 AND ($2=0 OR version_number<$2) ORDER BY version_number DESC,id DESC LIMIT $3`, familyID, before, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var item TestCase
		if err = rows.Scan(testCaseDestinations(&item)...); err != nil {
			rows.Close()
			return out, err
		}
		out.Entries = append(out.Entries, HistoryEntry{Revision: item, Releases: []ReleaseRef{}})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Entries) > limit {
		out.Entries = out.Entries[:limit]
		out.NextBefore = out.Entries[limit-1].Revision.VersionNumber
	}
	for i := range out.Entries {
		refs, e := s.repository.pool.Query(ctx, `SELECT r.id,r.release_number FROM test_suite_releases r JOIN test_suite_release_items i ON i.release_id=r.id WHERE i.test_case_id=$1 ORDER BY r.id DESC LIMIT 50`, out.Entries[i].Revision.ID)
		if e != nil {
			return out, e
		}
		for refs.Next() {
			var ref ReleaseRef
			if e = refs.Scan(&ref.ID, &ref.Number); e != nil {
				refs.Close()
				return out, e
			}
			out.Entries[i].Releases = append(out.Entries[i].Releases, ref)
		}
		e = refs.Err()
		refs.Close()
		if e != nil {
			return out, e
		}
	}
	return out, nil
}

type DiffPage struct {
	RevisionDiff
	Total      int  `json:"total"`
	NextOffset *int `json:"next_offset,omitempty"`
}

func (s *Service) Comparison(ctx context.Context, family, from, to int64, offset, limit int) (DiffPage, error) {
	if offset < 0 || limit < 1 || limit > 50 {
		return DiffPage{}, ErrInvalidInput
	}
	diff, err := s.repository.Diff(ctx, family, from, to)
	if err != nil {
		return DiffPage{}, err
	}
	flattened := []FieldDiff{}
	for _, change := range diff.Changes {
		left, right := reflect.ValueOf(change.Before), reflect.ValueOf(change.After)
		if left.Kind() == reflect.Slice && right.Kind() == reflect.Slice {
			for i := 0; i < max(left.Len(), right.Len()); i++ {
				var a, b any
				if i < left.Len() {
					a = left.Index(i).Interface()
				}
				if i < right.Len() {
					b = right.Index(i).Interface()
				}
				if !reflect.DeepEqual(a, b) {
					flattened = append(flattened, FieldDiff{Field: fmt.Sprintf("%s[%d]", change.Field, i+1), Before: a, After: b})
				}
			}
		} else {
			flattened = append(flattened, change)
		}
	}
	out := DiffPage{RevisionDiff: diff, Total: len(flattened)}
	start := min(offset, len(flattened))
	end := min(start+limit, len(flattened))
	out.Changes = flattened[start:end]
	if end < len(flattened) {
		out.NextOffset = &end
	}
	return out, nil
}

type RevisionRun struct {
	ID         int64     `json:"id"`
	TestCaseID int64     `json:"test_case_id"`
	Version    int       `json:"version"`
	RunID      int64     `json:"run_id"`
	ArtifactID *int64    `json:"artifact_id,omitempty"`
	Status     string    `json:"status"`
	Expected   string    `json:"expected"`
	Actual     string    `json:"actual"`
	CreatedAt  time.Time `json:"created_at"`
}
type RunPage struct {
	Runs       []RevisionRun `json:"runs"`
	NextBefore int64         `json:"next_before,omitempty"`
}

func (s *Service) RevisionRuns(ctx context.Context, id int64, familyScope bool, before int64) (RunPage, error) {
	out := RunPage{Runs: []RevisionRun{}}
	if id <= 0 || before < 0 {
		return out, ErrInvalidInput
	}
	detail, err := s.Get(ctx, id)
	if err != nil {
		return out, err
	}
	rows, err := s.repository.pool.Query(ctx, `SELECT i.id,i.test_case_id,t.version_number,i.test_run_id,i.automation_artifact_id,i.status,i.expected_result_snapshot,i.actual_result,i.created_at FROM test_run_items i JOIN test_cases t ON t.id=i.test_case_id WHERE (($2 AND t.family_id=$3) OR (NOT $2 AND t.id=$1)) AND ($4=0 OR i.id<$4) ORDER BY i.id DESC LIMIT 21`, id, familyScope, detail.TestCase.FamilyID, before)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v RevisionRun
		if err = rows.Scan(&v.ID, &v.TestCaseID, &v.Version, &v.RunID, &v.ArtifactID, &v.Status, &v.Expected, &v.Actual, &v.CreatedAt); err != nil {
			return out, err
		}
		out.Runs = append(out.Runs, v)
	}
	if len(out.Runs) > 20 {
		out.Runs = out.Runs[:20]
		out.NextBefore = out.Runs[19].ID
	}
	return out, rows.Err()
}

type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }
