package knobs

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// This file covers the two whole-surface concurrency/isolation contracts that don't already have
// a dedicated regression test: a goroutine storm exercising Get, Set, and Derive together across
// multiple scopes (TestConcurrentGetSetDeriveAcrossScopes), and Scope isolation
// (TestScopeIsolation). Three related contracts already have their own tests elsewhere and are
// deliberately not duplicated here:
//   - a Parse callback reading a different knob completes without deadlocking, timeout-guarded:
//     TestScopeCrossKnobParseDoesNotDeadlock (scope_test.go)
//   - concurrent DefaultScope() callers observe the same *Scope: TestDefaultScopeConcurrentInit
//     (scope_test.go)
//   - concurrent SetLogger + Get is race-clean: TestLogFnConcurrentAccess (log_test.go)

// TestConcurrentGetSetDeriveAcrossScopes storms Get, Set, and Derive concurrently, across
// multiple knobs and multiple scopes, and must pass cleanly under `go test -race`. It does not
// assert on the specific values that survive the storm -- which goroutine's Set wins a given race
// is nondeterministic by design -- only that concurrent use of the whole public surface together
// never panics or races.
func TestConcurrentGetSetDeriveAcrossScopes(t *testing.T) {
	t.Parallel()

	const (
		numRoots        = 10
		numScopes       = 5
		numGoroutines   = 50
		opsPerGoroutine = 200
	)

	roots := make([]Knob[int], numRoots)
	for i := range roots {
		roots[i] = Register(&Definition[int]{Default: i})
	}

	scopes := make([]*Scope, numScopes)
	for i := range scopes {
		scopes[i] = NewScope()
	}

	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	for g := 0; g < numGoroutines; g++ {
		go func(seed int) {
			defer wg.Done()
			for op := 0; op < opsPerGoroutine; op++ {
				sc := scopes[(seed+op)%numScopes]
				root := roots[(seed+op)%numRoots]

				switch op % 3 {
				case 0:
					_ = GetScope(sc, root)
				case 1:
					SetScope(sc, root, Code, seed+op)
				case 2:
					d := Derive(root)
					_ = GetScope(sc, d)
				}
			}
		}(g)
	}
	wg.Wait()
}

// TestScopeIsolation confirms that Set/SetScope on one scope never leaks into another: the same
// Knob holds an independent value per Scope.
func TestScopeIsolation(t *testing.T) {
	t.Parallel()

	knob := Register(&Definition[string]{Default: "default"})

	sc1 := NewScope()
	sc2 := NewScope()

	SetScope(sc1, knob, Code, "scope-1-value")
	SetScope(sc2, knob, Code, "scope-2-value")

	require.Equal(t, "scope-1-value", GetScope(sc1, knob))
	require.Equal(t, "scope-2-value", GetScope(sc2, knob))
	require.Equal(t, "default", Get(knob)) // the package-wide default scope is untouched
}
