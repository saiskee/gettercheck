# gettercheck

gettercheck is a program for checking for unused getters in Golang programs.

## Install

    go get -u github.com/saiskee/gettercheck

gettercheck requires Go 1.12 or newer.

## Use

For basic usage, just give the package path of interest as the first argument:

    gettercheck github.com/saiskee/gettercheck/testdata

To check all packages beneath the current directory:

    gettercheck ./...

Or check all packages in your $GOPATH and $GOROOT:

    gettercheck all

gettercheck also recognizes the following command-line options:


`-ignoregenerated`: This will ignore any files that are generated, e.g. any files
that have a line that matches the regex `^//\s+Code generated.*DO NOT EDIT\.$`.

`-ignoretests`: This will ignore any test files, or any files that end with `_test.go`.

`-verbose`: Will print a more verbose message on unused getters that are found. This will include
the source file of the unused getter.

### go/analysis

The package provides `Analyzer` instance that can be used with
[go/analysis](https://pkg.go.dev/golang.org/x/tools/go/analysis) API.

Just as the API itself, the analyzer is exprimental and may change in the
future.

## Exit Codes

gettercheck returns 1 if any problems were found in the checked files.
It returns 2 if there were any other failures.