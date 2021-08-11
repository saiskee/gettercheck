package src

import (
	. "github.com/saiskee/gettercheck/gettercheck/testdata/src/generated"
)

func main() {

p := Parent{Child: &Basic{Name: "hello"}}
_ = ((p.Child.Name))
}