package product

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrInvalidQuantity   = errors.New("quantity must be positive")
	ErrInsufficientStock = errors.New("insufficient stock")
)

type Product struct {
	ID    int64
	Stock int
}

type Repository interface {
	GetByID(ctx context.Context, id int64) (Product, error)
	UpdateStock(ctx context.Context, id int64, stock int) error
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) ReserveStock(ctx context.Context, productID int64, quantity int) error {
	if quantity <= 0 {
		return ErrInvalidQuantity
	}
	product, err := s.repository.GetByID(ctx, productID)
	if err != nil {
		return fmt.Errorf("get product: %w", err)
	}
	if product.Stock < quantity {
		return ErrInsufficientStock
	}
	if err := s.repository.UpdateStock(ctx, productID, product.Stock-quantity); err != nil {
		return fmt.Errorf("update stock: %w", err)
	}
	return nil
}
