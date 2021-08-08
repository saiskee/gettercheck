package main

import (
	"flag"
	"fmt"
	"github.com/saiskee/goprotogettercheck/errcheck"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
)

const (
	exitCodeOk int = iota
	exitUncheckedError
	exitFatalError
)

type ignoreFlag map[string]*regexp.Regexp

// global flags
var (
	abspath bool
	verbose bool
)

func (f ignoreFlag) String() string {
	pairs := make([]string, 0, len(f))
	for pkg, re := range f {
		prefix := ""
		if pkg != "" {
			prefix = pkg + ":"
		}
		pairs = append(pairs, prefix+re.String())
	}
	return fmt.Sprintf("%q", strings.Join(pairs, ","))
}

func (f ignoreFlag) Set(s string) error {
	if s == "" {
		return nil
	}
	for _, pair := range strings.Split(s, ",") {
		colonIndex := strings.Index(pair, ":")
		var pkg, re string
		if colonIndex == -1 {
			pkg = ""
			re = pair
		} else {
			pkg = pair[:colonIndex]
			re = pair[colonIndex+1:]
		}
		regex, err := regexp.Compile(re)
		if err != nil {
			return err
		}
		f[pkg] = regex
	}
	return nil
}

type tagsFlag []string

func (f *tagsFlag) String() string {
	return fmt.Sprintf("%q", strings.Join(*f, ","))
}

func (f *tagsFlag) Set(s string) error {
	if s == "" {
		return nil
	}
	tags := strings.FieldsFunc(s, func(c rune) bool {
		return c == ' ' || c == ','
	})
	for _, tag := range tags {
		if tag != "" {
			*f = append(*f, tag)
		}
	}
	return nil
}

func reportResult(e errcheck.Result) {
	wd, err := os.Getwd()
	if err != nil {
		wd = ""
	}
	for _, unusedGetterError := range e.UnusedGetterError {
		pos := unusedGetterError.Pos.String()
		if !abspath {
			newPos, err := filepath.Rel(wd, pos)
			if err == nil {
				pos = newPos
			}
		}
		// Print result to stdout
		fmt.Printf("%s:\t%s\t%s\n\tGetter at %s\n", pos, unusedGetterError.FuncName, unusedGetterError.Line, unusedGetterError.GetterPos.String())
	}
}

func logf(msg string, args ...interface{}) {
	if verbose {
		fmt.Fprintf(os.Stderr, msg+"\n", args...)
	}
}

func mainCmd(args []string) int {
	var checker errcheck.Checker
	paths, rc := parseFlags(&checker, args)
	if rc != exitCodeOk {
		return rc
	}
	// Check paths
	result, err := checkPaths(&checker, paths...)
	if err != nil {
		if err == errcheck.ErrNoGoFiles {
			fmt.Fprintln(os.Stderr, err)
			return exitCodeOk
		}
		fmt.Fprintf(os.Stderr, "error: failed to check packages: %s\n", err)
		return exitFatalError
	}
	// Report unused getter error if errors are found
	if len(result.UnusedGetterError) > 0 {
		reportResult(result)
		return exitUncheckedError
	}
	return exitCodeOk
}

func checkPaths(c *errcheck.Checker, paths ...string) (errcheck.Result, error) {
	pkgs, err := c.LoadPackages(paths...)
	if err != nil {
		return errcheck.Result{}, err
	}
	// Check for errors in the initial packages.
	work := make(chan *packages.Package, len(pkgs))
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return errcheck.Result{}, fmt.Errorf("errors while loading package %s: %v", pkg.ID, pkg.Errors)
		}
		work <- pkg
	}
	close(work)

	var wg sync.WaitGroup
	result := &errcheck.Result{}
	mu := &sync.Mutex{}
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			for pkg := range work {
				logf("checking %s", pkg.Types.Path())
				r := c.CheckPackage(pkg)
				mu.Lock()
				result.Append(r)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	return result.Unique(), nil
}

func parseFlags(checker *errcheck.Checker, args []string) ([]string, int) {
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)

	flags.BoolVar(&checker.Exclusions.TestFiles, "ignoretests", false, "if true, checking of _test.go files is disabled")
	flags.BoolVar(&checker.Exclusions.GeneratedFiles, "ignoregenerated", false, "if true, checking of files with generated code is disabled")
	flags.BoolVar(&verbose, "verbose", false, "produce more verbose logging")
	flags.BoolVar(&abspath, "abspath", false, "print absolute paths to files")


	flags.StringVar(&checker.Mod, "mod", "", "module download mode to use: readonly or vendor. See 'go help modules' for more.")

	if err := flags.Parse(args[1:]); err != nil {
		return nil, exitFatalError
	}


	paths := flags.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}

	return paths, exitCodeOk
}

func main() {
	os.Exit(mainCmd(os.Args))
}
