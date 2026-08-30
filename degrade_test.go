package knobs

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGetZeroValueKnobDoesNotPanic confirms that Get on a zero-value,
// never-registered Knob[T] degrades to the type's zero value instead of
// panicking. This is the "state.val.hasValue is false and there is no
// parent" branch of GetScope.
//
// We deliberately do not assert anything about whether a log was emitted:
// knob id 0 is shared by every zero-value Knob[T] in the whole test binary
// regardless of T, so warnOnce's suppression means only the very first such
// call across the entire suite actually logs. Asserting on that would make
// this test order-dependent and flaky.
func TestGetZeroValueKnobDoesNotPanic(t *testing.T) {
	t.Parallel()

	var k Knob[string]

	var value string
	require.NotPanics(t, func() {
		value = Get(k)
	})
	require.Equal(t, "", value)
}

// TestGetForgedKnobWrongTypeReturnsZeroAndLogsOnce confirms that reading a
// registered knob through a Knob[T] handle of the wrong T degrades to the
// zero value instead of panicking, and that the degraded-read log is only
// emitted once even across repeated calls (warnOnce suppression).
func TestGetForgedKnobWrongTypeReturnsZeroAndLogsOnce(t *testing.T) {
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

	stringKnob := Register(&Definition[string]{
		Default: "a-string-value",
	})
	forged := Knob[int](int(stringKnob))

	var first, second int
	require.NotPanics(t, func() {
		first = Get(forged)
	})
	require.NotPanics(t, func() {
		second = Get(forged)
	})
	require.Equal(t, 0, first)
	require.Equal(t, 0, second)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, messages, 1)
	require.Contains(t, messages[0], fmt.Sprintf("%d", int(stringKnob)))
}

// TestGetDefinitionAnyNilDefaultReturnsNilWithoutLogging is the regression
// test for the bug this step exists to fix: a Definition[any] with a nil
// Default has hasValue == true (a value WAS set) but that value happens to
// be the nil interface. Before this fix, GetScope treated any nil
// s.current as "unset" and always hit the trailing type assertion, which
// panics. It must now return the zero value (nil, for T = any) silently,
// with no degraded-read log, since this is a perfectly valid knob value and
// not a misuse.
func TestGetDefinitionAnyNilDefaultReturnsNilWithoutLogging(t *testing.T) {
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

	knob := Register(&Definition[any]{Default: nil})

	var value any
	require.NotPanics(t, func() {
		value = Get(knob)
	})
	require.Nil(t, value)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, messages, 0)
}

// TestDerivedKnobFromForeignScopeDoesNotPanic confirms that reading a
// derived knob from a scope other than the one it was derived in degrades
// to the zero value instead of panicking. A derived knob's *state only
// exists in the scope it was created in; looking it up from a foreign scope
// auto-vivifies a brand new, definition-less, parent-less *state there, so
// there is nothing to fall back to.
//
// The result is explicitly the ZERO value, NOT the parent's default value:
// making derived knobs resolve their parent correctly across scopes
// requires derived knobs to become registry-backed, which is a LATER step
// of this campaign. This step only fixes the PANIC, converting it into a
// graceful (if currently wrong) zero-value degrade.
func TestDerivedKnobFromForeignScopeDoesNotPanic(t *testing.T) {
	t.Parallel()

	base := Register(&Definition[string]{
		Default: "parent-default",
	})
	derived := Derive(base) // lands in the default scope

	sc2 := NewScope()

	var value string
	require.NotPanics(t, func() {
		value = GetScope(sc2, derived)
	})
	require.Equal(t, "", value)
}

// TestSetDerivedKnobNonCodeOriginDoesNotPanicAndIsRejected confirms that
// Set-ing a derived knob with a non-Code origin no longer nil-dereferences
// (a derived state's embedded *definition is nil, and the old code
// unconditionally read s.origins without checking that first), and that the
// rejected write does not silently take effect.
//
// Teaching derived knobs to inherit their parent's allowed Origins is a
// LATER step's feature (it requires promoting origins/parent resolution
// onto a shared, registry-backed representation); this step only fixes the
// crash, so every non-Code Set on a derived knob is rejected for now.
func TestSetDerivedKnobNonCodeOriginDoesNotPanicAndIsRejected(t *testing.T) {
	t.Parallel()

	base := Register(&Definition[string]{
		Default: "base-default",
		Origins: []Origin{Env},
	})
	derived := Derive(base)

	require.NotPanics(t, func() {
		Set(derived, Env, "should not apply")
	})

	require.Equal(t, Get(base), Get(derived))
}

// TestSetRejectsDefaultOrigin confirms that Default can never be used as a
// Set origin: it represents a knob's own baseline established at init, not
// an externally pushed value, so asserting "this came from Default" via an
// external Set call is a provenance lie and must be rejected.
func TestSetRejectsDefaultOrigin(t *testing.T) {
	t.Parallel()

	knob := Register(&Definition[string]{
		Default: "original-default",
	})

	require.NotPanics(t, func() {
		Set(knob, Default, "attempted-override")
	})

	require.Equal(t, "original-default", Get(knob))
}

// TestSetUnlistedOriginIsRejectedAndLogged confirms that Set is rejected,
// and the rejection logged, when the origin is not Code and not present in
// the knob's configured Origins.
func TestSetUnlistedOriginIsRejectedAndLogged(t *testing.T) {
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

	knob := Register(&Definition[string]{
		Default: "unset-default",
		// Origins intentionally left empty: no non-Code origin is allowed.
	})

	Set(knob, Env, "v")

	require.Equal(t, "unset-default", Get(knob))

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(messages), 1)
}
