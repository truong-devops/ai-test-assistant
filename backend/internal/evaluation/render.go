package evaluation

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"
)

func RenderJSON(report Report) ([]byte, error) {
	result, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode evaluation report: %w", err)
	}
	return append(result, '\n'), nil
}

func RenderCSV(report Report) ([]byte, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	header := []string{
		"experiment", "variant", "observations", "syntactic_validity_pct",
		"compile_validity_pct", "execution_validity_pct", "human_acceptance_pct",
		"first_pass_success_pct", "repair_success_pct", "final_success_pct",
		"mean_duration_seconds", "mean_coverage_delta_pp",
	}
	if err := writer.Write(header); err != nil {
		return nil, fmt.Errorf("write evaluation CSV header: %w", err)
	}
	for _, group := range report.Groups {
		row := []string{
			group.Experiment, group.Variant, strconv.Itoa(group.ObservationCount),
			rateString(group.SyntacticValidity), rateString(group.CompileValidity),
			rateString(group.ExecutionValidity), rateString(group.HumanAcceptance),
			rateString(group.FirstPassSuccess), rateString(group.RepairSuccess),
			rateString(group.FinalSuccess), floatString(group.MeanDurationSeconds),
			floatString(group.MeanCoverageDeltaPP),
		}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("write evaluation CSV row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("flush evaluation CSV: %w", err)
	}
	return buffer.Bytes(), nil
}

func RenderMarkdown(report Report) []byte {
	var result strings.Builder
	fmt.Fprintf(&result, "# Evaluation report: %s\n\n", report.DatasetName)
	fmt.Fprintf(&result, "%s\n\n", report.Description)
	fmt.Fprintf(&result, "- Schema: `%s`\n- Dataset SHA-256: `%s`\n\n",
		report.SchemaVersion, report.DatasetHash)
	result.WriteString("A passing test is not treated as automatic evidence of usefulness. Syntax, compile, execution, coverage, and human acceptance remain separate metrics.\n\n")
	result.WriteString("## Variant summaries\n\n")
	result.WriteString("| Experiment | Variant | N | Syntax | Compile | Execution | Human acceptance | First pass | Repair success | Final success | Mean time (s) | Coverage Δ (pp) |\n")
	result.WriteString("|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, group := range report.Groups {
		fmt.Fprintf(&result, "| %s | %s | %d | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			group.Experiment, group.Variant, group.ObservationCount,
			rateMarkdown(group.SyntacticValidity), rateMarkdown(group.CompileValidity),
			rateMarkdown(group.ExecutionValidity), rateMarkdown(group.HumanAcceptance),
			rateMarkdown(group.FirstPassSuccess), rateMarkdown(group.RepairSuccess),
			rateMarkdown(group.FinalSuccess), floatMarkdown(group.MeanDurationSeconds),
			floatMarkdown(group.MeanCoverageDeltaPP))
	}
	result.WriteString("\n## Paired comparisons\n\n")
	for _, comparison := range report.Comparisons {
		fmt.Fprintf(&result, "### %s\n\n", comparison.Experiment)
		fmt.Fprintf(&result, "Treatment `%s` compared with baseline `%s`.\n\n",
			comparison.TreatmentVariant, comparison.BaselineVariant)
		result.WriteString("| Metric | Change |\n|---|---:|\n")
		appendComparison(&result, "Syntactic validity", comparison.SyntacticValidityDeltaPP, "pp")
		appendComparison(&result, "Compile validity", comparison.CompileValidityDeltaPP, "pp")
		appendComparison(&result, "Execution validity", comparison.ExecutionValidityDeltaPP, "pp")
		appendComparison(&result, "Human acceptance", comparison.HumanAcceptanceDeltaPP, "pp")
		appendComparison(&result, "First-pass success", comparison.FirstPassSuccessDeltaPP, "pp")
		appendComparison(&result, "Repair success rate", comparison.RepairSuccessRatePct, "%")
		appendComparison(&result, "Final success", comparison.FinalSuccessDeltaPP, "pp")
		appendComparison(&result, "Mean coverage delta", comparison.MeanCoverageDeltaChangePP, "pp")
		appendComparison(&result, "Mean time reduction", comparison.MeanDurationReductionSec, "s")
		appendComparison(&result, "Mean time reduction", comparison.MeanDurationReductionPct, "%")
		result.WriteByte('\n')
	}
	return []byte(result.String())
}

func RenderSVG(report Report) []byte {
	type chart struct {
		title  string
		groups []GroupSummary
		metric func(GroupSummary) (float64, bool)
		suffix string
	}
	charts := []chart{
		{title: "Experiment A · execution validity", groups: groupsFor(report, ExperimentContextImpact),
			metric: func(group GroupSummary) (float64, bool) {
				if group.ExecutionValidity == nil {
					return 0, false
				}
				return group.ExecutionValidity.RatePct, true
			}, suffix: "%"},
		{title: "Experiment B · final success", groups: groupsFor(report, ExperimentRepairImpact),
			metric: func(group GroupSummary) (float64, bool) {
				if group.FinalSuccess == nil {
					return 0, false
				}
				return group.FinalSuccess.RatePct, true
			}, suffix: "%"},
		{title: "Experiment C · mean effort", groups: groupsFor(report, ExperimentHumanEffort),
			metric: func(group GroupSummary) (float64, bool) {
				if group.MeanDurationSeconds == nil {
					return 0, false
				}
				return *group.MeanDurationSeconds, true
			}, suffix: "s"},
	}
	const width, panelHeight = 960, 240
	var result strings.Builder
	fmt.Fprintf(&result, "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\">\n",
		width, panelHeight*len(charts), width, panelHeight*len(charts))
	result.WriteString("<style>text{font-family:Inter,Arial,sans-serif;fill:#18303d}.title{font-size:18px;font-weight:700}.label{font-size:12px}.value{font-size:13px;font-weight:700}.track{fill:#e7ecef}.bar{fill:#087a6f}.bar.alt{fill:#295f8b}</style>\n")
	for chartIndex, current := range charts {
		y := chartIndex * panelHeight
		fmt.Fprintf(&result, "<g transform=\"translate(0 %d)\"><text class=\"title\" x=\"28\" y=\"36\">%s</text>\n",
			y, html.EscapeString(current.title))
		maxValue := 100.0
		if current.suffix == "s" {
			maxValue = 0
			for _, group := range current.groups {
				value, ok := current.metric(group)
				if ok && value > maxValue {
					maxValue = value
				}
			}
			if maxValue == 0 {
				maxValue = 1
			}
		}
		for index, group := range current.groups {
			value, ok := current.metric(group)
			if !ok {
				continue
			}
			barY := 68 + index*68
			barWidth := value / maxValue * 650
			className := "bar"
			if index%2 == 1 {
				className = "bar alt"
			}
			fmt.Fprintf(&result, "<text class=\"label\" x=\"28\" y=\"%d\">%s</text>", barY,
				html.EscapeString(group.Variant))
			fmt.Fprintf(&result, "<rect class=\"track\" x=\"220\" y=\"%d\" width=\"650\" height=\"24\" rx=\"4\"/>", barY-17)
			fmt.Fprintf(&result, "<rect class=\"%s\" x=\"220\" y=\"%d\" width=\"%.2f\" height=\"24\" rx=\"4\"/>",
				className, barY-17, barWidth)
			fmt.Fprintf(&result, "<text class=\"value\" x=\"885\" y=\"%d\">%.2f%s</text>\n",
				barY, value, current.suffix)
		}
		result.WriteString("</g>\n")
	}
	result.WriteString("</svg>\n")
	return []byte(result.String())
}

func groupsFor(report Report, experiment string) []GroupSummary {
	result := make([]GroupSummary, 0, 2)
	for _, group := range report.Groups {
		if group.Experiment == experiment {
			result = append(result, group)
		}
	}
	return result
}

func appendComparison(result *strings.Builder, label string, value *float64, suffix string) {
	if value == nil {
		return
	}
	fmt.Fprintf(result, "| %s | %+.2f %s |\n", label, *value, suffix)
}

func rateString(value *RateMetric) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(value.RatePct, 'f', 2, 64)
}

func floatString(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', 2, 64)
}

func rateMarkdown(value *RateMetric) string {
	if value == nil {
		return "—"
	}
	return fmt.Sprintf("%.2f%% (%d/%d)", value.RatePct, value.Successes, value.Total)
}

func floatMarkdown(value *float64) string {
	if value == nil {
		return "—"
	}
	return fmt.Sprintf("%.2f", *value)
}
