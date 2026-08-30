package knobs

import (
	"sync"
	"sync/atomic"
)

type Scope struct {
	mu     sync.RWMutex
	states map[int]*state
}

func (sc *Scope) get(kn int) *state {
	if sc == nil {
		return nil
	}

	sc.mu.RLock()
	s, ok := sc.states[kn]
	sc.mu.RUnlock()

	if !ok {
		regMux.RLock()
		d := registry[kn]
		regMux.RUnlock()

		sc.mu.Lock()
		// NOTE: `=`, not `:=`, in the next line. With `:=` this still
		// compiles, but it shadows the outer `s`, leaving it nil, and
		// s.init() below would nil-dereference.
		if s, ok = sc.states[kn]; !ok {
			if sc.states == nil { // &Scope{} is constructable; guard against a nil map
				sc.states = make(map[int]*state)
			}
			s = &state{
				definition: d,
			}
			sc.states[kn] = s
		}
		sc.mu.Unlock()
	}

	// s.init() MUST run on every call, not only when the state is newly
	// created above, and MUST run outside sc.mu. sync.Once (inside
	// state.init) supplies the happens-before edge that lets a fast-path
	// reader safely observe the initializer's writes; moving this call
	// inside the `if !ok` block as an "optimization" would reintroduce a
	// data race. Running it outside sc.mu is what stops a Parse/Resolve
	// callback that reads a *different* knob in this scope from
	// deadlocking on this scope's own lock.
	s.init()
	return s
}

func (sc *Scope) set(kn int, s *state) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.states == nil {
		sc.states = make(map[int]*state)
	}
	sc.states[kn] = s
}

func (sc *Scope) delete(kn int) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	delete(sc.states, kn)
}

func NewScope() *Scope {
	return &Scope{
		states: make(map[int]*state),
	}
}

var defScope atomic.Pointer[Scope]

func DefaultScope() *Scope {
	if sc := defScope.Load(); sc != nil {
		return sc
	}
	sc := NewScope()
	if defScope.CompareAndSwap(nil, sc) {
		return sc
	}
	// Lost the race to another goroutine's first call; discard our
	// throwaway Scope and use theirs so every caller observes the same
	// pointer.
	return defScope.Load()
}

func SetDefaultScope(sc *Scope) {
	defScope.Store(sc)
}
