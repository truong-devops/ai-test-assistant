package service

import "example.com/impactfixture/contract"

func Load(reader contract.Reader, id contract.ID) string {
	return reader.Read(contract.Normalize(id))
}

func Wrap[T any](value T) contract.Box[T] {
	return contract.Box[T]{Value: value}
}
