package knobs_test

import (
	"fmt"

	"github.com/darccio/knobs"
)

func Example() {
	// 1. Define your configuration knob
	def := &knobs.Definition[string]{
		Default: "default",
	}
	// 2. Register it
	knob := knobs.Register(def)
	// 3. Retrieve the value, resolved on first access
	value := knobs.Get(knob)
	fmt.Println(value)
	// Output: default
}

func ExampleDerive() {
	// 1. Given an already existing configuration knob
	def := &knobs.Definition[string]{
		Default: "default",
	}
	knob := knobs.Register(def)
	// 2. Define a derived configuration knob that overrides the parent
	derived := knobs.Derive(knob)
	knobs.Set(derived, knobs.Code, "overridden")
	// 3. Retrieve the value, resolved on first access
	value := knobs.Get(derived)
	fmt.Println(value)
	// Output: overridden
}
