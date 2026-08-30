// Package knobs provides a declarative, unobtrusive configuration system.
//
// # Lifecycle
//
// A configuration value is declared once via Register, which returns an opaque, comparable Knob
// handle:
//
//	var MaxRetries = knobs.Register(&knobs.Definition[int]{
//		Default: 3,
//		EnvVars: []knobs.EnvVar{{Key: "MAX_RETRIES"}},
//		Parse:   knobs.ToInt,
//	})
//
// Registering a Definition does not read the environment or run any callback -- resolution is
// lazy, happening once per (Knob, Scope) pair on the first call to Get or GetScope for that pair.
// Resolution walks the Definition's EnvVars in order, taking the first one that is set (later ones
// are logged as shadowed, not silently ignored), optionally validated/redirected through Resolve,
// then converted with Parse. If no EnvVar is set, or the found value fails to Resolve or Parse,
// the knob reads as its Default. A knob's value can later be overridden directly with Set or
// SetScope, subject to the Origins rule below.
//
// A Knob's identity is independent of any Scope: the same Knob can be read from and written to
// multiple Scopes, each holding its own independent value. Get/Set operate on the package-wide
// DefaultScope; GetScope/SetScope take an explicit *Scope, letting callers isolate configuration
// per request, per tenant, or per test.
//
// # Derived knobs
//
// Derive creates a new Knob that falls through to its parent's current value until it is given a
// value of its own via Set -- a live fallthrough, not a one-time snapshot, so a derived knob sees
// values set on its parent after the derive. This is useful for layered overrides: a derived knob
// can be set for a single request or tenant while everything else continues to read the parent's
// (possibly also overridden) value.
//
// # Origins
//
// Every Set/SetScope call is tagged with an Origin, which is both provenance (retrievable via
// GetWithOrigin/GetWithOriginScope) and an access-control gate: Code is always allowed; Default is
// never allowed, since it represents a knob's own baseline rather than an external source
// asserting one; any other Origin, including the built-in Env, is allowed only if the
// Definition's Origins field listed it. A knob derived via Derive inherits its allowed Origins
// from its root at Derive time. Applications that push configuration from multiple systems
// (environment variables, a remote config service, request-scoped overrides) typically define
// their own Origin constants past Code, so a value's source stays identifiable.
//
// # Callback contracts
//
// Definition's Resolve and Parse, and EnvVar's Transform, share the same contract:
//
//   - Each runs at most once per (Knob, Scope) pair, on that pair's first access -- guarded by a
//     non-reentrant sync.Once. A callback must not call Get, GetScope, Set, or SetScope on the
//     knob it is currently initializing: doing so deadlocks, since the guarding Once is already
//     held by the outer call. Calling those functions on a different, already-registered knob is
//     safe.
//   - If a callback panics, the panic propagates to the caller of Get/GetScope. Because sync.Once
//     treats a panicking call as complete, the callback never runs again for that (Knob, Scope)
//     pair -- the knob is left permanently readable at its Default for that pair, since the
//     Default is committed before any callback runs.
package knobs
