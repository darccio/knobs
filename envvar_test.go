package knobs

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvVar(t *testing.T) {
	t.Run("env unset", func(t *testing.T) {
		e := EnvVar{Key: "MY_ENV"}
		v, err := e.getValue()
		assert.NoError(t, err)
		assert.Equal(t, "", v)
	})

	t.Run("env set - no transform", func(t *testing.T) {
		t.Setenv("MY_ENV", "something")
		e := EnvVar{Key: "MY_ENV"}
		v, err := e.getValue()
		assert.NoError(t, err)
		assert.Equal(t, "something", v)
	})

	t.Run("env set - with transform", func(t *testing.T) {
		t.Setenv("MY_ENV", "something")
		e := EnvVar{Key: "MY_ENV", Transform: func(s string) (string, error) { return "something-else", nil }}
		v, err := e.getValue()
		assert.NoError(t, err)
		assert.Equal(t, "something-else", v)
	})

	t.Run("env set - transform returns error", func(t *testing.T) {
		someSentinelErr := errors.New("transform blew up")
		t.Setenv("MY_ENV", "something")
		e := EnvVar{Key: "MY_ENV", Transform: func(s string) (string, error) { return "", someSentinelErr }}
		v, err := e.getValue()
		assert.ErrorIs(t, err, someSentinelErr)
		assert.Equal(t, "", v)
	})

	t.Run("env set - transform signals not applicable", func(t *testing.T) {
		t.Setenv("MY_ENV", "something")
		e := EnvVar{Key: "MY_ENV", Transform: func(s string) (string, error) { return "", nil }}
		v, err := e.getValue()
		assert.NoError(t, err)
		assert.Equal(t, "", v)
	})
}
