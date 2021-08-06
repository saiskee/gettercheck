package errcheck

import (
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"regexp"

	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name:       "errcheck",
	Doc:        "check for unchecked errors",
	Run:        runAnalyzer,
	ResultType: reflect.TypeOf(Result{}),
}

var (
	argBlank       bool
	argAsserts     bool
	argExcludeFile string
	argExcludeOnly bool
)

func init() {
	Analyzer.Flags.BoolVar(&argBlank, "blank", false, "if true, check for errors assigned to blank identifier")
}

func runAnalyzer(pass *analysis.Pass) (interface{}, error) {

	exclude := map[string]bool{}
	if !argExcludeOnly {
		for _, name := range DefaultExcludedSymbols {
			exclude[name] = true
		}
	}
	if argExcludeFile != "" {
		excludes, err := ReadExcludes(argExcludeFile)
		if err != nil {
			return nil, fmt.Errorf("Could not read exclude file: %v\n", err)
		}
		for _, name := range excludes {
			exclude[name] = true
		}
	}

	var allErrors []UnusedGetterError
	for _, f := range pass.Files {
		v := &visitor{
			typesInfo: pass.TypesInfo,
			fset:      pass.Fset,
			exclude:   exclude,
			ignore:    map[string]*regexp.Regexp{}, // deprecated & not used
			lines:     make(map[string][]string),
			errors:    nil,
		}

		ast.Walk(v, f)

		for _, err := range v.errors {
			pass.Report(analysis.Diagnostic{
				Pos:     token.Pos(int(f.Pos()) + err.Pos.Offset),
				Message: "unchecked error",
			})
		}

		allErrors = append(allErrors, v.errors...)
	}

	return Result{UnusedGetterError: allErrors}, nil
}
