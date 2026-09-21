package document

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (s *Service) ListVersions(ctx context.Context, setID, documentID int64) ([]Version, error) {
	repo, ok := s.repository.(interface {
		ListVersions(context.Context, int64, int64) ([]Version, error)
	})
	if !ok {
		return nil, ErrUnsupported
	}
	if setID <= 0 || documentID <= 0 {
		return nil, ErrInvalidInput
	}
	return repo.ListVersions(ctx, setID, documentID)
}

func (r *PostgresRepository) ListVersions(ctx context.Context, setID, documentID int64) ([]Version, error) {
	var id int64
	if err := r.pool.QueryRow(ctx, `SELECT id FROM documents WHERE id=$1 AND document_set_id=$2`, documentID, setID).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT id,document_id,document_set_id,version_number,original_filename,
 media_type,size_bytes,sha256,storage_key,approval_status,parse_status,parse_error,block_count,
 attempt_count,next_attempt_at,lease_expires_at,uploaded_at,started_at,parsed_at
 FROM document_versions WHERE document_id=$1 ORDER BY version_number DESC`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Version{}
	for rows.Next() {
		var v Version
		if err := rows.Scan(versionDestinations(&v)...); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
