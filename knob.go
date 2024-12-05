package knobs

import (
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

// EnvVar represents an env var and an optional transform for remapping the value set at the env var
type EnvVar[T any] struct {
	key       string
	transform func(v string) (T, bool)
}

func StringEnvVar(key string, transform func(v string) (string, bool)) (zero EnvVar[string]) {
	if key == "" {
		// TODO: log something? return an error?
		return zero
	}
	return EnvVar[string]{
		key:       key,
		transform: transform,
	}
}

func IntEnvVar(key string, transform func(v string) (int, bool)) (zero EnvVar[int]) {
	if key == "" {
		// TODO: log something? Return an error?
		return zero
	}
	if transform != nil {
		return EnvVar[int]{
			key:       key,
			transform: transform,
		}
	}
	return EnvVar[int]{
		key: key,
		transform: func(v string) (int, bool) {
			if vv, err := strconv.Atoi(v); err == nil {
				return vv, true
			} else {
				return vv, false
			}
		},
	}
}

func Float64EnvVar(key string, transform func(v string) (float64, bool)) (zero EnvVar[float64]) {
	if key == "" {
		// TODO: log something? return an error?
		return zero
	}
	if transform != nil {
		return EnvVar[float64]{
			key:       key,
			transform: transform,
		}
	}
	return EnvVar[float64]{
		key: key,
		transform: func(v string) (float64, bool) {
			if vv, err := strconv.ParseFloat(v, 64); err == nil {
				return vv, true
			} else {
				return vv, false
			}
		},
	}
}

func BoolEnvVar(key string, transform func(v string) (bool, bool)) (zero EnvVar[bool]) {
	if key == "" {
		// TODO: log something? return an error?
		return zero
	}
	if transform != nil {
		return EnvVar[bool]{
			key:       key,
			transform: transform,
		}
	}
	return EnvVar[bool]{
		key: key,
		transform: func(v string) (bool, bool) {
			if vv, err := strconv.ParseBool(v); err == nil {
				return vv, true
			} else {
				return vv, false
			}
		},
	}
}

func (e *EnvVar[T]) getValue() (zero T, ok bool) {
	v, ok := os.LookupEnv(e.key)
	if !ok {
		return zero, false
	}
	v = strings.TrimSpace(v)
	if e.transform != nil {
		return e.transform(v)
	}
	if _, ok := any(zero).(string); ok {
		return any(strings.TrimSpace(v)).(T), true
	}
	return zero, false
}

// Definition declares how a configuration is sourced.
type Definition[T any] struct {
	Default T
	Origins []Origin    // Default and Env origins are implicit
	EnvVars []EnvVar[T] // In order of precedence
}

func (def *Definition[T]) initializer(s *state) {
	var v T
	var ok bool
	for _, e := range def.EnvVars {
		v, ok = e.getValue()
		if ok {
			s.origin = Env
			s.current = v
			return
		}
	}
	s.current = def.Default
}

// Origin defines a known configuration source.
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

// Derive creates a new configuration based on a parent.
// It returns a Knob that can be used to retrieve the configuration value associated with the parent.
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
