package execution

import "strings"

func Classify(exitCode int, timedOut bool, stdout, stderr, artifactPath string, approved bool) (string, string) {
	combined := strings.ToLower(stderr + "\n" + stdout)
	if timedOut {
		return StatusTimedOut, "Sandbox execution exceeded its configured timeout."
	}
	if exitCode == 0 {
		return StatusPassed, "Automation completed and every assertion passed."
	}
	if containsAny(combined, "no space left on device", "cannot connect to the docker daemon",
		"temporary failure", "connection refused", "resource temporarily unavailable",
		"no required module provides package", "goproxy=off") {
		return StatusInfraError, summarizeOutput(stderr, stdout, "Execution environment or dependency setup failed.")
	}
	artifactMentioned := artifactPath != "" && strings.Contains(combined, strings.ToLower(artifactPath))
	if containsAny(combined, "build failed", "undefined:", "syntax error", "cannot use ",
		"imported and not used", "expected declaration", "setup failed") {
		return StatusAutomationError, summarizeOutput(stderr, stdout, "Generated automation did not compile or set up correctly.")
	}
	if strings.Contains(combined, "panic:") && artifactMentioned {
		return StatusAutomationError, summarizeOutput(stderr, stdout, "Generated automation panicked inside its test implementation.")
	}
	if approved && containsAny(combined, "--- fail:", "expected", "actual", "want ", "got ") {
		return StatusProductFailed, summarizeOutput(stderr, stdout, "Actual behavior did not satisfy the approved assertion.")
	}
	if !approved {
		return StatusAutomationError, summarizeOutput(stderr, stdout, "An unapproved automation artifact cannot establish a product failure.")
	}
	return StatusAutomationError, summarizeOutput(stderr, stdout, "The test process failed without a reliable product-failure signal.")
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func summarizeOutput(stderr, stdout, fallback string) string {
	value := strings.TrimSpace(stderr)
	if value == "" {
		value = strings.TrimSpace(stdout)
	}
	if value == "" {
		return fallback
	}
	const limit = 2000
	if len(value) > limit {
		value = value[len(value)-limit:]
	}
	return value
}
