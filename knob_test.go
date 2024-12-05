package knobs

import (
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
	e := EnvVar[string]{
		Key: "TEST_KNOB_INIT",
	}
	def := &Definition[string]{
		Default: "default",
	}
	t.Run("no env var", func(t *testing.T) {
		def.EnvVars = []EnvVar[string]{e}
		knob := Register(def)

		value := Get(knob)
		require.Equal(t, "default", value)
	})

	t.Run("env var", func(t *testing.T) {
		t.Setenv("TEST_KNOB_INIT", "env value")
		def.EnvVars = []EnvVar[string]{e}
		knob := Register(def)

		value := Get(knob)
		require.Equal(t, "env value", value)
	})
	t.Run("multi env var", func(t *testing.T) {
		t.Setenv("TEST_KNOB_INIT", "env value")
		t.Setenv("TEST_KNOB_INIT_2", "env_value_2")
		def.EnvVars = []EnvVar[string]{EnvVar[string]{Key: "DOES_NOT_EXIST"}, e, EnvVar[string]{Key: "TEST_KNOB_INIT_2"}}
		knob := Register(def)

		value := Get(knob)
		require.Equal(t, "env value", value)
	})
	t.Run("Transform", func(t *testing.T) {
		t.Setenv("TEST_KNOB_INIT", "env value")

		e.Transform = func(val string) (string, bool) {
			var valueMap = map[string]string{
				"env value":   "hello!",
				"other value": "goodbye!",
			}
			v, ok := valueMap[val]
			return v, ok
		}

		def.EnvVars = []EnvVar[string]{e}
		knob := Register(def)

		value := Get(knob)
		require.Equal(t, "hello!", value)
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

func TestGetValue(t *testing.T) {

}
