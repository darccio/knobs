package knobs

import (
	"os"
	"strings"
)

// EnvVar represents an env var and an optional transform for remapping the value set at the env var
type EnvVar struct {
	Key       string
	Transform func(s string) string
}

// getValue	returns the value set at the env var of e.Key, if set, with whitespace trimmed
// if e.Transform is not nil, the value is passed through transform first before getting returned
func (e *EnvVar) getValue() string {
	v, ok := os.LookupEnv(e.Key)
	if !ok {
		return ""
	}
	v = strings.TrimSpace(v)
	if e.Transform != nil {
		return e.Transform(v)
	}
	return v
}
