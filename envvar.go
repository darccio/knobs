package knobs

import (
	"os"
	"strconv"
	"strings"
)

// EnvVar represents an env var and an optional transform for remapping the value set at the env var
type EnvVar[T any] struct {
	key       string
	transform func(s string) (T, bool)
}

func NewEnvVar[T any](key string, transform func(s string) (T, bool)) (zero EnvVar[T]) {
	if key == "" {
		// TODO: log something? return an error?
		return zero
	}
	return EnvVar[T]{
		key:       key,
		transform: transform,
	}
}

// The following variables are transform functions for converting string values to other basic data types
// Their purpose is to make creating non-string EnvVars easier and cleaner, e.g. NewEnvVar("MY_VAR", ToInt)
var (
	ToInt = func(s string) (int, bool) {
		// TODO: Determine whether we want to accept floats into ints with this function; currently fails on input like "1.0"
		if vv, err := strconv.Atoi(s); err == nil {
			return vv, true
		} else {
			return vv, false
		}
	}
	ToFloat64 = func(s string) (float64, bool) {
		if vv, err := strconv.ParseFloat(s, 64); err == nil {
			return vv, true
		} else {
			return vv, false
		}
	}
	ToBool = func(s string) (bool, bool) {
		if vv, err := strconv.ParseBool(s); err == nil {
			return vv, true
		} else {
			return vv, false
		}
	}
	// ToString is mainly for documentation purposes
	ToString = func(s string) (string, bool) {
		return s, true
	}
)

// getValue	returns the value set at the env var of e.Key, if set, with whitespace trimmed
// if e.Transform is not nil, the value is passed through transform first before getting returned
func (e *EnvVar[T]) getValue() (zero T, ok bool) {
	v, ok := os.LookupEnv(e.key)
	if !ok {
		return zero, false
	}
	v = strings.TrimSpace(v)
	if e.transform != nil {
		return e.transform(v)
	}
	if _, ok := any(zero).(string); ok {
		return any(v).(T), true
	}
	return zero, false
}
