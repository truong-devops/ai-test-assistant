package analyzer

type Symbol struct {
	Name        string
	Kind        string
	Receiver    string
	PackageName string
	StartLine   int
	EndLine     int
}

type ParsedFile struct {
	PackageName string
	Symbols     []Symbol
}

type ChangedLines struct {
	Old map[int]struct{}
	New map[int]struct{}
}

type SymbolChange struct {
	Symbol
	ChangeType string
	Summary    string
}
