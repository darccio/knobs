package knobs_test

import (
	"fmt"
	"testing"

	"github.com/darccio/knobs"
	"github.com/stretchr/testify/require"
)

func TestRegister(t *testing.T) {
	t.Parallel()
	def := &knobs.Definition[string]{
		Default: "default",
	}
	knob := knobs.Register(def)

	value := knobs.Get(knob)
	require.Equal(t, "default", value)
}

func TestInitialize(t *testing.T) {
	def := &knobs.Definition[string]{
		Default: "default",
		EnvVars: []string{"TEST_KNOB"},
	}

	t.Run("no env var", func(t *testing.T) {
		knob := knobs.Register(def)

		value := knobs.Get(knob)
		require.Equal(t, "default", value)
	})

	t.Run("env var", func(t *testing.T) {
		t.Setenv("TEST_KNOB", "env value")
		knob := knobs.Register(def)

		value := knobs.Get(knob)
		require.Equal(t, "env value", value)
	})
}

func TestCleanEnvvar(t *testing.T) {
	t.Setenv("TEST_KNOB", "env value")
	def := &knobs.Definition[string]{
		Default: "default",
		EnvVars: []string{"TEST_KNOB"},
		CleanEnvvar: func(v string) string {
			return fmt.Sprintf("cleaned: %s", v)
		},
	}
	knob := knobs.Register(def)

	value := knobs.Get(knob)
	require.Equal(t, "cleaned: env value", value)
}

func TestSet(t *testing.T) {
	t.Parallel()
	def := &knobs.Definition[string]{
		Default: "default",
	}
	knob := knobs.Register(def)

	t.Run("set by known origin", func(t *testing.T) {
		knobs.Set(knob, "new value", knobs.Code)

		value := knobs.Get(knob)
		require.Equal(t, "new value", value)
	})

	t.Run("not set an unknown origin", func(t *testing.T) {
		knobs.Set(knob, "known value", knobs.Code)
		knobs.Set(knob, "this shouldn't be", knobs.Env) // this fails silently

		value := knobs.Get(knob)
		require.Equal(t, "known value", value)
	})
}

func TestDerive(t *testing.T) {
	def := &knobs.Definition[string]{
		Default: "default",
	}
	parent := knobs.Register(def)
	knob := knobs.Derive(parent)

	t.Run("parent default value", func(t *testing.T) {
		value := knobs.Get(knob)
		require.Equal(t, "default", value)
	})

	t.Run("parent set value", func(t *testing.T) {
		knobs.Set(parent, "parent value", knobs.Code)

		value := knobs.Get(knob)
		require.Equal(t, "parent value", value)
	})

	t.Run("knob set value", func(t *testing.T) {
		knobs.Set(knob, "knob value", knobs.Code)

		value := knobs.Get(knob)
		require.Equal(t, "knob value", value)
	})
}
