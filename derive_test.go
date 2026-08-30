package knobs

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDeriveResolvesAfterSetDefaultScope confirms that a derived knob
// resolves correctly through the package-level Get/Set (which always use
// DefaultScope()) even after the process-wide default scope is swapped out
// entirely -- proving the fallback chain lives in the registry, not in
// whichever *Scope happened to be current at Derive time.
func TestDeriveResolvesAfterSetDefaultScope(t *testing.T) {
	// Not parallel: mutates the process-wide default scope.
	prev := DefaultScope()
	t.Cleanup(func() { SetDefaultScope(prev) })

	base := Register(&Definition[string]{
		Default: "base-default",
	})
	derived := Derive(base)

	SetDefaultScope(NewScope())

	require.Equal(t, "base-default", Get(derived))
}

// TestDeriveResolvesAfterScopeDelete confirms that deleting a scope's cached
// state for the root knob doesn't break a derived knob's fallback in that
// same scope: the chain is resolved fresh from the registry on next access.
func TestDeriveResolvesAfterScopeDelete(t *testing.T) {
	t.Parallel()

	sc := NewScope()
	base := Register(&Definition[string]{
		Default: "base-default",
	})
	derived := Derive(base)

	require.Equal(t, "base-default", GetScope(sc, derived))

	sc.delete(base.id)

	require.Equal(t, "base-default", GetScope(sc, derived))
}

// TestRegisterNilDefinitionReturnsZeroKnob confirms that Register(nil)
// degrades to the zero Knob instead of nil-dereferencing on def.Origins.
func TestRegisterNilDefinitionReturnsZeroKnob(t *testing.T) {
	// Not parallel: installs the global logger.
	defer SetLogger(func(string, ...interface{}) {})

	// Knob id 0 -- the zero Knob this test expects Register(nil) to return
	// -- is shared by every zero-value Knob[T] in the whole test binary, and
	// GetScope's "unregistered id" warnOnce for it fires at most once across
	// the entire suite. Consume that one-time warning here, before
	// installing the message-recording logger below, so the message count
	// asserted below reflects only Register(nil)'s own log regardless of
	// whether some other test already happened to touch knob id 0 first.
	_ = Get(Knob[string]{})

	var (
		mu       sync.Mutex
		messages []string
	)
	SetLogger(func(format string, args ...interface{}) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	})

	var knob Knob[string]
	require.NotPanics(t, func() {
		knob = Register[string](nil)
	})
	require.Equal(t, Knob[string]{}, knob)
	require.Equal(t, "", Get(knob))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, messages, 1)
}

// TestForgedIdAllocatesNoState confirms that GetScope on an id that was
// never registered returns the zero value without caching a junk state in
// the scope -- Scope.get returns nil outright now, instead of the
// pre-Step-5 behavior of caching a permanent placeholder *state for every
// id it ever saw.
func TestForgedIdAllocatesNoState(t *testing.T) {
	t.Parallel()

	sc := NewScope()
	before := len(sc.states)

	forged := Knob[string]{id: 987654321}
	var value string
	require.NotPanics(t, func() {
		value = GetScope(sc, forged)
	})
	require.Equal(t, "", value)
	require.Equal(t, before, len(sc.states))
}

// TestDeriveFromUnregisteredParentLogsButStillCreatesKnob confirms that
// deriving from a parent id that was never registered logs the mistake but
// still produces a usable knob -- Code remains settable on it regardless of
// inherited origins, since Code is always allowed.
func TestDeriveFromUnregisteredParentLogsButStillCreatesKnob(t *testing.T) {
	// Not parallel: installs the global logger.
	defer SetLogger(func(string, ...interface{}) {})

	var (
		mu       sync.Mutex
		messages []string
	)
	SetLogger(func(format string, args ...interface{}) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	})

	forgedParent := Knob[string]{id: 123456789}
	var derived Knob[string]
	require.NotPanics(t, func() {
		derived = Derive(forgedParent)
	})

	mu.Lock()
	require.Len(t, messages, 1)
	mu.Unlock()

	require.NotPanics(t, func() {
		Set(derived, Code, "still-usable")
	})
	require.Equal(t, "still-usable", Get(derived))
}
