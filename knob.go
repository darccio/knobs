package knobs

import (
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	counter  atomic.Int32
	registry map[int]*state
	regMux   sync.RWMutex
)

type state struct {
	sync.RWMutex
	current    any
	initialize initializer
	origins    map[Origin]struct{}
	lastOrigin Origin
	parent     int
}

type initializer func(*state)

// Definition declares how a configuration is sourced.
type Definition[T any] struct {
	Default     T
	Origins     []Origin // Default and Env origins are implicit
	EnvVars     []string
	CleanEnvvar func(string) T
}

func (def *Definition[T]) initializer(s *state) {
	var v string
	for _, envVar := range def.EnvVars {
		v = os.Getenv(envVar)
		v = strings.TrimSpace(v)
		if v != "" {
			break
		}
	}
	if v == "" {
		s.current = def.Default
		return
	}
	s.lastOrigin = Env
	if def.CleanEnvvar == nil {
		s.current = v
		return
	}
	s.current = def.CleanEnvvar(v)
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
	if registry == nil {
		registry = make(map[int]*state)
	}
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
		s.initialize(s)
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
	s.lastOrigin = origin
	s.current = value
}
