package knobs

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLogFnConcurrentAccess is the regression test for the data race between
// SetLogger (writer) and knob initializers reading the package logger
// (readers) via logf. It must remain clean under `go test -race`.
//
// This test mutates global logger state, so it is intentionally NOT run in
// parallel with other tests (t.Parallel() is not called here).
func TestLogFnConcurrentAccess(t *testing.T) {
	// Restore the silencer installed by TestMain once this test is done, so
	// later tests aren't affected by whichever logger a writer goroutine
	// happened to install last.
	defer SetLogger(func(string, ...interface{}) {})

	const (
		numKnobs   = 50
		numWriters = 8
	)

	envKey := "KNOBS_RACE_TEST_VAR"
	t.Setenv(envKey, "some-value")

	knobs := make([]Knob[string], numKnobs)
	for i := 0; i < numKnobs; i++ {
		def := &Definition[string]{
			Default: "default",
			EnvVars: []EnvVar{{Key: envKey}},
			// Parse is intentionally left nil: this drives the initializer
			// into the "missing Parse function" logf call path.
		}
		knobs[i] = Register(def)
	}

	stop := make(chan struct{})

	var writers sync.WaitGroup
	for i := 0; i < numWriters; i++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					SetLogger(func(string, ...interface{}) {})
				}
			}
		}()
	}

	var readers sync.WaitGroup
	for i := 0; i < numKnobs; i++ {
		readers.Add(1)
		go func(k Knob[string]) {
			defer readers.Done()
			_ = Get(k)
		}(knobs[i])
	}

	readers.Wait()
	close(stop)
	writers.Wait()
}

// TestSetLoggerNilFallsBackToDefault verifies that resetting the logger with
// SetLogger(nil) does not leave a stored pointer to a nil func, which would
// panic the next time logf is invoked.
func TestSetLoggerNilFallsBackToDefault(t *testing.T) {
	defer SetLogger(func(string, ...interface{}) {})

	SetLogger(nil)

	envKey := "KNOBS_NIL_LOGGER_TEST_VAR"
	t.Setenv(envKey, "some-value")

	def := &Definition[string]{
		Default: "default",
		EnvVars: []EnvVar{{Key: envKey}},
		// Parse is intentionally left nil to drive the initializer into a
		// logf call.
	}
	knob := Register(def)

	require.NotPanics(t, func() {
		_ = Get(knob)
	})
}

// TestSetLoggerCustomReceivesMessage confirms that a custom logger installed
// via SetLogger actually receives log messages produced by the knob
// initializer, and that the "knobs: " prefix is present in the message.
func TestSetLoggerCustomReceivesMessage(t *testing.T) {
	defer SetLogger(func(string, ...interface{}) {})

	var (
		mu       sync.Mutex
		messages []string
	)
	SetLogger(func(format string, args ...interface{}) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	})

	envKey := "KNOBS_CUSTOM_LOGGER_TEST_VAR"
	t.Setenv(envKey, "some-value")

	def := &Definition[string]{
		Default: "default",
		EnvVars: []EnvVar{{Key: envKey}},
		// Parse is intentionally left nil to drive the initializer into a
		// logf call.
	}
	knob := Register(def)

	_ = Get(knob)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, messages, 1)
	require.Contains(t, messages[0], "knobs: ")
	require.Contains(t, messages[0], "missing Parse function")
}
