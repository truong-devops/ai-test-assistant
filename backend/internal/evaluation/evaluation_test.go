package evaluation

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func boolPointer(value bool) *bool        { return &value }
func floatPointer(value float64) *float64 { return &value }

func TestBuildReportSeparatesMetricsAndComparesVariants(t *testing.T) {
	report, err := BuildReport(validDataset())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Groups) != 6 || len(report.Comparisons) != 3 || len(report.DatasetHash) != 64 {
		t.Fatalf("report=%#v", report)
	}
	context := report.Comparisons[0]
	if context.Experiment != ExperimentContextImpact ||
		context.CompileValidityDeltaPP == nil || *context.CompileValidityDeltaPP != 100 ||
		context.HumanAcceptanceDeltaPP == nil || *context.HumanAcceptanceDeltaPP != 100 {
		t.Fatalf("context comparison=%#v", context)
	}
	repair := report.Comparisons[1]
	if repair.RepairSuccessRatePct == nil || *repair.RepairSuccessRatePct != 100 ||
		repair.FinalSuccessDeltaPP == nil || *repair.FinalSuccessDeltaPP != 100 {
		t.Fatalf("repair comparison=%#v", repair)
	}
	effort := report.Comparisons[2]
	if effort.MeanDurationReductionSec == nil || *effort.MeanDurationReductionSec != 60 ||
		effort.MeanDurationReductionPct == nil || *effort.MeanDurationReductionPct != 50 {
		t.Fatalf("effort comparison=%#v", effort)
	}
}

func TestWriteArtifacts(t *testing.T) {
	report, err := BuildReport(validDataset())
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := WriteArtifacts(directory, report); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"summary.json", "summary.csv", "report.md", "charts.svg"} {
		content, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil || len(content) == 0 {
			t.Fatalf("artifact %s: bytes=%d error=%v", name, len(content), err)
		}
	}
}

func TestValidateDatasetRejectsUnpairedAndInconsistentObservations(t *testing.T) {
	dataset := validDataset()
	dataset.Observations = dataset.Observations[:len(dataset.Observations)-1]
	if err := ValidateDataset(dataset); !errors.Is(err, ErrInvalidDataset) {
		t.Fatalf("unpaired error=%v", err)
	}
	dataset = validDataset()
	for index := range dataset.Observations {
		if dataset.Observations[index].Variant == VariantGenerateRepair {
			dataset.Observations[index].FinalSuccess = boolPointer(false)
			break
		}
	}
	if err := ValidateDataset(dataset); !errors.Is(err, ErrInvalidDataset) {
		t.Fatalf("inconsistent error=%v", err)
	}
}

func TestValidateDatasetRejectsDuplicateVariantWithinPair(t *testing.T) {
	dataset := validDataset()
	duplicate := dataset.Observations[0]
	duplicate.Key = "a-diff-duplicate"
	dataset.Observations = append(dataset.Observations, duplicate)
	if err := ValidateDataset(dataset); !errors.Is(err, ErrInvalidDataset) {
		t.Fatalf("duplicate pair error=%v", err)
	}
}

func TestValidateDatasetRejectsNonFiniteAndSkippedRepairMetrics(t *testing.T) {
	dataset := validDataset()
	dataset.Observations[0].CoverageAfterPct = floatPointer(math.NaN())
	if err := ValidateDataset(dataset); !errors.Is(err, ErrInvalidDataset) {
		t.Fatalf("non-finite error=%v", err)
	}
	dataset = validDataset()
	for index := range dataset.Observations {
		if dataset.Observations[index].Variant == VariantGenerateRepair {
			dataset.Observations[index].RepairAttempted = boolPointer(false)
			dataset.Observations[index].RepairSuccess = nil
			dataset.Observations[index].FinalSuccess = boolPointer(false)
			dataset.Observations[index].ExecutionValid = boolPointer(false)
			break
		}
	}
	if err := ValidateDataset(dataset); !errors.Is(err, ErrInvalidDataset) {
		t.Fatalf("skipped repair error=%v", err)
	}
}

func TestRenderersProducePortableArtifacts(t *testing.T) {
	report, err := BuildReport(validDataset())
	if err != nil {
		t.Fatal(err)
	}
	jsonReport, err := RenderJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(jsonReport, &decoded); err != nil || decoded.DatasetHash != report.DatasetHash {
		t.Fatalf("decoded=%#v error=%v", decoded, err)
	}
	csvReport, err := RenderCSV(report)
	if err != nil || !strings.Contains(string(csvReport), "syntactic_validity_pct") {
		t.Fatalf("csv=%q error=%v", csvReport, err)
	}
	markdown := string(RenderMarkdown(report))
	if !strings.Contains(markdown, "A passing test is not treated") ||
		!strings.Contains(markdown, "Paired comparisons") {
		t.Fatalf("markdown=%q", markdown)
	}
	svg := string(RenderSVG(report))
	if !strings.Contains(svg, "<svg") || !strings.Contains(svg, "Experiment A") {
		t.Fatalf("svg=%q", svg)
	}
}

func validDataset() Dataset {
	contextBase := Observation{Key: "a-diff", Scenario: "duplicate branch",
		Experiment: ExperimentContextImpact, Variant: VariantDiffOnly, Replicate: 1,
		SyntacticValid: boolPointer(true), CompileValid: boolPointer(false),
		ExecutionValid: boolPointer(false), HumanAccepted: boolPointer(false),
		CoverageBeforePct: floatPointer(70), CoverageAfterPct: floatPointer(70)}
	contextRAG := contextBase
	contextRAG.Key, contextRAG.Variant = "a-rag", VariantDiffRAG
	contextRAG.CompileValid, contextRAG.ExecutionValid, contextRAG.HumanAccepted =
		boolPointer(true), boolPointer(true), boolPointer(true)
	contextRAG.CoverageAfterPct = floatPointer(74)

	generateOnly := Observation{Key: "b-only", Scenario: "compile repair",
		Experiment: ExperimentRepairImpact, Variant: VariantGenerateOnly, Replicate: 1,
		SyntacticValid: boolPointer(false), CompileValid: boolPointer(false),
		ExecutionValid: boolPointer(false), FirstPassSuccess: boolPointer(false),
		FinalSuccess: boolPointer(false)}
	generateRepair := generateOnly
	generateRepair.Key, generateRepair.Variant = "b-repair", VariantGenerateRepair
	generateRepair.SyntacticValid, generateRepair.CompileValid, generateRepair.ExecutionValid =
		boolPointer(true), boolPointer(true), boolPointer(true)
	generateRepair.RepairAttempted, generateRepair.RepairSuccess, generateRepair.FinalSuccess =
		boolPointer(true), boolPointer(true), boolPointer(true)

	manual := Observation{Key: "c-manual", Scenario: "write boundary test",
		Experiment: ExperimentHumanEffort, Variant: VariantManual, Replicate: 1,
		HumanAccepted: boolPointer(true), DurationSeconds: floatPointer(120),
		CoverageBeforePct: floatPointer(60), CoverageAfterPct: floatPointer(64)}
	assisted := manual
	assisted.Key, assisted.Variant = "c-assisted", VariantAIAssisted
	assisted.DurationSeconds = floatPointer(60)
	return Dataset{SchemaVersion: SchemaVersionV1, Name: "fixture",
		Description:  "Controlled test fixture.",
		Observations: []Observation{contextBase, contextRAG, generateOnly, generateRepair, manual, assisted}}
}
