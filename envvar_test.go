package knobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringEnvVar(t *testing.T) {
	e := NewEnvVar("MY_ENV", ToString)
	t.Run("env unset", func(t *testing.T) {
		v, ok := e.getValue()
		var zero string
		assert.Equal(t, zero, v)
		assert.False(t, ok)
	})
	t.Run("env set", func(t *testing.T) {
		t.Setenv("MY_ENV", "value")
		v, ok := e.getValue()
		assert.Equal(t, "value", v)
		assert.True(t, ok)
	})
}

func TestIntEnvVar(t *testing.T) {
	e := NewEnvVar("MY_ENV", ToInt)
	var zero int
	t.Run("env unset", func(t *testing.T) {
		v, ok := e.getValue()
		assert.Equal(t, zero, v)
		assert.False(t, ok)
	})
	t.Run("env set", func(t *testing.T) {
		t.Setenv("MY_ENV", "1")
		v, ok := e.getValue()
		assert.Equal(t, 1, v)
		assert.True(t, ok)
	})
	t.Run("transformation invalid", func(t *testing.T) {
		t.Setenv("MY_ENV", "not-an-int")
		v, ok := e.getValue()
		assert.Equal(t, zero, v)
		assert.False(t, ok)
	})
}

func TestFloat64EnvVar(t *testing.T) {
	e := NewEnvVar("MY_ENV", ToFloat64)
	var zero float64
	t.Run("env unset", func(t *testing.T) {
		v, ok := e.getValue()
		assert.Equal(t, zero, v)
		assert.False(t, ok)
	})
	t.Run("env set", func(t *testing.T) {
		t.Setenv("MY_ENV", "1.0")
		v, ok := e.getValue()
		assert.Equal(t, 1.0, v)
		assert.True(t, ok)
	})
	t.Run("transformation invalid", func(t *testing.T) {
		t.Setenv("MY_ENV", "not-a-float")
		v, ok := e.getValue()
		assert.Equal(t, zero, v)
		assert.False(t, ok)
	})
}

func TestBoolEnvVar(t *testing.T) {
	e := NewEnvVar("MY_ENV", ToBool)
	var zero bool
	t.Run("env unset", func(t *testing.T) {
		v, ok := e.getValue()
		assert.Equal(t, zero, v)
		assert.False(t, ok)
	})
	t.Run("env set", func(t *testing.T) {
		t.Setenv("MY_ENV", "true")
		v, ok := e.getValue()
		assert.Equal(t, true, v)
		assert.True(t, ok)
	})
	t.Run("transformation invalid", func(t *testing.T) {
		t.Setenv("MY_ENV", "not-a-bool")
		v, ok := e.getValue()
		assert.Equal(t, zero, v)
		assert.False(t, ok)
	})
}

func TestEnvVarCustomTransform(t *testing.T) {
	transform := func(v string) (zero int32, ok bool) {
		if v == "special_value" {
			return int32(100), true
		}
		return zero, ok
	}
	e := NewEnvVar("MY_ENV", transform)
	var zero int32
	t.Run("env unset", func(t *testing.T) {
		v, ok := e.getValue()
		assert.Equal(t, zero, v)
		assert.False(t, ok)
	})
	t.Run("env set", func(t *testing.T) {
		t.Setenv("MY_ENV", "special_value")
		v, ok := e.getValue()
		assert.Equal(t, int32(100), v)
		assert.True(t, ok)
	})
	t.Run("transformation invalid", func(t *testing.T) {
		t.Setenv("MY_ENV", "not-a-bool")
		v, ok := e.getValue()
		assert.Equal(t, zero, v)
		assert.False(t, ok)
	})
}
