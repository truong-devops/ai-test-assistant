package service

import (
	"testing"

	"example.com/impactfixture/contract"
	"example.com/impactfixture/storage"
)

func TestLoad(t *testing.T) {
	if got := Load(storage.Store{}, contract.ID("42")); got == "" {
		t.Fatal("Load returned an empty value")
	}
}
