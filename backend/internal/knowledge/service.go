package knowledge

import (
	"context"
	"errors"
	"fmt"
)

type IndexRequester interface {
	RequestIndex(ctx context.Context, projectID int64, ref string) (IndexJob, error)
	GetIndex(ctx context.Context, projectID int64) (IndexJob, error)
}

type Service struct {
	projects ProjectGetter
	indexes  IndexRequester
}

func NewService(projects ProjectGetter, indexes IndexRequester) *Service {
	return &Service{projects: projects, indexes: indexes}
}

func (s *Service) RequestIndex(ctx context.Context, projectID int64) (IndexJob, error) {
	registeredProject, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		return IndexJob{}, err
	}
	result, err := s.indexes.RequestIndex(ctx, registeredProject.ID, registeredProject.DefaultBranch)
	if err != nil {
		return IndexJob{}, fmt.Errorf("request index: %w", err)
	}
	return result, nil
}

func (s *Service) GetIndexStatus(ctx context.Context, projectID int64) (IndexJob, error) {
	registeredProject, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		return IndexJob{}, err
	}
	result, err := s.indexes.GetIndex(ctx, registeredProject.ID)
	if errors.Is(err, ErrIndexNotFound) {
		return IndexJob{ProjectID: registeredProject.ID, Ref: registeredProject.DefaultBranch,
			Status: IndexStatusNotIndexed}, nil
	}
	return result, err
}
