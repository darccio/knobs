package knobs

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResolveReturningUnknownKeyKeepsDefault is the regression test for a
// Resolve callback returning a key that doesn't exist in the collected
// environ map (e.g. a typo). Proceeding to Parse(environ[key]) would silently
// evaluate to Parse(""), which for some Parse functions (e.g. ToString)
// succeeds and silently sets the knob to "". Instead, the knob must keep its
// Default and the mistake must be logged.
func TestResolveReturningUnknownKeyKeepsDefault(t *testing.T) {
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

	envKey := "KNOBS_RESOLVE_UNKNOWN_KEY_TEST_VAR"
	t.Setenv(envKey, "some-value")

	def := &Definition[string]{
		Default: "default",
		EnvVars: []EnvVar{{Key: envKey}},
		Resolve: func(environ map[string]string, decision string) (string, error) {
			return "TYPO_KEY_NOT_IN_ENVIRON", nil
		},
		Parse: ToString,
	}
	knob := Register(def)

	value := Get(knob)
	require.Equal(t, "default", value)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, messages, 1)
	require.Contains(t, messages[0], "TYPO_KEY_NOT_IN_ENVIRON")
}

// TestOriginReflectsActualSource is the regression test for bug (1): origin
// must only ever be reported as Env once Parse has actually validated the
// value, never merely because some env var was found. Also doubles as
// GetWithOrigin's own regression coverage, since Step 6 added it specifically
// to make this contract observable through the public API.
func TestOriginReflectsActualSource(t *testing.T) {
	t.Run("failed parse keeps origin at Default", func(t *testing.T) {
		envKey := "KNOBS_ORIGIN_FAILED_PARSE_TEST_VAR"
		t.Setenv(envKey, "not-an-int")

		def := &Definition[int]{
			Default: 7,
			EnvVars: []EnvVar{{Key: envKey}},
			Parse:   ToInt,
		}
		knob := Register(def)

		value, origin := GetWithOrigin(knob)
		require.Equal(t, 7, value)
		require.Equal(t, Default, origin)
	})

	t.Run("successful parse reports Env", func(t *testing.T) {
		envKey := "KNOBS_ORIGIN_SUCCESS_PARSE_TEST_VAR"
		t.Setenv(envKey, "42")

		def := &Definition[int]{
			Default: 7,
			EnvVars: []EnvVar{{Key: envKey}},
			Parse:   ToInt,
		}
		knob := Register(def)

		value, origin := GetWithOrigin(knob)
		require.Equal(t, 42, value)
		require.Equal(t, Env, origin)
	})
}

// TestShadowedEnvVarIsLogged is the regression test for bug (3): when
// multiple EnvVars are set simultaneously, first-wins behavior must remain
// unchanged, but the shadowed var(s) must now be logged so a user migrating
// from a deprecated env var name has a way to diagnose why the old one
// "isn't working".
func TestShadowedEnvVarIsLogged(t *testing.T) {
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

	firstKey := "KNOBS_SHADOW_FIRST_TEST_VAR"
	secondKey := "KNOBS_SHADOW_SECOND_TEST_VAR"
	t.Setenv(firstKey, "first-value")
	t.Setenv(secondKey, "second-value")

	def := &Definition[string]{
		Default: "default",
		EnvVars: []EnvVar{{Key: firstKey}, {Key: secondKey}},
		Parse:   ToString,
	}
	knob := Register(def)

	value := Get(knob)
	require.Equal(t, "first-value", value)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, messages, 1)
	require.Contains(t, messages[0], firstKey)
	require.Contains(t, messages[0], secondKey)
}

// TestPanickingParsePropagatesAndLeavesDefaultReadable is the regression test
// proving that sync.Once's "a panicking call counts as done" behavior does
// not strand the knob at the zero value: because the Default is committed in
// phase 1, before Parse ever runs, a panicking Parse still leaves the knob
// readable at its Default on every subsequent Get.
func TestPanickingParsePropagatesAndLeavesDefaultReadable(t *testing.T) {
	envKey := "KNOBS_PANICKING_PARSE_TEST_VAR"
	t.Setenv(envKey, "some-value")

	def := &Definition[string]{
		Default: "the-default",
		EnvVars: []EnvVar{{Key: envKey}},
		Parse: func(string) (string, error) {
			panic("boom: Parse blew up")
		},
	}
	knob := Register(def)

	require.Panics(t, func() { Get(knob) })

	require.NotPanics(t, func() {
		require.Equal(t, "the-default", Get(knob))
	})
}
