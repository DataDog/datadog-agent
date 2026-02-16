---
name: create-component
description: Create a new Fx component using the modern def/fx/impl pattern (NOT legacy)
allowed-tools: Bash, Read, Write, Edit, Glob, Grep
argument-hint: "<bundle>/<component-name> [--team team-name] [--with-params] [--with-lifecycle] [--with-mock]"
---

Create a new Fx component following the **modern** (new-style) pattern with separate `def/`, `fx/`, and `impl/` sub-packages.

**IMPORTANT**: NEVER use the legacy pattern (single-directory with `fx.Provide` directly). Always use the new-style pattern described below.

## Instructions

1. **Parse `$ARGUMENTS`** to determine:
   - `<bundle>/<component-name>`: e.g., `core/remoteflags` means bundle=core, component=remoteflags
   - `--team <team-name>`: optional team ownership tag (default: ask the user)
   - `--with-params`: include a `Params` struct in the def package
   - `--with-lifecycle`: include `compdef.Lifecycle` in the Requires struct (for start/stop hooks)
   - `--with-mock`: also generate a `mock/` sub-package

2. **Ask the user** (if not provided via arguments):
   - What is the component's interface? (what methods should it expose?)
   - Which team owns it? (for the `// team:` comment)
   - Does it need lifecycle hooks (start/stop)?
   - Does it need a Params struct?
   - What are its dependencies (other components it requires)?

3. **Create the directory structure** under `comp/<bundle>/<component>/`:

```
comp/<bundle>/<component>/
├── def/
│   ├── go.mod
│   ├── component.go     # Interface definition + team tag
│   └── params.go        # (optional) Params struct
├── fx/
│   ├── go.mod
│   └── fx.go            # Module() function
└── impl/
    ├── go.mod
    └── <component>.go   # Requires, Provides, NewComponent()
```

4. **Create each file** following the templates below.

5. **Register the modules** in `modules.yml` — add entries for `comp/<bundle>/<component>/def`, `comp/<bundle>/<component>/fx`, and `comp/<bundle>/<component>/impl` (and `comp/<bundle>/<component>/mock` if applicable).

6. **Run `dda inv modules.add-all-replace`** to generate the `replace` directives in all `go.mod` files. Then run `dda inv tidy` to tidy everything.

7. **Wire into a bundle** if appropriate — add the component's `Module()` to the relevant `comp/<bundle>/bundle.go`. If the component needs a `Params`, add a `fx.Provide` bridge from `BundleParams` to the component's `Params` type.

8. **Validate the component** by running the linters that check component correctness:
   ```bash
   dda inv lint-components lint-fxutil-oneshot-test github.lint-codeowner
   ```
   These linters verify the component structure, fxutil usage, and CODEOWNERS entries. Fix any errors they report and re-run until clean — this is your feedback loop to ensure the component is correct.

## File Templates

### `def/component.go` — Interface definition

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package <component> defines the interface for the <component> component.
package <component>

// team: <team-name>

// Component is the component type.
type Component interface {
	// <exported methods here>
}
```

Key rules:
- Package name = component name (e.g., `package remoteflags`)
- Include the `// team:` comment right after the package declaration
- Only define the interface here, no implementation details
- Keep imports minimal (ideally none or just standard library)

### `def/params.go` — Parameters (optional)

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package <component>

// Params defines the parameters for the <component> component.
type Params struct {
	// <fields here>
}
```

### `def/go.mod`

```
module github.com/DataDog/datadog-agent/comp/<bundle>/<component>/def

go 1.24.0
```

Keep this module as thin as possible — the def package should have minimal dependencies.

### `fx/fx.go` — Fx wiring

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx defines the fx options for this component.
package fx

import (
	<component>impl "github.com/DataDog/datadog-agent/comp/<bundle>/<component>/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			<component>impl.NewComponent,
		),
	)
}
```

Key rules:
- **Always use `fxutil.ProvideComponentConstructor`**, NEVER raw `fx.Provide`
- Import the impl package with an alias like `<component>impl`
- The `Module()` function returns `fxutil.Module` (not `fx.Module`)
- This file should be very short — just wiring, no logic

### `fx/go.mod`

```
module github.com/DataDog/datadog-agent/comp/<bundle>/<component>/fx

go 1.24.0

require (
	github.com/DataDog/datadog-agent/comp/<bundle>/<component>/impl v0.0.0
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.0.0
)
```

### `impl/<component>.go` — Implementation

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package <component>impl implements the <component> component.
package <component>impl

