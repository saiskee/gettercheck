package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

func main() {
	src := `
package p

type A struct {
 Name string
}

func (a *A) GetName() string {
 if a == nil {
   return ""
 }
 return a.Name
}
func main(){
a := &A{}

x := a.Name
x.C().B.A = "hi"
}
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		panic(err)
	}
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		//var s string
		switch n.(type) {
		case *ast.BasicLit:
			//s = x.Value
		case *ast.Ident:
			//s = x.Name
		default:
			fmt.Printf("deatils - %T : %+v\n", n, n)
		}

		//if s != "" {
		// fmt.Printf("%s (%T):\t%s\n", fset.Position(n.Pos()), n, s)
		//}else{
		//  s = fmt.Sprintf("%T",n)
		//  fmt.Printf("%s, %T\n",fset.Position(n.Pos()), n)
		//}

		return true
	})
}

//type A struct {
// Name string
//}
//
//func (a *A) GetName() string {
// if a == nil {
//   return ""
// }
// return a.Name
//}
//
//func main(){
// a := &A{}
// a = nil
// fmt.Println(a.Name)
//
//}
