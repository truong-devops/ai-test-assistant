package evaluation

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

var ErrInvalidDataset = errors.New("invalid evaluation dataset")

var experimentVariants = map[string][2]string{
	ExperimentContextImpact: {VariantDiffOnly, VariantDiffRAG},
	ExperimentRepairImpact:  {VariantGenerateOnly, VariantGenerateRepair},
	ExperimentHumanEffort:   {VariantManual, VariantAIAssisted},
}

func ValidateDataset(dataset Dataset) error {
	if dataset.SchemaVersion != SchemaVersionV1 {
		return invalid("schema_version must be %q", SchemaVersionV1)
	}
	if strings.TrimSpace(dataset.Name) == "" || len(dataset.Name) > 200 {
		return invalid("name must contain between 1 and 200 characters")
	}
	if len(dataset.Description) > 4000 {
		return invalid("description exceeds 4000 characters")
	}
	if len(dataset.Observations) == 0 {
		return invalid("at least one observation is required")
	}

	keys := make(map[string]struct{}, len(dataset.Observations))
	pairs := make(map[string]map[string]struct{})
	for index, observation := range dataset.Observations {
		if err := validateObservation(observation); err != nil {
			return invalid("observation %d: %v", index+1, err)
		}
		if _, exists := keys[observation.Key]; exists {
			return invalid("observation key %q is duplicated", observation.Key)
		}
		keys[observation.Key] = struct{}{}
		pairKey := fmt.Sprintf("%s\x00%s\x00%d", observation.Experiment, observation.Scenario, observation.Replicate)
		if pairs[pairKey] == nil {
			pairs[pairKey] = make(map[string]struct{})
		}
		if _, exists := pairs[pairKey][observation.Variant]; exists {
			return invalid("paired scenario %q contains variant %s more than once", pairKey, observation.Variant)
		}
		pairs[pairKey][observation.Variant] = struct{}{}
	}

	for pairKey, variants := range pairs {
		experiment := strings.SplitN(pairKey, "\x00", 2)[0]
		expected := experimentVariants[experiment]
		for _, variant := range expected {
			if _, exists := variants[variant]; !exists {
				return invalid("paired scenario %q is missing variant %s", pairKey, variant)
			}
		}
	}
	for experiment, variants := range experimentVariants {
		found := false
		for _, observation := range dataset.Observations {
			if observation.Experiment == experiment {
				found = true
				break
			}
		}
		if !found {
			return invalid("experiment %s has no observations for variants %s and %s",
				experiment, variants[0], variants[1])
		}
	}
	return nil
}

func validateObservation(item Observation) error {
	if strings.TrimSpace(item.Key) == "" || len(item.Key) > 200 {
		return errors.New("key must contain between 1 and 200 characters")
	}
	if strings.TrimSpace(item.Scenario) == "" || len(item.Scenario) > 500 {
		return errors.New("scenario must contain between 1 and 500 characters")
	}
	variants, exists := experimentVariants[item.Experiment]
	if !exists {
		return fmt.Errorf("unsupported experiment %q", item.Experiment)
	}
	if item.Variant != variants[0] && item.Variant != variants[1] {
		return fmt.Errorf("variant %q does not belong to experiment %s", item.Variant, item.Experiment)
	}
	if item.Replicate <= 0 {
		return errors.New("replicate must be positive")
	}
	if len(item.Notes) > 4000 {
		return errors.New("notes exceeds 4000 characters")
	}
	for name, value := range map[string]*float64{
		"coverage_before_pct": item.CoverageBeforePct,
		"coverage_after_pct":  item.CoverageAfterPct,
	} {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 || *value > 100) {
			return fmt.Errorf("%s must be between 0 and 100", name)
		}
	}
	if item.DurationSeconds != nil && (math.IsNaN(*item.DurationSeconds) || math.IsInf(*item.DurationSeconds, 0) || *item.DurationSeconds <= 0) {
		return errors.New("duration_seconds must be positive")
	}
	if trueValue(item.CompileValid) && !trueValue(item.SyntacticValid) {
		return errors.New("compile_valid requires syntactic_valid")
	}
	if trueValue(item.ExecutionValid) && !trueValue(item.CompileValid) {
		return errors.New("execution_valid requires compile_valid")
	}
	if trueValue(item.RepairSuccess) && !trueValue(item.RepairAttempted) {
		return errors.New("repair_success requires repair_attempted")
	}
	if trueValue(item.FirstPassSuccess) && !trueValue(item.FinalSuccess) {
		return errors.New("first_pass_success requires final_success")
	}

	switch item.Experiment {
	case ExperimentContextImpact:
		if anyNil(item.SyntacticValid, item.CompileValid, item.ExecutionValid, item.HumanAccepted) ||
			item.CoverageBeforePct == nil || item.CoverageAfterPct == nil {
			return errors.New("context impact requires validity, acceptance, and coverage metrics")
		}
	case ExperimentRepairImpact:
		if anyNil(item.SyntacticValid, item.CompileValid, item.ExecutionValid,
			item.FirstPassSuccess, item.FinalSuccess) {
			return errors.New("repair impact requires validity, first-pass, and final-success metrics")
		}
		if item.Variant == VariantGenerateOnly {
			if item.RepairAttempted != nil || item.RepairSuccess != nil {
				return errors.New("generate-only observations cannot contain repair metrics")
			}
			if *item.FirstPassSuccess != *item.FinalSuccess {
				return errors.New("generate-only final_success must equal first_pass_success")
			}
		} else {
			if item.RepairAttempted == nil {
				return errors.New("generate-repair observations require repair_attempted")
			}
			if *item.RepairAttempted != (item.RepairSuccess != nil) {
				return errors.New("repair_success must be present exactly when repair was attempted")
			}
			if *item.FirstPassSuccess && *item.RepairAttempted {
				return errors.New("a first-pass success must not attempt repair")
			}
			if !*item.FirstPassSuccess && !*item.RepairAttempted {
				return errors.New("a failed first pass must attempt repair")
			}
			wantFinal := *item.FirstPassSuccess || trueValue(item.RepairSuccess)
			if *item.FinalSuccess != wantFinal {
				return errors.New("final_success must reflect first-pass or repair success")
			}
		}
		if *item.FinalSuccess != *item.ExecutionValid {
			return errors.New("final_success must equal execution_valid")
		}
	case ExperimentHumanEffort:
		if item.DurationSeconds == nil || item.HumanAccepted == nil ||
			item.CoverageBeforePct == nil || item.CoverageAfterPct == nil {
			return errors.New("human effort requires duration, acceptance, and coverage metrics")
		}
	}
	return nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidDataset, fmt.Sprintf(format, args...))
}

func anyNil(values ...*bool) bool {
	for _, value := range values {
		if value == nil {
			return true
		}
	}
	return false
}

func trueValue(value *bool) bool { return value != nil && *value }
