// Package errcheck is the library used to implement the errcheck command-line tool.
package errcheck

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"sort"

	"golang.org/x/tools/go/packages"
)

var errorType *types.Interface

func init() {
	errorType = types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
}

var (
	// ErrNoGoFiles is returned when CheckPackage is run on a package with no Go source files
	ErrNoGoFiles = errors.New("package contains no go source files")
)

// UnusedGetterError indicates the position of an unchecked error return.
type UnusedGetterError struct {
	Pos          token.Position
	Line         string
	FuncName     string
	SelectorName string
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

	// Packages lists paths of excluded packages.
	Packages []string

	// SymbolRegexpsByPackage maps individual package paths to regular
	// expressions that match symbols to be excluded.
	//
	// Packages whose paths appear both here and in Packages list will
	// be excluded entirely.
	//
	// This is a legacy input that will be deprecated in errcheck version 2 and
	// should not be used.
	SymbolRegexpsByPackage map[string]*regexp.Regexp

	// Symbols lists patterns that exclude individual package symbols.
	//
	// For example:
	//
	//   "fmt.Errorf"              // function
	//   "fmt.Fprintf(os.Stderr)"  // function with set argument value
	//   "(hash.Hash).Write"       // method
	//
	Symbols []string

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

	// BlankAssignments ignores assignments to blank identifier.
	BlankAssignments bool

	// TypeAssertions ignores unchecked type assertions.
	TypeAssertions bool
}

// Checker checks that you checked errors.
type Checker struct {
	// Exclusions defines code packages, symbols, and other elements that will not be checked.
	Exclusions Exclusions

	// Tags are a list of build tags to use.
	Tags []string

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
	buildFlags := []string{fmtTags(c.Tags)}
	if c.Mod != "" {
		buildFlags = append(buildFlags, fmt.Sprintf("-mod=%s", c.Mod))
	}
	cfg := &packages.Config{
		Mode:       packages.LoadAllSyntax,
		Tests:      !c.Exclusions.TestFiles,
		BuildFlags: buildFlags,
	}
	return loadPackages(cfg, paths...)
}

var generatedCodeRegexp = regexp.MustCompile("^// Code generated .* DO NOT EDIT\\.$")
var dotStar = regexp.MustCompile(".*")

func (c *Checker) shouldSkipFile(file *ast.File) bool {
	if !c.Exclusions.GeneratedFiles {
		return false
	}

	for _, cg := range file.Comments {
		for _, comment := range cg.List {
			if generatedCodeRegexp.MatchString(comment.Text) {
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
	excludedSymbols := map[string]bool{}
	for _, sym := range c.Exclusions.Symbols {
		excludedSymbols[sym] = true
	}

	ignore := map[string]*regexp.Regexp{}
	// Apply SymbolRegexpsByPackage first so that if the same path appears in
	// Packages, a more narrow regexp will be superceded by dotStar below.
	if regexps := c.Exclusions.SymbolRegexpsByPackage; regexps != nil {
		for pkg, re := range regexps {
			// TODO warn if previous entry overwritten?
			ignore[nonVendoredPkgPath(pkg)] = re
		}
	}
	for _, pkg := range c.Exclusions.Packages {
		// TODO warn if previous entry overwritten?
		ignore[nonVendoredPkgPath(pkg)] = dotStar
	}

	v := &visitor{
		types:     pkg.Types,
		typesInfo: pkg.TypesInfo,
		fset:      pkg.Fset,
		imports:   pkg.Imports,
		ignore:    ignore,
		lines:     make(map[string][]string),
		exclude:   excludedSymbols,
		errors:    []UnusedGetterError{},
	}

	for _, astFile := range pkg.Syntax {
		if c.shouldSkipFile(astFile) {
			continue
		}
		ast.Walk(v, astFile)
	}
	return Result{UnusedGetterError: v.errors}
}
