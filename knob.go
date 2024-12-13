package knobs

import (
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
)

// Global variables
var (
	counter  atomic.Int32
	regMux   sync.RWMutex
	registry map[int]*definition
	regOnce  sync.Once // Ensures the registry is created only once
)

var (
	// ErrInvalidValue is returned when the value cannot be converted to the expected type.
	// This error is useful when the Clean function fails to convert the value.
	ErrInvalidValue = errors.New("knobs: invalid value")
)

// definition is an internal representation of a configuration definition. See Definition.
type definition struct {
	def     any
	init    initializer
	origins map[Origin]struct{}
}

// state is an instance of a configuration definition.
type state struct {
	sync.RWMutex
	*definition

	once    sync.Once
	current any
	origin  Origin // Last origin
	parent  int
}

func (s *state) init() {
	if s.definition == nil {
		// Knob is a derived knob and has no definition.
		return
	}
	s.once.Do(func() { s.definition.init(s) })
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
	Default  T
	Origins  []Origin // Default and Env origins are implicit
	EnvVars  []EnvVar
	Requires []any                   // Knobs that must be set to a non-zero value before this one; used only for documentation purposes
	Parse    func(string) (T, error) // Parse converts a string to the expected type; ignores the returned value if an error is returned
}

func (def *Definition[T]) initializer(s *state) {
	s.current = def.Default
	var (
		v string
		e EnvVar
	)
	for _, e = range def.EnvVars {
		if v = e.getValue(); v != "" {
			break
		}
	}
	if v == "" {
		return
	}
	s.origin = Env
	if def.Parse == nil {
		logFn("knobs: missing Parse function for environment variable %q", e.Key)
		return
	}
	if final, err := def.Parse(v); err == nil {
		s.current = final
		return
	} else {
		logFn("%s", err.Error())
	}
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

// Register adds a new configuration to the default scope.
// Register returns a Knob that can be used to retrieve the configuration value. A Knob can be used in multiple scopes.
// Register is not idempotent, so calling it multiple times with the same Definition will create multiple Knobs.
func Register[T any](def *Definition[T]) Knob[T] {
	var (
		k       = int(counter.Add(1))
		origins = make(map[Origin]struct{}, len(def.Origins))
	)
	for _, o := range def.Origins {
		origins[o] = struct{}{}
	}
	d := &definition{
		def:     def.Default,
		init:    def.initializer,
		origins: origins,
	}
	regMux.Lock()
	defer regMux.Unlock()

	regOnce.Do(func() {
		registry = make(map[int]*definition)
	})

	registry[k] = d
	return Knob[T](k)
}

// Derive creates a new configuration based on a parent Knob from the default scope.
// Derive returns a Knob initialized with the parent value, which can either be kept or overwritten with a new value.
// The parent Knob can be another derived Knob.
// Derive is not idempotent, so calling it multiple times with the same parent will create multiple Knobs.
func Derive[T any](parent Knob[T]) Knob[T] {
	return DeriveScope(DefaultScope(), parent)
}

// DeriveScope creates a new configuration based on a parent Knob from a specific scope.
// DeriveScope returns a Knob initialized with the parent value, which can either be kept or overwritten with a new value.
// The parent Knob can be another derived Knob.
// DeriveScope is not idempotent, so calling it multiple times with the same parent will create multiple Knobs.
func DeriveScope[T any](sc *Scope, parent Knob[T]) Knob[T] {
	dk := int(counter.Add(1))
	s := &state{
		// Derived Knobs fall back to their parent's value if they don't have their own.
		parent: int(parent),
	}
	sc.set(dk, s)
	return Knob[T](dk)
}

// Get retrieves the current configuration value from the default scope.
func Get[T any](kn Knob[T]) T {
	return GetScope(DefaultScope(), kn)
}

// GetScope retrieves the current configuration value from a specific scope.
func GetScope[T any](sc *Scope, kn Knob[T]) T {
	k := int(kn)
	s := sc.get(k)
	if s == nil {
		// This shouldn't happen, but we fail graciously by returning
		// the zero value.
		var zero T
		return zero
	}
	s.RLock()
	defer s.RUnlock()

	if s.current != nil {
		return s.current.(T)
	}
	if s.parent > 0 {
		return GetScope(sc, Knob[T](s.parent))
	}
	return s.current.(T)
}

// Set sets value for a new configuration value to the default scope.
func Set[T any](kn Knob[T], origin Origin, value T) {
	SetScope(DefaultScope(), kn, origin, value)
}

// SetScope sets value for a new configuration value to a specific scope.
func SetScope[T any](sc *Scope, kn Knob[T], origin Origin, value T) {
	s := sc.get(int(kn))
	if s == nil {
		// This shouldn't happen.
		return
	}
	s.Lock()
	defer s.Unlock()

	if origin != Code {
		if _, ok := s.origins[origin]; !ok {
			// Update from this origin is not allowed.
			return
		}
	}
	s.origin = origin
	s.current = value
}
