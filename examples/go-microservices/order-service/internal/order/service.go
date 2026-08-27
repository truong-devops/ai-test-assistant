package order

import (
	"context"
	"errors"
	"fmt"
)

const MaxQuantity = 20

var ErrInvalidQuantity = errors.New("quantity must be between 1 and 20")

type Order struct {
	ID        int64
	ProductID int64
	Quantity  int
}

type Repository interface {
	Create(ctx context.Context, order Order) (Order, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) PlaceOrder(ctx context.Context, productID int64, quantity int) (Order, error) {
	if productID <= 0 {
		return Order{}, errors.New("product id must be positive")
	}
	if quantity < 1 || quantity > MaxQuantity {
		return Order{}, ErrInvalidQuantity
	}
	created, err := s.repository.Create(ctx, Order{ProductID: productID, Quantity: quantity})
	if err != nil {
		return Order{}, fmt.Errorf("create order: %w", err)
	}
	return created, nil
}
