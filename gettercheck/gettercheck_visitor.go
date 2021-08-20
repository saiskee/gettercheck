package gettercheck

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/ast/astutil"
	"os"
	"strings"

	"golang.org/x/tools/go/packages"
)

// visitor implements the gettercheck algorithm
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

// TODO (dtcaciuc) collect token.Pos and then convert them to UnusedGetterError
// after visitor is done running. This will allow to integrate more cleanly
// with analyzer so that we don't have to convert Position back to Pos.
func (v *visitor) addErrorAtPosition(position token.Pos, ident *ast.Ident, getterPos token.Position) {
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

	v.errors = append(v.errors, UnusedGetterError{
		pos,
		getterPos,
		line,
		name,
	})
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

func (v *visitor) Visit(c *astutil.Cursor) bool {
	node := c.Node()
	if node == nil {
		return false
	}
	switch n := node.(type) {
	case *ast.SelectorExpr:
		// this switch controls for special cases where we may
		// not want to use the getter
		switch p := c.Parent().(type) {
			case *ast.AssignStmt:
				// If the parent is an assignment statement, we only want to
				// report if it's not on the left hand side (e.g. we are not setting this value)
				for _, i := range p.Lhs{
					if n == i {
						return true
					}
				}
		case *ast.BinaryExpr:
			if p.Op == token.EQL {
				if i, ok := p.Y.(*ast.Ident); ok {
					if v.typesInfo.TypeOf(i).String() == "untyped nil" {
						if sel, ok := p.X.(*ast.SelectorExpr); ok {
							b := v.typesInfo.TypeOf(sel.Sel)
							basicPointerTypes := []string{"*string", "*int", "*bool", "*int", "*int32", "*int64", "*float32", "*float64", "*uint32", "*uint64"}
							if contains(basicPointerTypes, b.String()) {
								return true
							}
						}
					}
				}
			}
		case *ast.UnaryExpr:
			if p.Op == token.AND {
				return true
			}
		}

		obj := v.typesInfo.ObjectOf(n.Sel)
		switch obj.(type) {
		case *types.Var:
		default:
			return true
		}
		p := obj.Pos()
		f := v.fset.File(p)
		goPos := f.Position(p)
		// If the variable is from a `.pb.go` file, it has a getter
		// and the getter should be being used instead
		if strings.Contains(goPos.String(), ".pb.go:") {
			getter := fmt.Sprintf("Get%s", n.Sel.Name)
			typ := v.typesInfo.TypeOf(n.X)
			if method := FindMethod(typ, getter); method != nil {
				mPos := method.Pos()
				goMethodPos := v.fset.File(mPos).Position(mPos)
				n.Sel.Name = getter + "()"
				v.addErrorAtPosition(n.Sel.Pos(), n.Sel, goMethodPos)
				c.Replace(n)
			}
		}
		return true
	case *ast.KeyValueExpr:
		res := astutil.Apply(n.Value, v.Visit, nil)
		n.Value = res.(ast.Expr)
		c.Replace(n)
		return true
	case *ast.UnaryExpr:
		switch x := n.X.(type) {
		case *ast.SelectorExpr:
			res := astutil.Apply(x.X, v.Visit, nil)
			x.X = res.(ast.Expr)
			// todo: potential bug?
			c.Replace(n)
			return true
		}
		uV := &UnaryVisitor{
			typesInfo: v.typesInfo,
			fset:      v.fset,
		}
		res := astutil.Apply(n.X, uV.Visit, nil)
		n.X = res.(ast.Expr)
		c.Replace(n)
		return true
	}
	return true
}

func FindMethod(p types.Type, methodName string) *types.Func {
	switch typ := p.(type) {
	case *types.Pointer:
		return FindMethod(typ.Elem(), methodName)
	case *types.Named:
		for i := 0; i < typ.NumMethods(); i++ {
			method := typ.Method(i)
			if method.Name() == methodName {
				return method
			}
		}
		return nil
	}
	return nil
}

type UnaryVisitor struct {
	typesInfo *types.Info
	fset      *token.FileSet
}

func (v *UnaryVisitor) Visit(c *astutil.Cursor) bool {
	node := c.Node()
	if node == nil {
		return false
	}
	switch x := node.(type) {
	case *ast.ParenExpr:
		if sel, ok := x.X.(*ast.SelectorExpr); ok {
			rv := &visitor{
				typesInfo: v.typesInfo,
				fset:      v.fset,
			}
			res := astutil.Apply(sel.X, rv.Visit, nil)
			sel.X = res.(ast.Expr)
		}
		if sel, ok := x.X.(*ast.ParenExpr); ok {
			res := astutil.Apply(sel.X, v.Visit, nil)
			sel.X = res.(ast.Expr)
		}
	case *ast.SelectorExpr:
		res := astutil.Apply(x.X, v.Visit, nil)
		x.X = res.(ast.Expr)
	}
	c.Replace(node)
	return true
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
