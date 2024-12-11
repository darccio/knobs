package knobs

import "sync"

var (
	defMux   sync.Mutex
	defScope *Scope
	defOnce  sync.Once
)

type Scope struct {
	sync.RWMutex

	states map[int]*state
}

func (sc *Scope) get(kn int) *state {
	sc.Lock()
	defer sc.Unlock()

	s, ok := sc.states[kn]
	if ok {
		return s
	}

	// To avoid race conditions, get must create a new state and set it in the scope
	// because there can be multiple goroutines trying to get and set the same Knob
	// concurrently.
	// It also simplifies the code because we don't need to check if the Knob is already
	// in the scope in multiple places.
	regMux.RLock()
	defer regMux.RUnlock()

	s = &state{
		definition: registry[kn],
	}
	// Unconditionally initialize the state.
	s.init()

	sc.states[kn] = s // This is safe because we have the lock.
	return s
}

func (sc *Scope) set(kn int, s *state) {
	sc.Lock()
	defer sc.Unlock()

	sc.states[kn] = s
}

func (sc *Scope) delete(kn int) {
	sc.Lock()
	defer sc.Unlock()

	delete(sc.states, kn)
}

func DefaultScope() *Scope {
	defMux.Lock()
	defer defMux.Unlock()

	if defScope != nil {
		return defScope
	}
	defScope = &Scope{
		states: make(map[int]*state),
	}
	return defScope
}

func SetDefaultScope(sc *Scope) {
	defMux.Lock()
	defer defMux.Unlock()

	defScope = sc
}
