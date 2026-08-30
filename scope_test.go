package knobs

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestScopeCrossKnobParseDoesNotDeadlock is the regression test for the
// deadlock described in the adversarial review: Scope.get used to hold its
// lock across the whole lookup-or-create path, including the call to
// state.init(), which runs user-supplied callbacks such as Definition.Parse.
// If that callback called Get on a DIFFERENT knob in the same scope, the
// nested call blocked forever waiting for the lock the outer call already
// held.
//
// This does NOT test (and must not be read as testing) the same-knob case:
// a Parse/Resolve callback calling Get/Set on the knob it is currently
// initializing still deadlocks, because sync.Once is not reentrant. That
// case is documented on Definition's Resolve/Parse fields, not fixed here.
func TestScopeCrossKnobParseDoesNotDeadlock(t *testing.T) {
	// Not parallel: uses t.Setenv to force Parse to run, which panics if
	// combined with t.Parallel().

	other := Register(&Definition[string]{
		Default: "other-default",
	})

	t.Setenv("KNOBS_TEST_CROSS_KNOB_DEADLOCK", "trigger")
	knob := Register(&Definition[string]{
		Default: "default",
		EnvVars: []EnvVar{{Key: "KNOBS_TEST_CROSS_KNOB_DEADLOCK"}},
		Parse: func(string) (string, error) {
			// Reads a DIFFERENT, already-registered knob from the same
			// (default) scope while this knob is still initializing.
			return Get(other), nil
		},
	})

	const goroutines = 20
	done := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		wg.Add(goroutines)
		for i := 0; i < goroutines; i++ {
			go func() {
				defer wg.Done()
				Get(knob)
			}()
		}
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// OK: all concurrent Get calls completed.
	case <-time.After(5 * time.Second):
		t.Fatal("Get deadlocked: Scope.get appears to hold its lock across a cross-knob Parse callback")
	}

	require.Equal(t, "other-default", Get(knob))
}

// TestScopeInitializerRunsExactlyOnce is a race-detector storm proving that,
// however many goroutines race to call Get on a knob for the first time,
// the knob's initializer (and therefore its Parse callback) runs exactly
// once. Must pass under go test -race.
func TestScopeInitializerRunsExactlyOnce(t *testing.T) {
	// Not parallel: uses t.Setenv to force Parse to run, which panics if
	// combined with t.Parallel().

	var calls atomic.Int32

	t.Setenv("KNOBS_TEST_INIT_ONCE", "trigger")
	knob := Register(&Definition[string]{
		Default: "default",
		EnvVars: []EnvVar{{Key: "KNOBS_TEST_INIT_ONCE"}},
		Parse: func(string) (string, error) {
			calls.Add(1)
			return "parsed", nil
		},
	})

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			Get(knob)
		}()
	}
	wg.Wait()

	require.Equal(t, int32(1), calls.Load())
}

// TestScopeZeroValueBehavesLikeNewScope confirms that a zero-value Scope
// (constructable by any caller since Scope is exported) lazily initializes
// its internal map on first use and behaves identically to a Scope built
// with NewScope, instead of panicking with "assignment to entry in nil
// map".
func TestScopeZeroValueBehavesLikeNewScope(t *testing.T) {
	t.Parallel()

	knob := Register(&Definition[string]{
		Default: "zero-value-scope-default",
	})

	var sc Scope // deliberately not NewScope()

	var value string
	require.NotPanics(t, func() {
		value = GetScope(&sc, knob)
	})
	require.Equal(t, "zero-value-scope-default", value)
}

// TestScopeGetScopeNilScopeReturnsZeroValue confirms that GetScope degrades
// gracefully to the type's zero value when handed a nil *Scope, instead of
// panicking.
func TestScopeGetScopeNilScopeReturnsZeroValue(t *testing.T) {
	t.Parallel()

	knob := Register(&Definition[string]{
		Default: "should-not-be-returned",
	})

	var value string
	require.NotPanics(t, func() {
		value = GetScope(nil, knob)
	})
	require.Equal(t, "", value)
}

// TestScopeSetScopeNilScopeNoop confirms that SetScope is a safe no-op when
// handed a nil *Scope, instead of panicking.
func TestScopeSetScopeNilScopeNoop(t *testing.T) {
	t.Parallel()

	knob := Register(&Definition[int]{
		Default: 0,
	})

	require.NotPanics(t, func() {
		SetScope(nil, knob, Code, 42)
	})
}

// TestDefaultScopeConcurrentInit exercises DefaultScope's first-call race:
// many goroutines racing to initialize the process-wide default scope must
// all observe the same *Scope instance.
//
// This test is intentionally NOT t.Parallel(): it temporarily nils out the
// process-global default scope via SetDefaultScope(nil), which would
// corrupt any other test relying on DefaultScope() if it ran concurrently.
// Because it isn't parallel, Go's test scheduler guarantees it runs to
// completion before any t.Parallel() test group in this package is
// unpaused, so this is safe without extra synchronization against other
// test files.
func TestDefaultScopeConcurrentInit(t *testing.T) {
	prev := DefaultScope()
	t.Cleanup(func() { SetDefaultScope(prev) })
	SetDefaultScope(nil)

	const n = 50
	results := make([]*Scope, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			results[i] = DefaultScope()
		}(i)
	}
	wg.Wait()

	for i := 1; i < n; i++ {
		require.Same(t, results[0], results[i])
	}
}
