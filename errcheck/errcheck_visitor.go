package errcheck

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"strings"

	"golang.org/x/tools/go/packages"
)

// visitor implements the errcheck algorithm
type visitor struct {
	types     *types.Package
	typesInfo *types.Info
	fset      *token.FileSet
	lines     map[string][]string

	errors  []UnusedGetterError
	imports map[string]*packages.Package
}

// selectorAndFunc tries to get the selector and function from call expression.
// For example, given the call expression representing "a.b()", the selector
// is "a.b" and the function is "b" itself.
//
// The final return value will be true if it is able to do extract a selector
// from the call and look up the function object it refers to.
//
// If the call does not include a selector (like if it is a plain "f()" function call)
// then the final return value will be false.
func (v *visitor) selectorAndFunc(call *ast.CallExpr) (*ast.SelectorExpr, *types.Func, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, nil, false
	}

	fn, ok := v.typesInfo.ObjectOf(sel.Sel).(*types.Func)
	if !ok {
		// Shouldn't happen, but be paranoid
		return nil, nil, false
	}

	return sel, fn, true

}

// fullName will return a package / receiver-type qualified name for a called function
// if the function is the result of a selector. Otherwise it will return
// the empty string.
//
// The name is fully qualified by the import path, possible type,
// function/method name and pointer receiver.
//
// For example,
//   - for "fmt.Printf(...)" it will return "fmt.Printf"
//   - for "base64.StdEncoding.Decode(...)" it will return "(*encoding/base64.Encoding).Decode"
//   - for "myFunc()" it will return ""
func (v *visitor) fullName(call *ast.CallExpr) string {
	_, fn, ok := v.selectorAndFunc(call)
	if !ok {
		return ""
	}

	// TODO(dh): vendored packages will have /vendor/ in their name,
	// thus not matching vendored standard library packages. If we
	// want to support vendored stdlib packages, we need to implement
	// FullName with our own logic.
	return fn.FullName()
}

func getSelectorName(sel *ast.SelectorExpr) string {
	if ident, ok := sel.X.(*ast.Ident); ok {
		return fmt.Sprintf("%s.%s", ident.Name, sel.Sel.Name)
	}
	if s, ok := sel.X.(*ast.SelectorExpr); ok {
		return fmt.Sprintf("%s.%s", getSelectorName(s), sel.Sel.Name)
	}

	return ""
}

// selectorName will return a name for a called function
// if the function is the result of a selector. Otherwise it will return
// the empty string.
//
// The name is fully qualified by the import path, possible type,
// function/method name and pointer receiver.
//
// For example,
//   - for "fmt.Printf(...)" it will return "fmt.Printf"
//   - for "base64.StdEncoding.Decode(...)" it will return "base64.StdEncoding.Decode"
//   - for "myFunc()" it will return ""
func (v *visitor) selectorName(call *ast.CallExpr) string {
	sel, _, ok := v.selectorAndFunc(call)
	if !ok {
		return ""
	}

	return getSelectorName(sel)
}

// namesForExcludeCheck will return a list of fully-qualified function names
// from a function call that can be used to check against the exclusion list.
//
// If a function call is against a local function (like "myFunc()") then no
// names are returned. If the function is package-qualified (like "fmt.Printf()")
// then just that function's fullName is returned.
//
// Otherwise, we walk through all the potentially embeddded interfaces of the receiver
// the collect a list of type-qualified function names that we will check.
func (v *visitor) namesForExcludeCheck(call *ast.CallExpr) []string {
	sel, fn, ok := v.selectorAndFunc(call)
	if !ok {
		return nil
	}

	name := v.fullName(call)
	if name == "" {
		return nil
	}

	// This will be missing for functions without a receiver (like fmt.Printf),
	// so just fall back to the the function's fullName in that case.
	selection, ok := v.typesInfo.Selections[sel]
	if !ok {
		return []string{name}
	}

	// This will return with ok false if the function isn't defined
	// on an interface, so just fall back to the fullName.
	ts, ok := walkThroughEmbeddedInterfaces(selection)
	if !ok {
		return []string{name}
	}

	result := make([]string, len(ts))
	for i, t := range ts {
		// Like in fullName, vendored packages will have /vendor/ in their name,
		// thus not matching vendored standard library packages. If we
		// want to support vendored stdlib packages, we need to implement
		// additional logic here.
		result[i] = fmt.Sprintf("(%s).%s", t.String(), fn.Name())
	}
	return result
}

// isBufferType checks if the expression type is a known in-memory buffer type.
func (v *visitor) argName(expr ast.Expr) string {
	// Special-case literal "os.Stdout" and "os.Stderr"
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		if obj := v.typesInfo.ObjectOf(sel.Sel); obj != nil {
			vr, ok := obj.(*types.Var)
			if ok && vr.Pkg() != nil && vr.Pkg().Name() == "os" && (vr.Name() == "Stderr" || vr.Name() == "Stdout") {
				return "os." + vr.Name()
			}
		}
	}
	t := v.typesInfo.TypeOf(expr)
	if t == nil {
		return ""
	}
	return t.String()
}

