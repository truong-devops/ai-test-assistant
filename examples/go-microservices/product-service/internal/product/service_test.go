package product

import (
	"context"
	"errors"
	"testing"
)

type repositoryMock struct {
	product      Product
	updatedStock int
}

func (r *repositoryMock) GetByID(context.Context, int64) (Product, error) { return r.product, nil }
func (r *repositoryMock) UpdateStock(_ context.Context, _ int64, stock int) error {
	r.updatedStock = stock
	return nil
}

func TestServiceReserveStock(t *testing.T) {
	tests := []struct {
		name      string
		stock     int
		quantity  int
		wantStock int
		wantErr   error
	}{
		{name: "reserves available stock", stock: 10, quantity: 4, wantStock: 6},
		{name: "allows exact stock", stock: 4, quantity: 4, wantStock: 0},
		{name: "rejects insufficient stock", stock: 3, quantity: 4, wantErr: ErrInsufficientStock},
		{name: "rejects non-positive quantity", stock: 3, quantity: 0, wantErr: ErrInvalidQuantity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &repositoryMock{product: Product{ID: 1, Stock: test.stock}}
			err := NewService(repository).ReserveStock(context.Background(), 1, test.quantity)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("ReserveStock() error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil && repository.updatedStock != test.wantStock {
				t.Fatalf("updated stock = %d, want %d", repository.updatedStock, test.wantStock)
			}
		})
	}
}
