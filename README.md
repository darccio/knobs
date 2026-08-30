# knobs

`knobs` is a declarative, unobtrusive configuration system for Go: declare a configuration once as
a typed `Knob`, and read it anywhere without threading a config struct through every call site.

## Install

```
go get github.com/darccio/knobs
```

## Usage

Declare a knob once, typically as a package-level variable:

```go
var MaxRetries = knobs.Register(&knobs.Definition[int]{
	Default: 3,
	EnvVars: []knobs.EnvVar{{Key: "MAX_RETRIES"}},
	Parse:   knobs.ToInt,
})
```

Read it anywhere:

```go
retries := knobs.Get(MaxRetries)
```

Resolution -- checking `MAX_RETRIES`, parsing it with `ToInt` -- happens lazily, on the first
`Get`, not at `Register` time. If the env var is unset, or fails to parse, the knob reads as its
`Default`.

### Overriding a value

```go
knobs.Set(MaxRetries, knobs.Code, 5)
```

Every `Set` is tagged with an `Origin`. `Code` is always allowed; `Default` never is (it's a
baseline, not an external source); any other `Origin`, including the built-in `Env`, is only
allowed if it's listed in the `Definition`'s `Origins` field. Applications that push configuration
from more than one place typically define their own `Origin` values past `knobs.Code`.

### Derived knobs

```go
requestMaxRetries := knobs.Derive(MaxRetries)
knobs.Set(requestMaxRetries, knobs.Code, 10) // only this request's knob changes
```

A derived knob falls through to its parent's current value until it's given one of its own -- a
live fallthrough, so it keeps tracking the parent if the parent changes and the derived knob was
never overridden.

### Scopes

`Get`/`Set` operate on a package-wide default scope. `GetScope`/`SetScope` take an explicit
`*knobs.Scope`, so configuration can be isolated per request, per tenant, or per test:

```go
sc := knobs.NewScope()
knobs.SetScope(sc, MaxRetries, knobs.Code, 1)
knobs.GetScope(sc, MaxRetries) // 1, independent of the default scope
```

See the package documentation (`go doc github.com/darccio/knobs`) for the full lifecycle,
`Origins` semantics, and the `Resolve`/`Parse`/`Transform` callback contract.

## Breaking changes since the initial proof of concept

- `Scope` no longer exports `Lock`/`Unlock`/`RLock`/`RUnlock`.
- `DeriveScope` is removed; `Derive` now resolves correctly in every `Scope`, including ones
  created after the derive.
- `Knob[T]` is now an opaque type; it can no longer be constructed or read as a plain `int`.

There are no tagged releases, so no compatibility guarantee is broken by these changes.

## Status

This library started as a proof of concept and has since been substantially hardened for
correctness and concurrency safety. There are no tagged releases yet, so breaking changes may
still land as the design settles further.