// nonVendoredPkgPath returns the unvendored version of the provided package
// path (or returns the provided path if it does not represent a vendored
// path).
func nonVendoredPkgPath(pkgPath string) string {
	lastVendorIndex := strings.LastIndex(pkgPath, "/vendor/")
	if lastVendorIndex == -1 {
		return pkgPath
	}
	return pkgPath[lastVendorIndex+len("/vendor/"):]
}

// TODO (dtcaciuc) collect token.Pos and then convert them to UnusedGetterError
// after visitor is done running. This will allow to integrate more cleanly
// with analyzer so that we don't have to convert Position back to Pos.
func (v *visitor) addErrorAtPosition(position token.Pos, ident *ast.Ident) {
	pos := v.fset.Position(position)
	lines, ok := v.lines[pos.Filename]
	if !ok {
		lines = readfile(pos.Filename)
		v.lines[pos.Filename] = lines
	}

	line := "??"
	if pos.Line-1 < len(lines) {
		line = strings.TrimSpace(lines[pos.Line-1])
	}

	var name = ident.Name

	v.errors = append(v.errors, UnusedGetterError{pos, line, name, name})
}

func readfile(filename string) []string {
	var f, err = os.Open(filename)
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []string
	var scanner = bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

func (v *visitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}
	switch n := node.(type) {
	case *ast.SelectorExpr:
		obj := v.typesInfo.ObjectOf(n.Sel)
		switch obj.(type) {
		case *types.Var:
		default:
			return v
		}
		p := obj.Pos()
		f := v.fset.File(p)
		goPos := f.Position(p)
		// If the variable is from a `.pb.go` file, it has a getter
		// and the getter should be being used instead
		if strings.Contains(goPos.String(), ".pb.go:") {
			getter := fmt.Sprintf("Get%s", n.Sel.Name)
			typ := v.typesInfo.TypeOf(n.X)
			if method := FindMethod(typ, getter); method != nil{
				mPos := method.Pos()
				goMethodPos := v.fset.File(mPos).Position(mPos)
				fmt.Println(getter, goMethodPos.String())
				v.addErrorAtPosition(n.Sel.Pos(), n.Sel)
			}
		}

	case *ast.KeyValueExpr:
		ast.Walk(v, n.Value)
		return nil
	case *ast.UnaryExpr:
		switch x := n.X.(type) {
		case *ast.SelectorExpr:
			ast.Walk(v, x.X)
			return nil
		}
		return &UnaryVisitor{
			typesInfo: v.typesInfo,
			fset:      v.fset,
		}
	case *ast.AssignStmt:
		for i := 0; i < len(n.Rhs); i++ {
			ast.Walk(v, n.Rhs[i])
		}
		for i := 0; i < len(n.Lhs); i++ {
			lNode := n.Lhs[i]
			switch x := lNode.(type) {
			case *ast.SelectorExpr:
				ast.Walk(v, x.X)
			}
		}
		return nil
	//case *ast.Ident:
	//	//fmt.Printf("Visiting ident -  %s\n", n.Name)
	//	obj := v.typesInfo.ObjectOf(n)
	//	switch obj.(type) {
	//	case *types.Var:
	//	default:
	//		return v
	//	}
	//	p := obj.Pos()
	//	f := v.fset.File(p)
	//	goPos := f.Position(p)
	//	// If the variable is from a `.pb.go` file, it has a getter
	//	// and the getter should be being used instead
	//	if strings.Contains(goPos.String(), ".pb.go:") {
	//		v.addErrorAtPosition(n.Pos(), n)
	//	}
	//	return nil
	}
	return v
}

func FindMethod(p types.Type, methodName string)*types.Func{
	switch typ := p.(type){
	case *types.Pointer:
		return FindMethod(typ.Elem(), methodName)
	case *types.Named:
		for i := 0; i < typ.NumMethods(); i++{
			method := typ.Method(i)
			if method.Name() == methodName{
				return method
			}
		}
		return nil
	}
	return nil
}


func print(f interface{}){
	fmt.Printf("debug - %+v\n", f)
}

type UnaryVisitor struct {
	typesInfo *types.Info
	fset      *token.FileSet
}

func (v *UnaryVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}
	switch x := node.(type) {
	case *ast.ParenExpr:
		if sel, ok := x.X.(*ast.SelectorExpr); ok {
			ast.Walk(&visitor{
				typesInfo: v.typesInfo,
				fset:      v.fset,
			}, sel.X)
		}
		if sel, ok := x.X.(*ast.ParenExpr); ok {
			ast.Walk(v, sel.X)
		}
	case *ast.SelectorExpr:
		ast.Walk(v, x.X)
	}
	return v
}
