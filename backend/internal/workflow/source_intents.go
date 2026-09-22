package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type SourceSelection struct {
	VersionID      int64  `json:"version_id"`
	SHA256         string `json:"sha256"`
	ApprovalStatus string `json:"approval_status"`
}
type SourceCommand struct {
	Command                string            `json:"command"`
	ExpectedSourceRevision int64             `json:"expected_source_revision"`
	Selected               []SourceSelection `json:"selected"`
	ExcludedVersionIDs     []int64           `json:"excluded_version_ids"`
	ReviewerName           string            `json:"reviewer_name"`
}
type SourceIntent struct {
	ID              int64           `json:"id"`
	DocumentSetID   int64           `json:"document_set_id"`
	SourceRevision  int64           `json:"source_revision"`
	Command         string          `json:"command"`
	Status          string          `json:"status"`
	Request         json.RawMessage `json:"request"`
	IndexJobID      *int64          `json:"index_job_id,omitempty"`
	ExtractionJobID *int64          `json:"extraction_job_id,omitempty"`
	ErrorMessage    string          `json:"error_message,omitempty"`
}

// SourceCommand records exact source approvals and the continuation in one transaction.
// The display name is audit context; only the trusted role/actor authorizes the command.
func (s *Service) SourceCommand(ctx context.Context, setID int64, input SourceCommand, key, role, actor string) (SourceIntent, error) {
	input.Command = strings.ToUpper(strings.TrimSpace(input.Command))
	if setID <= 0 || len(key) == 0 || len(key) > 200 || len(input.Selected) > 100 || len(input.ExcludedVersionIDs) > 100 || len(input.ReviewerName) > 160 {
		return SourceIntent{}, ErrInvalidInput
	}
	required := "editor"
	if input.Command == "APPROVE" || input.Command == "APPROVE_AND_EXTRACT" {
		required = "reviewer"
	} else if input.Command != "INDEX" && input.Command != "EXTRACT" {
		return SourceIntent{}, ErrInvalidInput
	}
	if !roleAtLeast(role, required) {
		return SourceIntent{}, ErrForbidden
	}
	if required == "reviewer" && (len(input.Selected) == 0 || strings.TrimSpace(input.ReviewerName) == "") {
		return SourceIntent{}, ErrInvalidInput
	}
	if required == "editor" && len(input.Selected) > 0 {
		return SourceIntent{}, ErrInvalidInput
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return SourceIntent{}, err
	}
	tx, err := s.repository.pool.Begin(ctx)
	if err != nil {
		return SourceIntent{}, err
	}
	defer tx.Rollback(ctx)
	var revision int64
	var status string
	if err = tx.QueryRow(ctx, `SELECT source_revision,status FROM document_sets WHERE id=$1 FOR UPDATE`, setID).Scan(&revision, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SourceIntent{}, ErrNotFound
		}
		return SourceIntent{}, err
	}
	var existingID int64
	var same bool
	err = tx.QueryRow(ctx, `SELECT id,request=$3::jsonb FROM document_source_intents WHERE document_set_id=$1 AND idempotency_key=$2`, setID, key, payload).Scan(&existingID, &same)
	if err == nil {
		if !same {
			return SourceIntent{}, ErrIdempotencyConflict
		}
		_ = tx.Rollback(ctx)
		return s.repository.sourceIntent(ctx, existingID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return SourceIntent{}, err
	}
	if revision != input.ExpectedSourceRevision {
		return SourceIntent{}, ErrInputStale
	}
	if status != "ACTIVE" {
		return SourceIntent{}, &BlockedError{Code: "DOCUMENT_SET_INACTIVE", Message: "Bộ tài liệu đã lưu trữ; hãy khôi phục trước khi xử lý."}
	}
	rows, err := tx.Query(ctx, `SELECT v.id,v.sha256,v.approval_status,v.parse_status FROM documents d
 JOIN LATERAL(SELECT * FROM document_versions WHERE document_id=d.id ORDER BY version_number DESC LIMIT 1)v ON TRUE
 WHERE d.document_set_id=$1 ORDER BY v.id FOR UPDATE OF d`, setID)
	if err != nil {
		return SourceIntent{}, err
	}
	type version struct{ hash, approval, parse string }
	versions := map[int64]version{}
	for rows.Next() {
		var id int64
		var v version
		if err = rows.Scan(&id, &v.hash, &v.approval, &v.parse); err != nil {
			rows.Close()
			return SourceIntent{}, err
		}
		versions[id] = v
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return SourceIntent{}, err
	}
	excluded := map[int64]bool{}
	selected := map[int64]bool{}
	for _, id := range input.ExcludedVersionIDs {
		if _, ok := versions[id]; !ok || excluded[id] {
			return SourceIntent{}, ErrInvalidInput
		}
		excluded[id] = true
	}
	for _, selection := range input.Selected {
		v, ok := versions[selection.VersionID]
		if !ok || selected[selection.VersionID] || excluded[selection.VersionID] || v.hash != selection.SHA256 || v.approval != selection.ApprovalStatus {
			return SourceIntent{}, ErrRevisionConflict
		}
		if v.parse != "PARSED" {
			return SourceIntent{}, &BlockedError{Code: "SOURCE_NOT_PARSED", Message: "Chỉ có thể duyệt phiên bản đã đọc xong."}
		}
		selected[selection.VersionID] = true
		if v.approval == "APPROVED" {
			continue
		}
		result, updateErr := tx.Exec(ctx, `UPDATE document_versions SET approval_status='APPROVED'
   WHERE id=$1 AND sha256=$2 AND approval_status=$3 AND parse_status='PARSED'`, selection.VersionID, selection.SHA256, selection.ApprovalStatus)
		if updateErr != nil {
			return SourceIntent{}, updateErr
		}
		if result.RowsAffected() != 1 {
			return SourceIntent{}, ErrRevisionConflict
		}
		if _, err = tx.Exec(ctx, `INSERT INTO document_version_reviews(document_version_id,reviewer_name,decision,comment)
   VALUES($1,$2,'APPROVED',$3)`, selection.VersionID, actor, "Reviewer display name: "+input.ReviewerName); err != nil {
			return SourceIntent{}, err
		}
		v.approval = "APPROVED"
		versions[selection.VersionID] = v
	}
	if len(versions) == len(excluded) {
		return SourceIntent{}, ErrInvalidInput
	}
	if input.Command == "EXTRACT" || input.Command == "APPROVE_AND_EXTRACT" {
		for id, v := range versions {
			if !excluded[id] && (v.approval != "APPROVED" || v.parse != "PARSED") {
				return SourceIntent{}, &BlockedError{Code: "SOURCE_NOT_APPROVED", Message: "Hãy duyệt tất cả nguồn trong phạm vi hoặc chọn loại nguồn chưa sẵn sàng.", NextAction: "REVIEW_SOURCE"}
			}
		}
	}
	var id int64
	if err = tx.QueryRow(ctx, `INSERT INTO document_source_intents(document_set_id,source_revision,command,request,idempotency_key,requested_by)
 VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, setID, revision, input.Command, payload, key, actor).Scan(&id); err != nil {
		return SourceIntent{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return SourceIntent{}, err
	}
	return s.repository.sourceIntent(ctx, id)
}

func (r *Repository) sourceIntent(ctx context.Context, id int64) (SourceIntent, error) {
	var v SourceIntent
	err := r.pool.QueryRow(ctx, `SELECT id,document_set_id,source_revision,command,status,request,index_job_id,extraction_job_id,error_message FROM document_source_intents WHERE id=$1`, id).
		Scan(&v.ID, &v.DocumentSetID, &v.SourceRevision, &v.Command, &v.Status, &v.Request, &v.IndexJobID, &v.ExtractionJobID, &v.ErrorMessage)
	return v, err
}
func (r *Repository) SourceIntents(ctx context.Context, setID int64) ([]SourceIntent, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,document_set_id,source_revision,command,status,request,index_job_id,extraction_job_id,error_message FROM document_source_intents WHERE document_set_id=$1 ORDER BY id DESC LIMIT 20`, setID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SourceIntent{}
	for rows.Next() {
		var v SourceIntent
		if err = rows.Scan(&v.ID, &v.DocumentSetID, &v.SourceRevision, &v.Command, &v.Status, &v.Request, &v.IndexJobID, &v.ExtractionJobID, &v.ErrorMessage); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// AdvanceSources is called by the worker, never by a GET or browser timer.
// Row locks serialize each continuation; enqueue keys survive crash before acknowledgement.
func (s *Service) AdvanceSources(ctx context.Context) error {
	rows, err := s.repository.pool.Query(ctx, `SELECT id FROM document_source_intents WHERE status IN ('WAITING_PARSE','INDEXING','EXTRACTING') ORDER BY updated_at,id LIMIT 20`)
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = s.advanceSource(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) advanceSource(ctx context.Context, id int64) error {
	tx, err := s.repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var locked int64
	err = tx.QueryRow(ctx, `SELECT id FROM document_source_intents WHERE id=$1 AND status IN ('WAITING_PARSE','INDEXING','EXTRACTING') FOR UPDATE SKIP LOCKED`, id).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	intent, err := s.repository.sourceIntent(ctx, id)
	if err != nil {
		return err
	}
	update := func(status, message string) error {
		_, err := tx.Exec(ctx, `UPDATE document_source_intents SET status=$2,error_message=$3,updated_at=NOW() WHERE id=$1`, id, status, message)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	var revision int64
	var setStatus string
	if err = tx.QueryRow(ctx, `SELECT source_revision,status FROM document_sets WHERE id=$1 FOR SHARE`, intent.DocumentSetID).Scan(&revision, &setStatus); err != nil {
		return err
	}
	if revision != intent.SourceRevision && intent.Status != "EXTRACTING" {
		return update("SUPERSEDED", "Có phiên bản nguồn mới; thao tác này giữ phạm vi cũ và không tự trích xuất nguồn mới.")
	}
	if setStatus != "ACTIVE" {
		return update("FAILED", "Bộ tài liệu đã được lưu trữ.")
	}
	var command SourceCommand
	if err = json.Unmarshal(intent.Request, &command); err != nil {
		return err
	}
	if intent.IndexJobID == nil {
		_, _, snapshotErr := s.repository.BuildIndexSnapshot(ctx, intent.DocumentSetID, command.ExcludedVersionIDs)
		if snapshotErr != nil {
			var pending int
			if err = tx.QueryRow(ctx, `SELECT count(*) FROM document_versions v WHERE document_set_id=$1 AND parse_status IN ('UPLOADED','PARSING') AND NOT EXISTS(SELECT 1 FROM document_versions n WHERE n.document_id=v.document_id AND n.version_number>v.version_number)`, intent.DocumentSetID).Scan(&pending); err != nil {
				return err
			}
			if pending > 0 {
				return update("WAITING_PARSE", "")
			}
			return update("FAILED", "Có tài liệu chưa đọc được. Tải bản khác hoặc chọn loại khỏi phạm vi rồi tiếp tục.")
		}
		job, _, enqueueErr := s.Enqueue(ctx, intent.DocumentSetID, OperationInput{Operation: OperationIndex, RequestedBy: "SOURCE_WORKFLOW", ExcludedVersionIDs: command.ExcludedVersionIDs}, fmt.Sprintf("source:%d:index", id), "editor")
		if errors.Is(enqueueErr, ErrOperationActive) {
			return update("WAITING_PARSE", "")
		}
		if enqueueErr != nil {
			return update("FAILED", enqueueErr.Error())
		}
		if _, err = tx.Exec(ctx, `UPDATE document_source_intents SET index_job_id=$2,status='INDEXING',updated_at=NOW() WHERE id=$1`, id, job.ID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	index, err := s.Get(ctx, *intent.IndexJobID)
	if err != nil {
		return err
	}
	if !Terminal(index.Status) {
		return update("INDEXING", "")
	}
	if index.Status != StatusSucceeded {
		return update("FAILED", "Xử lý nguồn chưa hoàn tất. Xem chi tiết xử lý để thử lại hoặc dừng.")
	}
	if intent.Command == "INDEX" || intent.Command == "APPROVE" {
		return update("SUCCEEDED", "")
	}
	if intent.ExtractionJobID == nil {
		// Another index operation may have changed scope without a new upload.
		// Hold the index row stable until the extraction enqueue is acknowledged.
		var currentGeneration int64
		if err = tx.QueryRow(ctx, `SELECT generation FROM document_index_status WHERE document_set_id=$1 FOR SHARE`, intent.DocumentSetID).Scan(&currentGeneration); err != nil {
			return err
		}
		var output struct {
			Generation int64 `json:"index_generation"`
		}
		if err = json.Unmarshal(index.OutputRefs, &output); err != nil {
			return err
		}
		if currentGeneration != output.Generation {
			return update("SUPERSEDED", "Phạm vi tìm kiếm đã thay đổi. Kiểm tra nguồn và xác nhận lại yêu cầu trích xuất.")
		}
		job, _, enqueueErr := s.Enqueue(ctx, intent.DocumentSetID, OperationInput{Operation: OperationExtract, RequestedBy: "SOURCE_WORKFLOW"}, fmt.Sprintf("source:%d:extract", id), "editor")
		if errors.Is(enqueueErr, ErrOperationActive) {
			return update("INDEXING", "")
		}
		if enqueueErr != nil {
			return update("FAILED", enqueueErr.Error())
		}
		if _, err = tx.Exec(ctx, `UPDATE document_source_intents SET extraction_job_id=$2,status='EXTRACTING',updated_at=NOW() WHERE id=$1`, id, job.ID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	extraction, err := s.Get(ctx, *intent.ExtractionJobID)
	if err != nil {
		return err
	}
	if !Terminal(extraction.Status) {
		return update("EXTRACTING", "")
	}
	if extraction.Status != StatusSucceeded {
		return update("FAILED", "Trích xuất chưa hoàn tất. Quyết định duyệt nguồn đã được giữ lại; mở chi tiết để thử lại job.")
	}
	return update("SUCCEEDED", "")
}
