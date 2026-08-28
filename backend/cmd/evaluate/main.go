package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/evaluation"
	"github.com/maccuatruong/ai-test-assistant/backend/internal/storage"
)

func main() {
	datasetPath := flag.String("dataset", "", "path to an evaluation-v1 JSON dataset")
	outputDirectory := flag.String("out", "", "directory for JSON, CSV, Markdown, and SVG artifacts")
	databaseURL := flag.String("database-url", "", "optional PostgreSQL URL for immutable run storage")
	flag.Parse()
	if *datasetPath == "" || *outputDirectory == "" {
		fmt.Fprintln(os.Stderr, "both -dataset and -out are required")
		os.Exit(2)
	}
	dataset, err := evaluation.LoadDataset(*datasetPath)
	if err != nil {
		fail(err)
	}
	report, err := evaluation.BuildReport(dataset)
	if err != nil {
		fail(err)
	}
	if err := evaluation.WriteArtifacts(*outputDirectory, report); err != nil {
		fail(err)
	}
	if *databaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		database, err := storage.Open(ctx, *databaseURL, 2)
		if err != nil {
			fail(err)
		}
		defer database.Close()
		stored, err := evaluation.NewService(evaluation.NewRepository(database.Pool())).Import(ctx, dataset)
		if err != nil {
			fail(err)
		}
		fmt.Printf("stored evaluation run %d\n", stored.Run.ID)
	}
	fmt.Printf("evaluation %s (%s) written to %s\n", report.DatasetName, report.DatasetHash, *outputDirectory)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
