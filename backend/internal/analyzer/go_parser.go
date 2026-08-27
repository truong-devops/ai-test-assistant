package analyzer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

func ParseGoFile(filename string, source []byte) (ParsedFile, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, filename, source, parser.ParseComments)
	if err != nil {
		return ParsedFile{}, fmt.Errorf("parse Go file %q: %w", filename, err)
	}
	result := ParsedFile{PackageName: file.Name.Name, Symbols: make([]Symbol, 0)}
	for _, declaration := range file.Decls {
		switch typed := declaration.(type) {
		case *ast.FuncDecl:
			kind := "function"
			receiver := ""
			if typed.Recv != nil && len(typed.Recv.List) > 0 {
				kind = "method"
				receiver = receiverName(typed.Recv.List[0].Type)
			}
			if typed.Recv == nil && strings.HasSuffix(filename, "_test.go") && strings.HasPrefix(typed.Name.Name, "Test") {
				kind = "test"
			}
			result.Symbols = append(result.Symbols, symbolFromNode(fileSet, typed.Name.Name, kind,
				receiver, result.PackageName, typed))
		case *ast.GenDecl:
			result.Symbols = append(result.Symbols, symbolsFromDeclaration(fileSet, typed, result.PackageName)...)
		}
	}
	return result, nil
}

func symbolsFromDeclaration(fileSet *token.FileSet, declaration *ast.GenDecl, packageName string) []Symbol {
	result := make([]Symbol, 0)
	for _, specification := range declaration.Specs {
		switch typed := specification.(type) {
		case *ast.TypeSpec:
			kind := "type"
			switch typed.Type.(type) {
			case *ast.StructType:
				kind = "struct"
			case *ast.InterfaceType:
				kind = "interface"
			}
			result = append(result, symbolFromNode(fileSet, typed.Name.Name, kind, "", packageName, typed))
		case *ast.ValueSpec:
			kind := "variable"
			if declaration.Tok == token.CONST {
				kind = "constant"
			}
			for _, name := range typed.Names {
				result = append(result, symbolFromNode(fileSet, name.Name, kind, "", packageName, typed))
			}
		}
	}
	return result
}

func symbolFromNode(fileSet *token.FileSet, name, kind, receiver, packageName string, node ast.Node) Symbol {
	return Symbol{
		Name: name, Kind: kind, Receiver: receiver, PackageName: packageName,
		StartLine: fileSet.Position(node.Pos()).Line,
		EndLine:   fileSet.Position(node.End()).Line,
	}
}

func receiverName(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		return receiverName(typed.X)
	case *ast.IndexExpr:
		return receiverName(typed.X)
	case *ast.IndexListExpr:
		return receiverName(typed.X)
	case *ast.SelectorExpr:
		return receiverName(typed.X) + "." + typed.Sel.Name
	case *ast.ParenExpr:
		return receiverName(typed.X)
	default:
		return ""
	}
}
