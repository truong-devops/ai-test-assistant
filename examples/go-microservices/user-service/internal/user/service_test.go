package user

import (
	"context"
	"errors"
	"testing"
)

func TestServiceCreateUser(t *testing.T) {
	tests := []struct {
		name       string
		email      string
		repository *mockRepository
		wantErr    error
	}{
		{name: "creates normalized email", email: " USER@Example.com ", repository: &mockRepository{findErr: ErrNotFound}},
		{name: "rejects invalid email", email: "invalid", repository: &mockRepository{}, wantErr: ErrInvalidEmail},
		{name: "rejects duplicate", email: "user@example.com", repository: &mockRepository{findResult: User{ID: 9}}, wantErr: ErrEmailExists},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			created, err := NewService(test.repository).CreateUser(context.Background(), test.email)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("CreateUser() error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil && (created.ID == 0 || created.Email != "user@example.com") {
				t.Fatalf("CreateUser() = %+v", created)
			}
		})
	}
}
