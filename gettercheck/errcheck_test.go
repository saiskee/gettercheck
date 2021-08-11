package gettercheck_test

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/saiskee/gettercheck/gettercheck"
	"io/ioutil"
	"strings"
)

const testPackage = "github.com/saiskee/gettercheck/gettercheck/testdata/src"

var _ = FDescribe("gettercheck Suite Test", func() {
	var (
		checker *gettercheck.Checker
	)

	ExpectUnusedGetterResult := func(e ...UnusedGetterExpectation){
		pkgs, err := checker.LoadPackages(testPackage)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		ExpectWithOffset(1, pkgs).To(HaveLen(1))
		r := checker.CheckPackage(pkgs[0])
		ExpectWithOffset(1, r.UnusedGetterError).To(HaveLen(len(e)))
		for i, ugError := range r.UnusedGetterError {
			expectation := e[i]
			ExpectWithOffset(1,ugError.FuncName).To(Equal(expectation.ExpectedGetter))
			if expectation.ExpectedLinePos != ""{
				ExpectWithOffset(1, fmt.Sprintf("%d:%d", ugError.Pos.Line, ugError.Pos.Column)).
					To(Equal(expectation.ExpectedLinePos))
			}
		}
	}

	BeforeEach(func(){
		checker = &gettercheck.Checker{
			Exclusions: gettercheck.Exclusions{
				GeneratedFiles: true,
			},
		}
	})
			It("finds unused getters on RHS of assignment", func(){
				WriteTestFileBoostrap(`
	a := &Basic{}
	_, _ = a.Name, a.Name
`)

				pkgs, err := checker.LoadPackages(testPackage)
				Expect(err).NotTo(HaveOccurred())
				for _, pkg := range pkgs {
					result := checker.CheckPackage(pkg)
					Expect(result.UnusedGetterError).To(HaveLen(2))
				}
			})

	It("doesn't error for variables being assigned to", func(){
		WriteTestFileBoostrap(`
	a := &Basic{}
	b := &Basic{}
	a.Name, b.Name = "hello", "bye"
`)
		// no errors found
		ExpectUnusedGetterResult()
	})

	It("doesn't error for variables being assigned to", func(){
		WriteTestFileBoostrap(`
	a := &Parent{
		Child: &Basic{
			Name: "hello",
		},
	}

	a.Child.Name = "hello"
`)

		ExpectUnusedGetterResult(UnusedGetterExpectation{
			ExpectedGetter:  "GetChild()",
			ExpectedLinePos: "15:4",
		})
	})

	It("throws error in key value pair", func(){
		WriteTestFileBoostrap(`b := Basic{}
	_ = &Parent{
		Child: &Basic{
			Name: b.Name,
		},
	}`)
		ExpectUnusedGetterResult(UnusedGetterExpectation{
			ExpectedGetter:  "GetName()",
			ExpectedLinePos: "11:12",
		})

	})

	It("throws error when chained getters are needed", func(){
		WriteTestFileBoostrap(`
g := GrandParent{Child: &Parent{Child: &Basic{}}}
g.Child.Child.Name = "hello"
`)
		ExpectUnusedGetterResult(UnusedGetterExpectation{
			ExpectedGetter:  "GetChild()",
			ExpectedLinePos: "10:9",
		}, UnusedGetterExpectation{
			ExpectedGetter:  "GetChild()",
			ExpectedLinePos: "10:3",
		})

	})

	It("doesn't show error when no getter is available", func(){
		WriteTestFileBoostrap(`
	u := ChildNoGetter{Name: "hi"}
	_ = u.Name`)

		ExpectUnusedGetterResult()
	})

	It("doesn't have error when taking address of struct field", func(){
		WriteTestFileBoostrap(`	
	p := &Parent{Child: &Basic{}}
	_ = &p.GetChild().Name`)

		ExpectUnusedGetterResult()
	})

	It("doesn't show error when comparing a pointer of a basic golang variable to nil", func(){
		// In this case, c.Address is a string pointer, and there are cases where
		// We want to evaluate if the pointer is nil.
		// GetAddress will return the actual string, and not the pointer, so
		// we can't evaluate if it's nil, which is why this is ok
		WriteTestFileBoostrap(`
address := "Muffin Man Lane"
	c := Basic{Address: &address}
	if c.Address == nil {
	}`)

		ExpectUnusedGetterResult()
	})

	FIt("finds errors inside parentheses", func(){
		WriteTestFileBoostrap(`
p := Parent{Child: &Basic{Name: "hello"}}
_ = ((p.Child.Name))`)
		ExpectUnusedGetterResult(
			UnusedGetterExpectation{
				ExpectedGetter:  "GetName()",
				ExpectedLinePos: "10:15",
			},
			UnusedGetterExpectation{
				ExpectedGetter:  "GetChild()",
				ExpectedLinePos: "10:9",
			})

	})

})

type UnusedGetterExpectation struct {
	ExpectedGetter string
	// If 0, this is not checked
	ExpectedLinePos string
}

func WriteTestFileBoostrap(contents string){
	toWrite := fmt.Sprintf(`package src

import (
	. "github.com/saiskee/gettercheck/gettercheck/testdata/src/generated"
)

func main() {
%s
}`, contents)

WriteMain(toWrite)
}

func WriteMain(contents string){
	fileToWrite := strings.TrimSpace(contents)
	err := ioutil.WriteFile("testdata/src/main.go", []byte(fileToWrite), 0644)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}