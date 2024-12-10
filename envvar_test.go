package knobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvVar(t *testing.T) {
	t.Run("env unset", func(t *testing.T) {
		e := EnvVar{Key: "MY_ENV"}
		v := e.getValue()
		assert.Equal(t, "", v)
	})

	t.Run("env set - no transform", func(t *testing.T) {
		t.Setenv("MY_ENV", "something")
		e := EnvVar{Key: "MY_ENV"}
		v := e.getValue()
		assert.Equal(t, "something", v)
	})

	t.Run("env set - with transform", func(t *testing.T) {
		t.Setenv("MY_ENV", "something")
		e := EnvVar{Key: "MY_ENV", Transform: func(s string) string { return "something-else" }}
		v := e.getValue()
		assert.Equal(t, "something-else", v)
	})
}
