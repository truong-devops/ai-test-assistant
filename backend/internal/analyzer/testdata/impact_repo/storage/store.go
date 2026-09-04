package storage

import "example.com/impactfixture/contract"

type Store struct{}

func (Store) Read(id contract.ID) string {
	return string(id)
}
