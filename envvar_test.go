package knobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvVar(t *testing.T) {
	t.Run("env unset", func(t *testing.T) {
		e := EnvVar{key: "MY_ENV"}
		v, ok := e.getValue()
		assert.Equal(t, "", v)
		assert.False(t, ok)
	})
	t.Run("env set - no transform", func(t *testing.T) {
		t.Setenv("MY_ENV", "something")
		e := EnvVar{key: "MY_ENV"}
		v, ok := e.getValue()
		assert.Equal(t, "something", v)
		assert.True(t, ok)
	})
	t.Run("env set - with transform", func(t *testing.T) {
		t.Setenv("MY_ENV", "something")
		e := EnvVar{key: "MY_ENV", transform: func(s string) (string, bool) { return "something-else", true }}
		v, ok := e.getValue()
		assert.Equal(t, "something-else", v)
		assert.True(t, ok)
	})
}
