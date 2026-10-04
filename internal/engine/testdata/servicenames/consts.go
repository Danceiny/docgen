package servicenames

import "fmt"

// A constant of another file is the name of a service.
const fromAnotherFile = "stock"

// A name made of constants.
const prefix = "store"

var notConstant = "var"

func name() string { return fmt.Sprintf("%s", "x") }
