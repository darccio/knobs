package knobs

import (
	"errors"
	"maps"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
)

// Global variables
var (
	counter  atomic.Int64
	regMux   sync.RWMutex
	registry = make(map[int]*definition)
)

var (
	// ErrInvalidValue is returned when the value cannot be converted to the expected type.
	// This error is useful when the Parse function fails to convert the value.
	ErrInvalidValue = errors.New("invalid value")
)

// definition is an internal representation of a configuration definition. See Definition.
type definition struct {
	initFn  initializer
	origins map[Origin]struct{}
	parent  int // 0 for a root knob; the parent's registry id for a derived knob
}

// slot holds a knob's current value together with its provenance and
// whether it has ever been set at all, grouped so all three move together
// under a single lock acquisition.
type slot struct {
	v        any
	origin   Origin
	hasValue bool // distinguishes "unset" from "set to a nil interface value"
}

// state is an instance of a configuration definition. *definition is a named field, not an
// anonymous embed: state.init (a method) and definition.initFn (a field) used to have similar
// enough names that embedding created a promotion-shadowing hazard -- renaming or deleting one
// could silently start resolving to the other, with different semantics, without a compiler
// error. Naming the field forces every access site to be explicit instead.
type state struct {
	mu  sync.RWMutex
	def *definition

	once sync.Once
	val  slot
}

func (s *state) init() {
	if s.def == nil || s.def.initFn == nil {
		// Knob is unregistered, or is a derived knob with no initializer of its own.
		return
	}
	s.once.Do(func() { s.def.initFn(s) })
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
	// applies regardless of what's listed here. A knob derived via Derive
	// inherits this set from its root at Derive time; it cannot be changed
	// afterward.
	Origins  []Origin
	EnvVars  []EnvVar
	Requires []any // Knobs that must be set to a non-zero value before this one; used only for documentation purposes
	// Resolve handles validation and conditional behavior.
	// It must not call Get/GetScope/Set/SetScope on the knob currently being initialized: each
	// knob's initialization is guarded by a non-reentrant sync.Once, so a same-knob call from
	// within Resolve deadlocks. Calls to other, already-registered knobs are safe.
	// Resolve runs at most once per (knob, scope) pair, on first access. If it panics, the
	// panic propagates to the caller of Get/GetScope, and -- because sync.Once treats a
	// panicking call as complete -- Resolve never runs again for this (knob, scope) pair; the
	// knob is left readable at its Default on every subsequent Get.
	Resolve func(environ map[string]string, decision string) (string, error)
	// Parse converts a string to the expected type; ignores the returned value if an error is returned.
	// It must not call Get/GetScope/Set/SetScope on the knob currently being initialized: each
	// knob's initialization is guarded by a non-reentrant sync.Once, so a same-knob call from
	// within Parse deadlocks. Calls to other, already-registered knobs are safe.
	// Parse runs at most once per (knob, scope) pair, on first access. If it panics, the panic
	// propagates to the caller of Get/GetScope, and -- because sync.Once treats a panicking
	// call as complete -- Parse never runs again for this (knob, scope) pair; the knob is left
	// readable at its Default on every subsequent Get.
	Parse func(string) (T, error)
}

