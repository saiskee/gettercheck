package gettercheck

import (
	"go/token"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/astutil"
	"reflect"
)

var Analyzer = &analysis.Analyzer{
	Name:       "gettercheck",
	Doc:        "check for unused getters",
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

		astutil.Apply(f, v.Visit,nil)
		//ast.Walk(v, f)

		for _, err := range v.errors {
			pass.Report(analysis.Diagnostic{
				Pos:     token.Pos(int(f.Pos()) + err.Pos.Offset),
				Message: "unused getter",
			})
		}

		allErrors = append(allErrors, v.errors...)
	}

	return Result{UnusedGetterError: allErrors}, nil
}
