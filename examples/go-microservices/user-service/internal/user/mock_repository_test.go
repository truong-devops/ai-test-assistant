package user

import "context"

type mockRepository struct {
	findResult User
	findErr    error
	created    User
	createErr  error
}

func (m *mockRepository) FindByEmail(context.Context, string) (User, error) {
	return m.findResult, m.findErr
}

func (m *mockRepository) Create(_ context.Context, user User) (User, error) {
	m.created = user
	if m.createErr != nil {
		return User{}, m.createErr
	}
	user.ID = 1
	return user, nil
}
