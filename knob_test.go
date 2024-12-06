package knobs

import (
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
		}
		def.EnvVars = []EnvVar[string]{NewEnvVar("TEST_KNOB_INIT", ToString)}
		knob := Register(def)

		value := Get(knob)
		require.Equal(t, "default", value)
	})

	t.Run("env set", func(t *testing.T) {
		def := &Definition[string]{
			Default: "default",
		}
		t.Setenv("TEST_KNOB_INIT", "env value")
		def.EnvVars = []EnvVar[string]{NewEnvVar("TEST_KNOB_INIT", ToString)}
		knob := Register(def)

		value := Get(knob)
		require.Equal(t, "env value", value)
	})
	t.Run("multi env var", func(t *testing.T) {
		def := &Definition[string]{
			Default: "default",
		}
		t.Setenv("TEST_KNOB_INIT", "env value")
		t.Setenv("TEST_KNOB_INIT_2", "env_value_2")
		def.EnvVars = []EnvVar[string]{NewEnvVar("DOES_NOT_EXIST", ToString), NewEnvVar("TEST_KNOB_INIT", ToString), NewEnvVar("TEST_KNOB_INIT_2", ToString)}
		knob := Register(def)

		value := Get(knob)
		require.Equal(t, "env value", value)
	})
	t.Run("custom transform", func(t *testing.T) {
		def := &Definition[float64]{
			Default: 0.0,
		}
		t.Setenv("TEST_KNOB_INIT", "parentbased_always_on")

		transform := func(val string) (zero float64, ok bool) {
			val = strings.TrimSpace(strings.ToLower(val))

			var samplerMapping = map[string]string{
				"parentbased_always_on":  "1.0",
				"parentbased_always_off": "0.0",
			}

			val, ok = samplerMapping[val]
			if !ok {
				return zero, ok
			}
			v, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return zero, ok
			}
			return v, true
		}

		def.EnvVars = []EnvVar[float64]{NewEnvVar("TEST_KNOB_INIT", transform)}
		knob := Register(def)

		value := Get(knob)
		require.Equal(t, 1.0, value)
	})
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
