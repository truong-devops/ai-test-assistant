package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidEmail = errors.New("invalid email")
	ErrEmailExists  = errors.New("email already exists")
)

type User struct {
	ID    int64
	Email string
}

type Repository interface {
	FindByEmail(ctx context.Context, email string) (User, error)
	Create(ctx context.Context, user User) (User, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) CreateUser(ctx context.Context, email string) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") {
		return User{}, ErrInvalidEmail
	}

	existing, err := s.repository.FindByEmail(ctx, email)
	if err == nil && existing.ID != 0 {
		return User{}, ErrEmailExists
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return User{}, fmt.Errorf("find existing user: %w", err)
	}

	created, err := s.repository.Create(ctx, User{Email: email})
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return created, nil
}

var ErrNotFound = errors.New("user not found")
