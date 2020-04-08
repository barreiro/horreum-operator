package controller

import (
	"github.com/Hyperfoil/horreum-operator/pkg/controller/horreum"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, horreum.Add)
}