func (def *Definition[T]) initializer(s *state) {
	// Phase 1: commit the Default immediately, before any user callback runs. A panicking
	// Transform/Resolve/Parse leaves sync.Once permanently "done" without a retry, so this is
	// the only commit that's guaranteed to have happened by the time such a panic is caught
	// upstream -- it must already be correct and correctly attributed to Default.
	s.mu.Lock()
	s.val = slot{v: def.Default, origin: Default, hasValue: true}
	s.mu.Unlock()

	if len(def.EnvVars) == 0 {
		return
	}
	var (
		current = ""
		environ = make(map[string]string, len(def.EnvVars))
	)
	for _, e := range def.EnvVars {
		v, err := e.getValue()
		if err != nil {
			logf("knobs: ignoring env var %q: %s", e.Key, err.Error())
			continue
		}
		if v == "" {
			continue
		}
		environ[e.Key] = v
		if len(environ) == 1 {
			current = e.Key
		} else {
			logf("knobs: environment variable %q=%q ignored: %q is already set and takes precedence", e.Key, v, current)
		}
	}
	if current == "" {
		return
	}
	if def.Resolve != nil {
		// Our current value found isn't definitive yet
		key, err := def.Resolve(environ, current)
		if err != nil {
			logf("knobs: ignoring %q=%q, setting to default %v: %s", current, environ[current], def.Default, err.Error())
			return
		}
		if _, ok := environ[key]; !ok {
			// A typo'd/unknown key would otherwise reach Parse(""), which for some Parse
			// functions (e.g. ToString) succeeds and silently sets the knob to "".
			logf("knobs: Resolve returned unknown key %q, keeping default %v", key, def.Default)
			return
		}
		current = key
	}
	if def.Parse == nil {
		logf("knobs: missing Parse function for environment variable %q", current)
		return
	}
	final, err := def.Parse(environ[current])
	if err != nil {
		logf("knobs: ignoring %q=%q, setting to default %v: %s", current, environ[current], def.Default, err.Error())
		return
	}

	// Phase 2: commit the parsed value, and only now claim Env provenance -- never before
	// Parse has actually validated it.
	s.mu.Lock()
	s.val = slot{v: final, origin: Env, hasValue: true}
	s.mu.Unlock()
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

// Knob defines an available configuration. Its representation is deliberately opaque: unlike a
// plain int, a Knob[T] cannot be forged from an arbitrary int outside this package, which closes
// off a class of type-confusion bugs where a forged Knob[T] of the wrong T could silently read
// another knob's value or permanently corrupt one. Comparable and usable as a map key.
type Knob[T any] struct {
	id int
}

// Register adds a new configuration, valid in every Scope.
// Register returns a Knob that can be used to retrieve the configuration value. A Knob can be used in multiple scopes.
// Register is not idempotent, so calling it multiple times with the same Definition will create multiple Knobs.
// Passing a nil Definition logs and returns the zero Knob, which behaves like any other unregistered id.
func Register[T any](def *Definition[T]) Knob[T] {
	if def == nil {
		logf("knobs: Register called with a nil Definition; returning the zero Knob")
		var zero Knob[T]
		return zero
	}

	// Snapshot def: resolution is lazy (first Get), and def.initializer is a method value bound
	// to whatever *Definition[T] it's given. Binding it to the caller's own pointer would let a
	// caller who mutates def.Default/Parse/EnvVars after Register but before the first Get race
	// against whichever goroutine's Get triggers the initializer. EnvVars is a slice (a reference
	// type), so copying the struct alone isn't enough for it -- it needs its own clone. The other
	// fields (Default, Parse, Resolve, Origins) are copied by value or are already-immutable
	// function values, so the struct copy alone is sufficient for them.
	defCopy := *def
	defCopy.EnvVars = slices.Clone(def.EnvVars)

	var (
		k       = int(counter.Add(1))
		origins = make(map[Origin]struct{}, len(defCopy.Origins))
	)
	for _, o := range defCopy.Origins {
		origins[o] = struct{}{}
	}
	d := &definition{
		initFn:  defCopy.initializer,
		origins: origins,
	}

	regMux.Lock()
	registry[k] = d
	regMux.Unlock()

	return Knob[T]{id: k}
}

// Derive creates a new configuration based on a parent Knob, valid in every Scope.
// A derived Knob has no value of its own until Set: until then, Get dynamically falls through to
// the parent's current value -- this is a live fallthrough, not a one-time snapshot, so a derived
// knob sees values set on its parent after the derive. The parent Knob can itself be another
// derived Knob, in which case the fallback chain and allowed Set origins are resolved transitively.
// Derive is not idempotent, so calling it multiple times with the same parent will create multiple Knobs.
func Derive[T any](parent Knob[T]) Knob[T] {
	pid := parent.id

	regMux.RLock()
	parentDef, ok := registry[pid]
	regMux.RUnlock()

	var origins map[Origin]struct{}
	if ok {
		origins = maps.Clone(parentDef.origins)
	} else {
		logf("knobs: Derive called with unregistered parent knob %d; the derived knob will have no allowed Set origins", pid)
		origins = map[Origin]struct{}{}
	}

	dk := int(counter.Add(1))
	d := &definition{
		// initFn is nil: a derived knob has no EnvVars/Parse/Resolve of its own;
		// its value always comes from Set or falls through to parent.
		origins: origins,
		parent:  pid,
	}

	regMux.Lock()
	registry[dk] = d
	regMux.Unlock()

	return Knob[T]{id: dk}
}

// Get retrieves the current configuration value from the default scope.
func Get[T any](kn Knob[T]) T {
	return GetScope(DefaultScope(), kn)
}

const maxDeriveDepth = 64

// GetScope retrieves the current configuration value from a specific scope.
func GetScope[T any](sc *Scope, kn Knob[T]) T {
	v, _ := GetWithOriginScope(sc, kn)
	return v
}

// GetWithOrigin retrieves the current configuration value and its Origin from the default scope,
// both under the same lock acquisition -- unlike calling Get and separately inspecting
// provenance some other way, this cannot observe a torn read where the value and its reported
// origin came from two different Sets. For a knob that has no value at all (unregistered, forged,
// or a derived knob whose parent chain doesn't resolve), the reported Origin is Default, matching
// Get's zero-value degrade.
func GetWithOrigin[T any](kn Knob[T]) (T, Origin) {
	return GetWithOriginScope(DefaultScope(), kn)
}

// GetWithOriginScope retrieves the current configuration value and its Origin from a specific
// scope, both under the same lock acquisition. See GetWithOrigin.
func GetWithOriginScope[T any](sc *Scope, kn Knob[T]) (T, Origin) {
	var zero T
	if sc == nil {
		logf("knobs: GetScope called with a nil *Scope for knob %d; returning the zero value", kn.id)
		return zero, Default
	}
	id := kn.id
	for hops := 0; hops < maxDeriveDepth; hops++ {
		s := sc.get(id)
		if s == nil {
			warnOnce(id, "knobs: knob %d is not registered; returning the zero value", id)
			return zero, Default
		}

		s.mu.RLock()
		val := s.val
		s.mu.RUnlock()

		if val.hasValue {
			if val.v == nil {
				// Only reachable when T is an interface type, where nil IS
				// the stored value (e.g. Definition[any]{Default: nil}).
				// This branch MUST come before the type assertion below: a
				// type assertion on a nil interface value fails even when
				// the target type is `any` itself.
				return zero, val.origin
			}
			if v, ok := val.v.(T); ok {
				return v, val.origin
			}
			warnOnce(id, "knobs: knob %d holds a value that is not the requested type; returning the zero value", id)
			return zero, Default
		}
		parent := s.def.parent // set once at Register/Derive and never mutated after, so safe to read without s.mu
		if parent <= 0 {
			warnOnce(id, "knobs: knob %d has no value and no parent (a root knob's Default should always be set, or a knob was derived from a forged/zero id); returning the zero value", id)
			return zero, Default
		}
		id = parent
	}
	warnOnce(id, "knobs: exceeded maximum derive depth (%d) resolving knob %d; returning the zero value", maxDeriveDepth, id)
	return zero, Default
}

// Set sets value for a new configuration value to the default scope.
func Set[T any](kn Knob[T], origin Origin, value T) {
	SetScope(DefaultScope(), kn, origin, value)
}

// SetScope sets value for a new configuration value to a specific scope.
func SetScope[T any](sc *Scope, kn Knob[T], origin Origin, value T) {
	id := kn.id
	if sc == nil {
		logf("knobs: SetScope called with a nil *Scope for knob %d; ignoring", id)
		return
	}
	s := sc.get(id)
	if s == nil {
		logf("knobs: SetScope called with unregistered knob %d; ignoring", id)
		return
	}

	if origin != Code {
		if origin == Default {
			logf("knobs: rejected Set(%d, Default, ...): Default is not a settable origin", id)
			return
		}
		if _, ok := s.def.origins[origin]; !ok {
			logf("knobs: rejected Set(%d, %v, ...): origin not in allowed set %v", id, origin, s.def.origins)
			return
		}
	}

	s.mu.Lock()
	s.val = slot{v: value, origin: origin, hasValue: true}
	s.mu.Unlock()
}
