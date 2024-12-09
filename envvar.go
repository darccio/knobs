package knobs

import (
	"os"
	"strings"
)

// EnvVar represents an env var and an optional transform for remapping the value set at the env var
type EnvVar struct {
	key       string
	transform func(s string) (string, bool)
}

// getValue	returns the value set at the env var of e.Key, if set, with whitespace trimmed
// if e.Transform is not nil, the value is passed through transform first before getting returned
func (e *EnvVar) getValue() (string, bool) {
	v, ok := os.LookupEnv(e.key)
	if !ok {
		return "", false
	}
	v = strings.TrimSpace(v)
	if e.transform != nil {
		return e.transform(v)
	}
	return v, true
}
