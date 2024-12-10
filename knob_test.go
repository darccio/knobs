package knobs

import (
	"fmt"
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
			EnvVars: []EnvVar{{Key: "TEST_KNOB_INIT"}},
			Parse:   ToString,
		}
		knob := Register(def)

		value := Get(knob)
		require.Equal(t, "default", value)
	})

	t.Run("env set", func(t *testing.T) {
		def := &Definition[string]{
			Default: "default",
			EnvVars: []EnvVar{{Key: "TEST_KNOB_INIT"}},
			Parse:   ToString,
		}
		t.Setenv("TEST_KNOB_INIT", "env value")
		knob := Register(def)

		value := Get(knob)
		require.Equal(t, "env value", value)
	})

	t.Run("multi env var", func(t *testing.T) {
		t.Setenv("TEST_KNOB_INIT", "env value")
		t.Setenv("TEST_KNOB_INIT_2", "env_value_2")

		def := &Definition[string]{
			Default: "default",
			EnvVars: []EnvVar{{Key: "TEST_KNOB_INIT"}, {Key: "TEST_KNOB_INIT_2"}},
			Parse:   ToString,
		}

		knob := Register(def)

		value := Get(knob)
		require.Equal(t, "env value", value)
	})

	t.Run("with envvar transform", func(t *testing.T) {
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
		def := &Definition[string]{
			Default: "0.0",
			EnvVars: []EnvVar{{"TEST_KNOB_INIT", transform}},
			Parse:   ToString,
		}

		knob := Register(def)

		value := Get(knob)
		require.Equal(t, "1.0", value)
	})
}

func TestParse(t *testing.T) {
	t.Setenv("TEST_KNOB_PARSE", "env value")

	t.Run("custom parser", func(t *testing.T) {
		def := &Definition[string]{
			Default: "default",
			EnvVars: []EnvVar{{Key: "TEST_KNOB_PARSE"}},
			Parse: func(v string) (string, error) {
				return fmt.Sprintf("cleaned: %s", v), nil
			},
		}
		knob := Register(def)

		value := Get(knob)
		require.Equal(t, "cleaned: env value", value)
	})

	t.Run("parse with error", func(t *testing.T) {
		defaultVal := "default"

		def := &Definition[string]{
			Default: defaultVal,
			EnvVars: []EnvVar{{Key: "TEST_KNOB_PARSE"}},
			Parse: func(v string) (string, error) {
				if v == "does_not_exist" {
					return "should_not_occur", nil
				}
				return "", ErrInvalidValue
			},
		}
		knob := Register(def)

		value := Get(knob)
		require.Equal(t, defaultVal, value)
	})

	t.Run("transform + CLEAN", func(t *testing.T) {
		// TODO: once EnvVar custom type changes are merged
		require.True(t, true)
	})
}

func TestSet(t *testing.T) {
	t.Parallel()

	def := &Definition[string]{
		Default: "default",
		Parse:   ToString,
	}
	knob := Register(def)

	t.Run("set by known origin", func(t *testing.T) {
		Set(knob, Code, "new value")

		value := Get(knob)
		require.Equal(t, "new value", value)
	})

	t.Run("not set an unknown origin", func(t *testing.T) {
		Set(knob, Code, "known value")
		Set(knob, Env, "this shouldn't be") // this fails silently

		value := Get(knob)
		require.Equal(t, "known value", value)
	})
}

func TestDerive(t *testing.T) {
	t.Parallel()

	def := &Definition[string]{
		Default: "default",
		Parse:   ToString,
	}
	parent := Register(def)
	knob := Derive(parent)

	t.Run("parent default value", func(t *testing.T) {
		value := Get(knob)
		require.Equal(t, "default", value)
	})

	t.Run("parent set value", func(t *testing.T) {
		Set(parent, Code, "parent value")

		value := Get(knob)
		require.Equal(t, "parent value", value)
	})

	t.Run("knob set value", func(t *testing.T) {
		Set(knob, Code, "knob value")

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
		EnvVars: []EnvVar{{Key: "TEST_KNOB_INT"}},
		Parse:   ToInt,
	}
	knob := Register(def)

	value := Get(knob)
	require.Equal(t, 42, value)
}
