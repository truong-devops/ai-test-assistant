package scope

import (
	"reflect"
	"testing"
)

func TestExtractIdentifiersRejectsOrdinaryWordsAndKeepsExplicitLinks(t *testing.T) {
	raw := `{"title":"Update user story for UC-B08 and FR_02",` +
		`"body":"Covers REQ123 and ticket SHOP-71; not username or storybook"}`
	want := []string{"FR_02", "REQ123", "SHOP-71", "UC-B08"}
	if got := extractIdentifiers(raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("extractIdentifiers()=%v, want %v", got, want)
	}
}
