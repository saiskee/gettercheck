package errcheck

import (
	"go/ast"
	"go/token"
	"reflect"

	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name:       "errcheck",
	Doc:        "check for unchecked errors",
	Run:        runAnalyzer,
	ResultType: reflect.TypeOf(Result{}),
}

func init() {
}

func runAnalyzer(pass *analysis.Pass) (interface{}, error) {

	var allErrors []UnusedGetterError
	for _, f := range pass.Files {
		v := &visitor{
			typesInfo: pass.TypesInfo,
			fset:      pass.Fset,
			lines:     make(map[string][]string),
			errors:    nil,
		}

		ast.Walk(v, f)

		for _, err := range v.errors {
			pass.Report(analysis.Diagnostic{
				Pos:     token.Pos(int(f.Pos()) + err.Pos.Offset),
				Message: "unused protobuf getter",
			})
		}

		allErrors = append(allErrors, v.errors...)
	}

	return Result{UnusedGetterError: allErrors}, nil
}
