package knobs

import (
	"os"
	"strings"
)

// EnvVar represents an env var and an optional transform for remapping the value set at the env var
type EnvVar struct {
	Key string
	// Transform, if set, remaps the raw (trimmed) env var value before it is used. Returning
	// ("", nil) signals "this var doesn't apply" -- treated the same as the env var being unset
	// -- and is not an error. A non-nil error is logged by the caller and the var is skipped.
	// Transform must not call Get/GetScope/Set/SetScope on the knob currently being initialized
	// (see Definition.Parse's doc for why); it runs at most once per (knob, scope) pair, and a
	// panic propagates to the caller of Get/GetScope.
	Transform func(s string) (string, error)
}

// getValue returns the value set at the env var of e.Key, with whitespace trimmed, and applies
// e.Transform to it if set. An unset env var, or one that trims to empty, is reported as ("", nil)
// -- not an error -- since an empty value means "this var doesn't apply", the same signal
// e.Transform may itself send by returning ("", nil). A non-nil error is returned only when
// e.Transform itself fails, so the caller can log that failure distinctly instead of silently
// treating a broken transform the same as an unset var.
func (e *EnvVar) getValue() (string, error) {
	v, ok := os.LookupEnv(e.Key)
	if !ok {
		return "", nil
	}
	v = strings.TrimSpace(v)
	if e.Transform == nil {
		return v, nil
	}
	return e.Transform(v)
}
