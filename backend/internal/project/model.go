package project

import "time"

const StatusActive = "active"

type Project struct {
	ID                int64     `json:"id"`
	Name              string    `json:"name"`
	Provider          string    `json:"provider"`
	ProviderProjectID int64     `json:"provider_project_id"`
	RepositoryURL     string    `json:"repository_url"`
	DefaultBranch     string    `json:"default_branch"`
	Language          string    `json:"language"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	// GitLabProjectID is accepted by older in-process callers and is not emitted.
	GitLabProjectID int64 `json:"-"`
}

type CreateInput struct {
	Name              string `json:"name"`
	Provider          string `json:"provider"`
	ProviderProjectID int64  `json:"provider_project_id"`
	// GitLabProjectID preserves the Phase 1 request contract.
	GitLabProjectID int64  `json:"gitlab_project_id,omitempty"`
	RepositoryURL   string `json:"repository_url"`
	DefaultBranch   string `json:"default_branch"`
	Language        string `json:"language"`
}

func (p Project) ExternalProjectID() int64 {
	if p.ProviderProjectID > 0 {
		return p.ProviderProjectID
	}
	return p.GitLabProjectID
}
