package knobs

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	counter  atomic.Int32
	registry map[int]*state
	regMux   sync.RWMutex
	regOnce  sync.Once
)

type state struct {
	sync.RWMutex
	once       sync.Once
	current    any
	initialize initializer
	origins    map[Origin]struct{}
	origin     Origin // Last origin
	parent     int
}

type initializer func(*state)

// The following variables are transform functions for converting string values to other basic data types
// Their purpose is to make creating non-string EnvVars easier and cleaner, e.g. NewEnvVar("MY_VAR", ToInt)
var (
	ToInt = func(s string) (int, error) {
		// TODO: Determine whether we want to accept floats into ints with this function; currently fails on input like "1.0"
		return strconv.Atoi(s)
	}
	ToFloat64 = func(s string) (float64, error) {
		return strconv.ParseFloat(s, 64)
	}
	ToBool = func(s string) (bool, error) {
		return strconv.ParseBool(s)
	}
	// ToString is mainly for documentation purposes
	ToString = func(s string) (string, error) {
		return s, nil
	}
)

// Definition declares how a configuration is sourced.
type Definition[T any] struct {
	Default T
	Origins []Origin // Default and Env origins are implicit
	EnvVars []EnvVar
	Clean   func(string) (T, error)
}

func (def *Definition[T]) initializer(s *state) {
	var v string
	for _, e := range def.EnvVars {
		if v = e.getValue(); v != "" {
			break
		}
	}
	if v == "" {
		s.current = def.Default
		return
	}
	s.origin = Env
	if def.Clean == nil {
		panic("knobs: Clean function is required for environment variables")
	}
	if final, err := def.Clean(v); err == nil {
		s.current = final
		return
	} else {
		fmt.Printf("Error cleaning variable: %v\n", err)
	}
	s.current = def.Default
}

// Origin defines a known configuration source.
// It's used to track where the configuration value comes from and
// self-document the code. Library users can define their own origins.
type Origin int

const (
	// Default is the default configuration source.
	Default Origin = iota
	// Env is the environment variable configuration source.
	Env
	// Code is the code configuration source.
	Code
)

// Knob defines an available configuration.
type Knob[T any] int

// Register adds a new configuration to the registry.
// It returns a Knob that can be used to retrieve the configuration value.
// It's not idempotent, so calling it multiple times with the same Definition will create multiple Knobs.
func Register[T any](def *Definition[T]) Knob[T] {
	var (
		k       = int(counter.Add(1))
		origins = make(map[Origin]struct{}, len(def.Origins))
	)
	for _, o := range def.Origins {
		origins[o] = struct{}{}
	}
	regOnce.Do(func() {
		registry = make(map[int]*state)
	})
	s := &state{
		initialize: def.initializer,
		origins:    origins,
	}
	setState(k, s)
	knob := Knob[T](k)
	runtime.SetFinalizer(&knob, func(k *Knob[T]) {
		// This keeps the registry clean.
		deleteState(int(*k))
	})
	return knob
}

// Derive creates a new configuration based on a parent knob.
// Derive returns a Knob initialized with the parent value, which can either be kept or overwritten with a new value.
// The parent knob can be another derived knob.
// It's not idempotent, so calling it multiple times with the same parent will create multiple Knobs.
func Derive[T any](parent Knob[T]) Knob[T] {
	dk := int(counter.Add(1))
	s := &state{
		parent: int(parent),
	}
	setState(dk, s)
	return Knob[T](dk)
}

// Get retrieves the current configuration value.
func Get[T any](kn Knob[T]) T {
	var (
		zero T
		s    = getState(int(kn))
	)
	if s == nil {
		// This shouldn't happen, but we fail graciously by returning
		// the zero value.
		return zero
	}
	s.RLock()
	defer s.RUnlock()

	if s.current != nil {
		return s.current.(T)
	}
	if s.initialize != nil {
		s.once.Do(func() { s.initialize(s) })
	}
	if s.parent > 0 {
		return Get(Knob[T](s.parent))
	}
	return s.current.(T)
}

func getState(kn int) *state {
	regMux.RLock()
	s := registry[kn]
	regMux.RUnlock()
	return s
}

func setState(kn int, s *state) {
	regMux.Lock()
	registry[kn] = s
	regMux.Unlock()
}

func deleteState(kn int) {
	regMux.Lock()
	delete(registry, kn)
	regMux.Unlock()
}

func Set[T any](kn Knob[T], value T, origin Origin) {
	k := int(kn)
	s := getState(k)
	if s == nil {
		return
	}
	s.Lock()
	defer s.Unlock()

	if origin != Code {
		if _, ok := s.origins[origin]; !ok {
			return
		}
	}
	s.origin = origin
	s.current = value
}
