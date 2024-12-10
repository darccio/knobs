package knobs

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegister(t *testing.T) {
	t.Parallel()

	def := &Definition[string]{
		Default: "default",
	}
	knob := Register(def)

	value := Get(knob)
	require.Equal(t, "default", value)
}

func TestInitialize(t *testing.T) {
	t.Run("env unset", func(t *testing.T) {
		def := &Definition[string]{
			Default: "default",
			EnvVars: []EnvVar{{key: "TEST_KNOB_INIT"}},
			Clean: func(v string, _ string /* default value */) (string, bool) {
				return v, true
			},
		}
		knob := Register(def)

		value := Get(knob)
		require.Equal(t, "default", value)
	})

	t.Run("env set", func(t *testing.T) {
		def := &Definition[string]{
			Default: "default",
			EnvVars: []EnvVar{{key: "TEST_KNOB_INIT"}},
			Clean: func(v string, _ string /* default value */) (string, bool) {
				return v, true
			},
		}
		t.Setenv("TEST_KNOB_INIT", "env value")
		knob := Register(def)

		value := Get(knob)
		require.Equal(t, "env value", value)
	})
	t.Run("multi env var", func(t *testing.T) {
		def := &Definition[string]{
			Default: "default",
			Clean: func(v string, _ string /* default value */) (string, bool) {
				return v, true
			},
		}
		t.Setenv("TEST_KNOB_INIT", "env value")
		t.Setenv("TEST_KNOB_INIT_2", "env_value_2")
		def.EnvVars = []EnvVar{{key: "DOES_NOT_EXIST"}, {key: "TEST_KNOB_INIT"}, {key: "TEST_KNOB_INIT_2"}}
		knob := Register(def)

		value := Get(knob)
		require.Equal(t, "env value", value)
	})
	t.Run("with envvar transform", func(t *testing.T) {
		def := &Definition[string]{
			Default: "0.0",
			Clean: func(v string, _ string /* default value */) (string, bool) {
				return v, true
			},
		}
		t.Setenv("TEST_KNOB_INIT", "parentbased_always_on")

		transform := func(val string) string {
			val = strings.TrimSpace(strings.ToLower(val))

			var samplerMapping = map[string]string{
				"parentbased_always_on":  "1.0",
				"parentbased_always_off": "0.0",
			}

			if val, ok := samplerMapping[val]; ok {
				return val
			} else {
				return ""
			}
		}
		def.EnvVars = []EnvVar{{"TEST_KNOB_INIT", transform}}
		knob := Register(def)

		value := Get(knob)
		require.Equal(t, "1.0", value)
	})
}

func TestCleanEnvvar(t *testing.T) {
	t.Setenv("TEST_KNOB_CLEAN", "env value")
	def := &Definition[string]{
		Default: "default",
		EnvVars: []EnvVar{{key: "TEST_KNOB_CLEAN"}},
		Clean: func(v string, _ string /* default value */) (string, bool) {
			return fmt.Sprintf("cleaned: %s", v), true
		},
	}
	knob := Register(def)
	value := Get(knob)
	require.Equal(t, "cleaned: env value", value)
}

func TestSet(t *testing.T) {
	t.Parallel()

	def := &Definition[string]{
		Default: "default",
	}
	knob := Register(def)

	t.Run("set by known origin", func(t *testing.T) {
		Set(knob, "new value", Code)

		value := Get(knob)
		require.Equal(t, "new value", value)
	})

	t.Run("not set an unknown origin", func(t *testing.T) {
		Set(knob, "known value", Code)
		Set(knob, "this shouldn't be", Env) // this fails silently

		value := Get(knob)
		require.Equal(t, "known value", value)
	})
}

func TestDerive(t *testing.T) {
	t.Parallel()

	def := &Definition[string]{
		Default: "default",
	}
	parent := Register(def)
	knob := Derive(parent)

	t.Run("parent default value", func(t *testing.T) {
		value := Get(knob)
		require.Equal(t, "default", value)
	})

	t.Run("parent set value", func(t *testing.T) {
		Set(parent, "parent value", Code)

		value := Get(knob)
		require.Equal(t, "parent value", value)
	})

	t.Run("knob set value", func(t *testing.T) {
		Set(knob, "knob value", Code)

		value := Get(knob)
		require.Equal(t, "knob value", value)
	})
}

func TestDeleteState(t *testing.T) {
	t.Parallel()

	knob := Register(&Definition[string]{
		Default: "default",
	})

	ref := int(knob)
	_, ok := registry[ref]
	require.True(t, ok)

	deleteState(ref)

	_, ok = registry[ref]
	require.False(t, ok)
}

func TestIntKnobFromEnv(t *testing.T) {
	t.Setenv("TEST_KNOB_INT", "42")

	def := &Definition[int]{
		EnvVars: []EnvVar{{key: "TEST_KNOB_INT"}},
		Clean:   func(v string, _ int) (int, bool) { i, _ := strconv.Atoi(v); return i, true },
	}
	knob := Register(def)

	value := Get(knob)
	require.Equal(t, 42, value)
}
