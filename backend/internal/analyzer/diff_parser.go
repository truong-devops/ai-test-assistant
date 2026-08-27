package analyzer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var hunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func ParseChangedLines(diff string) (ChangedLines, error) {
	result := ChangedLines{Old: make(map[int]struct{}), New: make(map[int]struct{})}
	if strings.TrimSpace(diff) == "" {
		return result, nil
	}
	oldLine, newLine := 0, 0
	inHunk := false
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "@@") {
			matches := hunkHeaderPattern.FindStringSubmatch(line)
			if len(matches) != 3 {
				return ChangedLines{}, fmt.Errorf("invalid unified diff hunk header %q", line)
			}
			var err error
			oldLine, err = strconv.Atoi(matches[1])
			if err != nil {
				return ChangedLines{}, fmt.Errorf("parse old hunk line: %w", err)
			}
			newLine, err = strconv.Atoi(matches[2])
			if err != nil {
				return ChangedLines{}, fmt.Errorf("parse new hunk line: %w", err)
			}
			inHunk = true
			continue
		}
		if !inHunk || strings.HasPrefix(line, "\\ No newline at end of file") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+"):
			result.New[newLine] = struct{}{}
			newLine++
		case strings.HasPrefix(line, "-"):
			result.Old[oldLine] = struct{}{}
			oldLine++
		default:
			oldLine++
			newLine++
		}
	}
	return result, nil
}
