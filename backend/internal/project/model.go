package project

import "time"

const StatusActive = "active"

type Project struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	GitLabProjectID int64     `json:"gitlab_project_id"`
	RepositoryURL   string    `json:"repository_url"`
	DefaultBranch   string    `json:"default_branch"`
	Language        string    `json:"language"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type CreateInput struct {
	Name            string `json:"name"`
	GitLabProjectID int64  `json:"gitlab_project_id"`
	RepositoryURL   string `json:"repository_url"`
	DefaultBranch   string `json:"default_branch"`
	Language        string `json:"language"`
}
