package document

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestUV00SourceFixturesAreParseableAndDescribeTheIntendedChange(t *testing.T) {
	t.Parallel()
	parser := NewStructuredParser()
	contents := make(map[string]string, 2)

	for _, name := range []string{"order-v1.md", "order-v2.md"} {
		t.Run(name, func(t *testing.T) {
			payload, err := os.ReadFile("testdata/uv00/" + name)
			if err != nil {
				t.Fatal(err)
			}
			blocks, err := parser.Parse(context.Background(), payload, MediaTypeMarkdown, name)
			if err != nil {
				t.Fatal(err)
			}
			if len(blocks) == 0 {
				t.Fatal("fixture produced no structured blocks")
			}
			contents[name] = string(payload)
		})
	}

	for name, text := range contents {
		if !strings.Contains(text, "EX-01") || !strings.Contains(text, "EX-02") {
			t.Fatalf("%s must contain both negative scenarios", name)
		}
	}
	if strings.Contains(contents["order-v1.md"], "payment_method=COD") ||
		!strings.Contains(contents["order-v2.md"], "payment_method=COD") {
		t.Fatal("fixtures no longer isolate the v2 step/test-data change")
	}
}
