package knobs

import (
	"log"
	"sync/atomic"
)

// loggerFunc is an alias (=), not a defined type, so SetLogger's exported
// signature stays identical to func(string, ...interface{}).
type loggerFunc = func(string, ...any)

var logFn atomic.Pointer[loggerFunc]

// SetLogger sets the package logger. Passing nil restores the default (log.Printf).
// Safe to call concurrently with any other knobs function.
func SetLogger(fn loggerFunc) {
	if fn == nil {
		logFn.Store(nil)
		return
	}
	logFn.Store(&fn)
}

func logf(format string, args ...any) {
	fn := log.Printf
	if p := logFn.Load(); p != nil {
		fn = *p
	}
	fn(format, args...)
}
