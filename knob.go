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
	// This error is useful when the Parse function fails to convert the value.
	ErrInvalidValue = errors.New("invalid value")
)

// definition is an internal representation of a configuration definition. See Definition.
type definition struct {
	def     any
	init    initializer
	origins map[Origin]struct{}
}

// slot holds a knob's current value together with its provenance and
// whether it has ever been set at all, grouped so all three move together
// under a single lock acquisition.
type slot struct {
	v        any
	origin   Origin
	hasValue bool // distinguishes "unset" from "set to a nil interface value"
}

// state is an instance of a configuration definition.
type state struct {
	mu sync.RWMutex
	*definition

	once   sync.Once
	val    slot
	parent int
}

func (s *state) init() {
	if s.definition == nil {
		// Knob is a derived knob and has no definition.
		return
	}
	s.once.Do(func() { s.definition.init(s) })
}

// warnedKnobs suppresses repeat degraded-read logs, keyed by knob id, so a
// misused knob logs once instead of flooding a hot Get loop.
var warnedKnobs sync.Map

func warnOnce(id int, format string, args ...any) {
	if _, already := warnedKnobs.LoadOrStore(id, struct{}{}); !already {
		logf(format, args...)
	}
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
	// Origins lists the origins, in addition to Code (always allowed), that may
	// write this knob via Set/SetScope. Default is never a valid Set origin: it
	// represents this knob's own baseline, not an external source that can push
	// a value to it. Listing Env here only affects external Set(kn, Env, v)
	// calls — it has no effect on initialization from EnvVars, which always
	// applies regardless of what's listed here.
	Origins  []Origin
	EnvVars  []EnvVar
	Requires []any // Knobs that must be set to a non-zero value before this one; used only for documentation purposes
	// Resolve handles validation and conditional behavior.
	// It must not call Get/GetScope/Set/SetScope on the knob currently being initialized: each
	// knob's initialization is guarded by a non-reentrant sync.Once, so a same-knob call from
	// within Resolve deadlocks. Calls to other, already-registered knobs are safe.
	Resolve func(environ map[string]string, decision string) (string, error)
	// Parse converts a string to the expected type; ignores the returned value if an error is returned.
	// It must not call Get/GetScope/Set/SetScope on the knob currently being initialized: each
	// knob's initialization is guarded by a non-reentrant sync.Once, so a same-knob call from
	// within Parse deadlocks. Calls to other, already-registered knobs are safe.
	Parse func(string) (T, error)
}

func (def *Definition[T]) initializer(s *state) {
	s.mu.Lock()
	s.val = slot{v: def.Default, hasValue: true}
	s.mu.Unlock()
	if len(def.EnvVars) == 0 {
		return
	}
	var (
		current = ""
		environ = make(map[string]string, len(def.EnvVars))
	)
	for _, e := range def.EnvVars {
		v := e.getValue()
		if v == "" {
			continue
		}
		environ[e.Key] = v
		if len(environ) == 1 {
			current = e.Key
		}
	}
	if current == "" {
		return
	}
	s.mu.Lock()
	s.val.origin = Env
	s.mu.Unlock()
	if def.Resolve != nil {
		// Our current value found isn't definitive yet
		key, err := def.Resolve(environ, current)
		if err != nil {
			logf("knobs: ignoring %q=%q, setting to default %v: %s", current, environ[current], def.Default, err.Error())
			return
		}
		current = key
	}
	if def.Parse == nil {
		logf("knobs: missing Parse function for environment variable %q", current)
		return
	}
	if final, err := def.Parse(environ[current]); err == nil {
		s.mu.Lock()
		s.val.v = final
		s.mu.Unlock()
		return
	} else {
		logf("knobs: ignoring %q=%q, setting to default %v: %s", current, environ[current], def.Default, err.Error())
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

const maxDeriveDepth = 64

// GetScope retrieves the current configuration value from a specific scope.
func GetScope[T any](sc *Scope, kn Knob[T]) T {
	var zero T
	id := int(kn)
	for hops := 0; hops < maxDeriveDepth; hops++ {
		s := sc.get(id)
		if s == nil {
			logf("knobs: GetScope called with a nil *Scope for knob %d; returning the zero value", id)
			return zero
		}
		s.mu.RLock()
		val, parent := s.val, s.parent
		s.mu.RUnlock()

		if val.hasValue {
			if val.v == nil {
				// Only reachable when T is an interface type, where nil IS
				// the stored value (e.g. Definition[any]{Default: nil}).
				// This branch MUST come before the type assertion below: a
				// type assertion on a nil interface value fails even when
				// the target type is `any` itself.
				return zero
			}
			if v, ok := val.v.(T); ok {
				return v
			}
			warnOnce(id, "knobs: knob %d holds a value that is not the requested type; returning the zero value", id)
			return zero
		}
		if parent <= 0 {
			warnOnce(id, "knobs: knob %d has no value and no parent (unregistered or forged knob id); returning the zero value", id)
			return zero
		}
		id = parent
	}
	warnOnce(id, "knobs: exceeded maximum derive depth (%d) resolving knob %d; returning the zero value", maxDeriveDepth, id)
	return zero
}

// Set sets value for a new configuration value to the default scope.
func Set[T any](kn Knob[T], origin Origin, value T) {
	SetScope(DefaultScope(), kn, origin, value)
}

// SetScope sets value for a new configuration value to a specific scope.
func SetScope[T any](sc *Scope, kn Knob[T], origin Origin, value T) {
	id := int(kn)
	s := sc.get(id)
	if s == nil {
		logf("knobs: SetScope called with a nil *Scope for knob %d; ignoring", id)
		return
	}

	if origin != Code {
		if origin == Default {
			logf("knobs: rejected Set(%d, Default, ...): Default is not a settable origin", id)
			return
		}
		if s.definition == nil {
			logf("knobs: rejected Set(%d, %v, ...): knob has no configured Origins (derived knob)", id, origin)
			return
		}
		if _, ok := s.origins[origin]; !ok {
			logf("knobs: rejected Set(%d, %v, ...): origin not in allowed set %v", id, origin, s.origins)
			return
		}
	}

	s.mu.Lock()
	s.val = slot{v: value, origin: origin, hasValue: true}
	s.mu.Unlock()
}