import (
	<component>def "github.com/DataDog/datadog-agent/comp/<bundle>/<component>/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// Requires defines the dependencies for the <component> component.
type Requires struct {
	Lc compdef.Lifecycle // include only if --with-lifecycle or if the component needs start/stop hooks

	// Add other component dependencies here, e.g.:
	// Config config.Component
	// Log    logdef.Component
}

// Provides defines the output of the <component> component.
type Provides struct {
	Comp <component>def.Component
}

// <unexportedType> implements the Component interface.
type <unexportedType> struct {
	// internal fields
}

// NewComponent creates a new <component> component.
func NewComponent(deps Requires) (Provides, error) {
	instance := &<unexportedType>{
		// initialize fields from deps
	}

	// If using lifecycle hooks:
	// deps.Lc.Append(compdef.Hook{
	//     OnStart: func(ctx context.Context) error { return instance.start(ctx) },
	//     OnStop:  func(ctx context.Context) error { return instance.stop(ctx) },
	// })

	return Provides{
		Comp: instance,
	}, nil
}

// Implement the Component interface methods on the unexported type
```

Key rules:
- Package name = `<component>impl` (e.g., `package remoteflagsimpl`)
- The `Requires` struct lists dependencies using their `def` types. Do NOT embed `compdef.In` — `ProvideComponentConstructor` handles that automatically via reflection
- The `Provides` struct lists what the component provides. Do NOT embed `compdef.Out` — same reason
- Constructor signature is `func NewComponent(deps Requires) (Provides, error)` — plain Go, no fx types
- The implementation type is **unexported** (e.g., `type remoteFlagsImpl struct`)
- Import the def package with an alias: `<component>def`
- Import `compdef "github.com/DataDog/datadog-agent/comp/def"` for `Lifecycle` and `Hook`

### `impl/go.mod`

```
module github.com/DataDog/datadog-agent/comp/<bundle>/<component>/impl

go 1.24.0

require (
	github.com/DataDog/datadog-agent/comp/<bundle>/<component>/def v0.0.0
	github.com/DataDog/datadog-agent/comp/def v0.0.0
)
```

## Critical Rules (NEW-STYLE vs LEGACY)

### DO (new-style):
- Use `def/`, `fx/`, `impl/` sub-packages, each with its own `go.mod`
- Use `fxutil.ProvideComponentConstructor` in `fx/fx.go`
- Write plain Go constructor: `func NewComponent(deps Requires) (Provides, error)`
- Keep `Requires`/`Provides` as plain structs (no `fx.In`/`fx.Out` embedding)
- Keep the def package thin (just interfaces and params)

### DON'T (legacy):
- Put everything in a single directory with one `go.mod`
- Use `fx.Provide(newComponent)` directly
- Embed `fx.In` or `fx.Out` in your structs
- Put implementation details in the def package
- Use `compdef.In` or `compdef.Out` in Requires/Provides (these are marker types only used internally by `ProvideComponentConstructor`)

## modules.yml Registration

Add entries under the `modules:` key:

```yaml
  comp/<bundle>/<component>/def: default
  comp/<bundle>/<component>/fx: default
  comp/<bundle>/<component>/impl: default
```

Use `default` for standard modules, or `used_by_otel: true` if the component is used by the OTel collector.

## Bundle Wiring

If the component should be part of a bundle (e.g., `comp/core/bundle.go`):

```go
import (
    <component>fx "github.com/DataDog/datadog-agent/comp/<bundle>/<component>/fx"
)

func Bundle() fxutil.BundleOptions {
    return fxutil.Bundle(
        // ... existing components ...

        // If the component has Params:
        fx.Provide(func(params BundleParams) <component>def.Params { return params.<Component>Params }),
        <component>fx.Module(),
    )
}
```

If the component does NOT have Params, just add `<component>fx.Module()` without the `fx.Provide` bridge.

## Usage

- `/create-component core/myfeature --team agent-runtimes --with-lifecycle`
- `/create-component metadata/hostinfo --team agent-metrics --with-params`
- `/create-component core/remoteflags --team remote-config --with-lifecycle --with-mock`

## Output

After creating all files, summarize what was created, then:
1. Run `dda inv modules.add-all-replace` to generate replace directives
2. Run `dda inv tidy` to tidy go modules
3. Run `dda inv lint-components lint-fxutil-oneshot-test github.lint-codeowner` to validate the component — fix any errors and re-run until clean
4. Remind the user to implement the interface methods on the unexported type in `impl/`
5. Wire the component into the appropriate bundle if needed
