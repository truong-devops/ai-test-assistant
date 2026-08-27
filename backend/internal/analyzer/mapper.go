package analyzer

import (
	"fmt"
	"sort"
)

func MapChangedSymbols(oldFile, newFile *ParsedFile, lines ChangedLines) []SymbolChange {
	oldSymbols := symbolsByKey(oldFile)
	newSymbols := symbolsByKey(newFile)
	oldHits := hitSymbols(oldFile, lines.Old)
	newHits := hitSymbols(newFile, lines.New)

	keys := make(map[string]struct{}, len(oldHits)+len(newHits))
	for key := range oldHits {
		keys[key] = struct{}{}
	}
	for key := range newHits {
		keys[key] = struct{}{}
	}

	changes := make([]SymbolChange, 0, len(keys))
	for key := range keys {
		oldSymbol, existedBefore := oldSymbols[key]
		newSymbol, existsNow := newSymbols[key]
		changeType := "modified"
		selected := newSymbol
		switch {
		case !existedBefore && existsNow:
			changeType = "added"
		case existedBefore && !existsNow:
			changeType = "deleted"
			selected = oldSymbol
		case !existsNow:
			selected = oldSymbol
		}
		changes = append(changes, SymbolChange{
			Symbol: selected, ChangeType: changeType,
			Summary: fmt.Sprintf("%s %s %s overlaps changed lines %d-%d",
				changeType, selected.Kind, selected.Name, selected.StartLine, selected.EndLine),
		})
	}
	sortSymbolChanges(changes)
	return changes
}

func symbolsByKey(file *ParsedFile) map[string]Symbol {
	result := make(map[string]Symbol)
	if file == nil {
		return result
	}
	for _, symbol := range file.Symbols {
		result[symbolKey(symbol)] = symbol
	}
	return result
}

func hitSymbols(file *ParsedFile, changedLines map[int]struct{}) map[string]Symbol {
	result := make(map[string]Symbol)
	if file == nil || len(changedLines) == 0 {
		return result
	}
	lines := make([]int, 0, len(changedLines))
	for line := range changedLines {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	for _, symbol := range file.Symbols {
		index := sort.SearchInts(lines, symbol.StartLine)
		if index < len(lines) && lines[index] <= symbol.EndLine {
			result[symbolKey(symbol)] = symbol
		}
	}
	return result
}

func symbolKey(symbol Symbol) string {
	return symbol.Kind + "\x00" + symbol.Receiver + "\x00" + symbol.Name
}

func sortSymbolChanges(changes []SymbolChange) {
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].StartLine != changes[j].StartLine {
			return changes[i].StartLine < changes[j].StartLine
		}
		return changes[i].Name < changes[j].Name
	})
}
