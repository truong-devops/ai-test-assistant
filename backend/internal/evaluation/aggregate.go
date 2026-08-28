package evaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

func BuildReport(dataset Dataset) (Report, error) {
	if err := ValidateDataset(dataset); err != nil {
		return Report{}, err
	}
	encoded, err := json.Marshal(dataset)
	if err != nil {
		return Report{}, fmt.Errorf("encode evaluation dataset: %w", err)
	}
	digest := sha256.Sum256(encoded)
	grouped := make(map[string][]Observation)
	for _, observation := range dataset.Observations {
		key := observation.Experiment + "\x00" + observation.Variant
		grouped[key] = append(grouped[key], observation)
	}
	groups := make([]GroupSummary, 0, len(grouped))
	for _, observations := range grouped {
		groups = append(groups, summarize(observations))
	}
	sort.Slice(groups, func(i, j int) bool {
		left, right := groupOrder(groups[i]), groupOrder(groups[j])
		return left < right
	})

	report := Report{
		SchemaVersion: dataset.SchemaVersion,
		DatasetName:   dataset.Name,
		DatasetHash:   hex.EncodeToString(digest[:]),
		Description:   dataset.Description,
		Groups:        groups,
	}
	for _, experiment := range []string{
		ExperimentContextImpact, ExperimentRepairImpact, ExperimentHumanEffort,
	} {
		variants := experimentVariants[experiment]
		baseline, ok := findGroup(groups, experiment, variants[0])
		if !ok {
			return Report{}, fmt.Errorf("baseline group %s/%s is missing", experiment, variants[0])
		}
		treatment, ok := findGroup(groups, experiment, variants[1])
		if !ok {
			return Report{}, fmt.Errorf("treatment group %s/%s is missing", experiment, variants[1])
		}
		report.Comparisons = append(report.Comparisons, compare(experiment, baseline, treatment))
	}
	return report, nil
}

func summarize(observations []Observation) GroupSummary {
	result := GroupSummary{
		Experiment: observations[0].Experiment, Variant: observations[0].Variant,
		ObservationCount: len(observations),
	}
	result.SyntacticValidity = boolRate(observations, func(item Observation) *bool { return item.SyntacticValid })
	result.CompileValidity = boolRate(observations, func(item Observation) *bool { return item.CompileValid })
	result.ExecutionValidity = boolRate(observations, func(item Observation) *bool { return item.ExecutionValid })
	result.HumanAcceptance = boolRate(observations, func(item Observation) *bool { return item.HumanAccepted })
	result.FirstPassSuccess = boolRate(observations, func(item Observation) *bool { return item.FirstPassSuccess })
	result.RepairSuccess = boolRate(observations, func(item Observation) *bool { return item.RepairSuccess })
	result.FinalSuccess = boolRate(observations, func(item Observation) *bool { return item.FinalSuccess })
	result.MeanDurationSeconds = mean(observations, func(item Observation) *float64 { return item.DurationSeconds })
	result.MeanCoverageDeltaPP = mean(observations, func(item Observation) *float64 {
		if item.CoverageBeforePct == nil || item.CoverageAfterPct == nil {
			return nil
		}
		delta := *item.CoverageAfterPct - *item.CoverageBeforePct
		return &delta
	})
	return result
}

func boolRate(observations []Observation, selectValue func(Observation) *bool) *RateMetric {
	result := RateMetric{}
	for _, observation := range observations {
		value := selectValue(observation)
		if value == nil {
			continue
		}
		result.Total++
		if *value {
			result.Successes++
		}
	}
	if result.Total == 0 {
		return nil
	}
	result.RatePct = round(float64(result.Successes) / float64(result.Total) * 100)
	return &result
}

func mean(observations []Observation, selectValue func(Observation) *float64) *float64 {
	var total float64
	count := 0
	for _, observation := range observations {
		value := selectValue(observation)
		if value == nil {
			continue
		}
		total += *value
		count++
	}
	if count == 0 {
		return nil
	}
	result := round(total / float64(count))
	return &result
}

func compare(experiment string, baseline, treatment GroupSummary) Comparison {
	result := Comparison{
		Experiment: experiment, BaselineVariant: baseline.Variant,
		TreatmentVariant:          treatment.Variant,
		SyntacticValidityDeltaPP:  rateDelta(baseline.SyntacticValidity, treatment.SyntacticValidity),
		CompileValidityDeltaPP:    rateDelta(baseline.CompileValidity, treatment.CompileValidity),
		ExecutionValidityDeltaPP:  rateDelta(baseline.ExecutionValidity, treatment.ExecutionValidity),
		HumanAcceptanceDeltaPP:    rateDelta(baseline.HumanAcceptance, treatment.HumanAcceptance),
		FirstPassSuccessDeltaPP:   rateDelta(baseline.FirstPassSuccess, treatment.FirstPassSuccess),
		FinalSuccessDeltaPP:       rateDelta(baseline.FinalSuccess, treatment.FinalSuccess),
		MeanCoverageDeltaChangePP: valueDelta(baseline.MeanCoverageDeltaPP, treatment.MeanCoverageDeltaPP),
	}
	if treatment.RepairSuccess != nil {
		value := treatment.RepairSuccess.RatePct
		result.RepairSuccessRatePct = &value
	}
	if baseline.MeanDurationSeconds != nil && treatment.MeanDurationSeconds != nil {
		reduction := round(*baseline.MeanDurationSeconds - *treatment.MeanDurationSeconds)
		result.MeanDurationReductionSec = &reduction
		if *baseline.MeanDurationSeconds > 0 {
			percent := round(reduction / *baseline.MeanDurationSeconds * 100)
			result.MeanDurationReductionPct = &percent
		}
	}
	return result
}

func rateDelta(baseline, treatment *RateMetric) *float64 {
	if baseline == nil || treatment == nil {
		return nil
	}
	value := round(treatment.RatePct - baseline.RatePct)
	return &value
}

func valueDelta(baseline, treatment *float64) *float64 {
	if baseline == nil || treatment == nil {
		return nil
	}
	value := round(*treatment - *baseline)
	return &value
}

func findGroup(groups []GroupSummary, experiment, variant string) (GroupSummary, bool) {
	for _, group := range groups {
		if group.Experiment == experiment && group.Variant == variant {
			return group, true
		}
	}
	return GroupSummary{}, false
}

func groupOrder(group GroupSummary) string {
	order := map[string]string{
		ExperimentContextImpact + "\x00" + VariantDiffOnly:      "1",
		ExperimentContextImpact + "\x00" + VariantDiffRAG:       "2",
		ExperimentRepairImpact + "\x00" + VariantGenerateOnly:   "3",
		ExperimentRepairImpact + "\x00" + VariantGenerateRepair: "4",
		ExperimentHumanEffort + "\x00" + VariantManual:          "5",
		ExperimentHumanEffort + "\x00" + VariantAIAssisted:      "6",
	}
	return order[group.Experiment+"\x00"+group.Variant]
}

func round(value float64) float64 {
	const precision = 100
	if value >= 0 {
		return float64(int64(value*precision+0.5)) / precision
	}
	return float64(int64(value*precision-0.5)) / precision
}
