// Package gettercheck is the library used to implement the gettercheck command-line tool.
package gettercheck

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/coreos/etcd/pkg/ioutil"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
	"regexp"
	"sort"
)

var errorType *types.Interface

func init() {
	errorType = types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
}

var (
	// ErrNoGoFiles is returned when CheckPackage is run on a package with no Go source files
	ErrNoGoFiles = errors.New("package contains no go source files")
)

// UnusedGetterError indicates the position of an unused protobuf getter.
type UnusedGetterError struct {
	//todo(sai): GetterPos
	Pos       token.Position
	GetterPos token.Position
	Line      string
	FuncName  string
}

// Result is returned from the CheckPackage function, and holds all the errors
// that were found to be unchecked in a package.
//
// Aggregation can be done using the Append method for users that want to
// combine results from multiple packages.
type Result struct {
	// UnusedGetterError is a list of all the unchecked errors in the package.
	// Printing an error reports its position within the file and the contents of the line.
	UnusedGetterError []UnusedGetterError
}

type byName []UnusedGetterError

// Less reports whether the element with index i should sort before the element with index j.
func (b byName) Less(i, j int) bool {
	ei, ej := b[i], b[j]

	pi, pj := ei.Pos, ej.Pos

	if pi.Filename != pj.Filename {
		return pi.Filename < pj.Filename
	}
	if pi.Line != pj.Line {
		return pi.Line < pj.Line
	}
	if pi.Column != pj.Column {
		return pi.Column < pj.Column
	}

	return ei.Line < ej.Line
}

func (b byName) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b byName) Len() int {
	return len(b)
}

// Append appends errors to e. Append does not do any duplicate checking.
func (r *Result) Append(other Result) {
	r.UnusedGetterError = append(r.UnusedGetterError, other.UnusedGetterError...)
}

// Returns the unique errors that have been accumulated. Duplicates may occur
// when a file containing an unchecked error belongs to > 1 package.
//
// The method receiver remains unmodified after the call to Unique.
func (r Result) Unique() Result {
	result := make([]UnusedGetterError, len(r.UnusedGetterError))
	copy(result, r.UnusedGetterError)
	sort.Sort((byName)(result))
	uniq := result[:0] // compact in-place
	for i, err := range result {
		if i == 0 || err != result[i-1] {
			uniq = append(uniq, err)
		}
	}
	return Result{UnusedGetterError: uniq}
}

// Exclusions define symbols and language elements that will be not checked
type Exclusions struct {

	// TestFiles excludes _test.go files.
	TestFiles bool

	// GeneratedFiles excludes generated source files.
	//
	// Source file is assumed to be generated if its contents
	// match the following regular expression:
	//
	//   ^// Code generated .* DO NOT EDIT\\.$
	//
	GeneratedFiles bool
}

// Checker checks that you checked errors.
type Checker struct {
	// Exclusions defines code packages, symbols, and other elements that will not be checked.
	Exclusions Exclusions

	WriteGetters bool

	// The mod flag for go build.
	Mod string
}

// loadPackages is used for testing.
var loadPackages = func(cfg *packages.Config, paths ...string) ([]*packages.Package, error) {
	return packages.Load(cfg, paths...)
}

// LoadPackages loads all the packages in all the paths provided. It uses the
// exclusions and build tags provided to by the user when loading the packages.
func (c *Checker) LoadPackages(paths ...string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode:  packages.LoadAllSyntax,
		Tests: !c.Exclusions.TestFiles,
	}
	return loadPackages(cfg, paths...)
}

var generatedCodeRegexp = regexp.MustCompile(`^//\s+Code generated.*DO NOT EDIT\.$`)
var dotStar = regexp.MustCompile(".*")

func (c *Checker) shouldSkipFile(file *ast.File) bool {
	if !c.Exclusions.GeneratedFiles {
		return false
	}

	for _, cg := range file.Comments {
		for _, comment := range cg.List {
			if generatedCodeRegexp.MatchString(comment.Text) {
				fmt.Println(comment.Text)
				return true
			}
		}
	}

	return false
}

// CheckPackage checks packages for errors that have not been checked.
//
// It will exclude specific errors from analysis if the user has configured
// exclusions.
func (c *Checker) CheckPackage(pkg *packages.Package) Result {

	v := &visitor{
		types:     pkg.Types,
		typesInfo: pkg.TypesInfo,
		fset:      pkg.Fset,
		imports:   pkg.Imports,
		lines:     make(map[string][]string),
		errors:    []UnusedGetterError{},
	}

	for _, astFile := range pkg.Syntax {
		if c.shouldSkipFile(astFile) {
			continue
		}

		newFile := astutil.Apply(astFile, v.Visit, nil)
		if c.WriteGetters {
			buf := &bytes.Buffer{}
			err := format.Node(buf, v.fset, newFile)
			if err != nil {
				panic(fmt.Errorf("error creating formatted code: %w", err))
			}
			fileName := v.fset.Position(astFile.Pos()).Filename
			err = ioutil.WriteAndSyncFile(fileName, buf.Bytes(), 0644)
			if err != nil {
				panic(err)
			}
		}
	}
	return Result{
		UnusedGetterError: v.errors,
	}
}
