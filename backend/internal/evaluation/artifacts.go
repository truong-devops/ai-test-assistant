package evaluation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxDatasetBytes = 10 << 20

func LoadDataset(path string) (Dataset, error) {
	file, err := os.Open(path)
	if err != nil {
		return Dataset{}, fmt.Errorf("open evaluation dataset: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maxDatasetBytes+1))
	decoder.DisallowUnknownFields()
	var dataset Dataset
	if err := decoder.Decode(&dataset); err != nil {
		return Dataset{}, fmt.Errorf("decode evaluation dataset: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Dataset{}, errors.New("evaluation dataset must contain exactly one JSON object")
	}
	if err := ValidateDataset(dataset); err != nil {
		return Dataset{}, err
	}
	return dataset, nil
}

func WriteArtifacts(directory string, report Report) error {
	jsonReport, err := RenderJSON(report)
	if err != nil {
		return err
	}
	csvReport, err := RenderCSV(report)
	if err != nil {
		return err
	}
	artifacts := map[string][]byte{
		"summary.json": jsonReport,
		"summary.csv":  csvReport,
		"report.md":    RenderMarkdown(report),
		"charts.svg":   RenderSVG(report),
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create artifact directory: %w", err)
	}
	for name, content := range artifacts {
		if err := writeAtomic(filepath.Join(directory, name), content); err != nil {
			return err
		}
	}
	return nil
}

func writeAtomic(path string, content []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".evaluation-*")
	if err != nil {
		return fmt.Errorf("create temporary artifact: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return fmt.Errorf("set artifact permissions: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("write artifact: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync artifact: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close artifact: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish artifact: %w", err)
	}
	return nil
}
