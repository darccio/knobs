package knobs

import "log"

var (
	// logFn is the logger used by the package. It defaults to log.Printf.
	logFn func(string, ...interface{}) = log.Printf
)

func SetLogger(fn func(string, ...interface{})) {
	logFn = fn
}
