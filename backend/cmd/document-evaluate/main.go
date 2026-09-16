package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/doceval"
)

func main() {
	input := flag.String("input", "", "path to a document-testing golden dataset")
	output := flag.String("output", "", "optional JSON report path; stdout when empty")
	flag.Parse()
	if *input == "" {
		fatal("-input is required")
	}
	content, err := os.ReadFile(*input)
	if err != nil {
		fatal(err.Error())
	}
	dataset, err := doceval.Parse(content)
	if err != nil {
		fatal(err.Error())
	}
	report, err := doceval.Evaluate(dataset)
	if err != nil {
		fatal(err.Error())
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fatal(err.Error())
	}
	encoded = append(encoded, '\n')
	if *output == "" {
		_, err = os.Stdout.Write(encoded)
	} else {
		err = os.WriteFile(*output, encoded, 0o600)
	}
	if err != nil {
		fatal(err.Error())
	}
}
func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
