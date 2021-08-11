package testdata

import "fmt"

type Child struct {
  Name string
}

func (c *Child) GetName() string{
  return c.Name
}

func main(){
  c := Child{}
  // The following line doesn't use a getter
  fmt.Printf("%s", c.Name)
}