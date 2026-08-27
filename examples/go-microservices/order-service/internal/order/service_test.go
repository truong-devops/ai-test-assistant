package order

import (
	"context"
	"errors"
	"testing"
)

type repositoryMock struct{ calls int }

func (r *repositoryMock) Create(_ context.Context, order Order) (Order, error) {
	r.calls++
	order.ID = 1
	return order, nil
}

func TestServicePlaceOrderQuantityBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		quantity int
		wantErr  error
	}{
		{name: "minimum", quantity: 1},
		{name: "maximum", quantity: MaxQuantity},
		{name: "zero", quantity: 0, wantErr: ErrInvalidQuantity},
		{name: "over maximum", quantity: MaxQuantity + 1, wantErr: ErrInvalidQuantity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &repositoryMock{}
			_, err := NewService(repository).PlaceOrder(context.Background(), 10, test.quantity)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("PlaceOrder() error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr != nil && repository.calls != 0 {
				t.Fatal("repository was called for invalid input")
			}
		})
	}
}
