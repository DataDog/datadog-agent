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
   - `--with-lifecycle`: include `compdef.Lifecycle` in the Requires struct
   - `--with-mock`: also generate a `mock/` sub-package

2. **Ask the user** (if not provided via arguments):
   - What is the component's interface? (what methods should it expose?)
   - Which team owns it? (for the `// team:` comment)
   - Does it need lifecycle hooks (start/stop)?
   - Does it need a Params struct?
   - What are its dependencies (other components it requires)?

3. **Read reference examples** before writing any code. Find a recent component under `comp/` using the `def/fx/impl` pattern (e.g. `comp/core/remoteagentregistry/`). Read:
   - `def/component.go` — interface definition with `// team:` comment
   - `fx/fx.go` — Module() with `fxutil.ProvideComponentConstructor`
   - `impl/<name>.go` — Requires/Provides structs, NewComponent constructor

4. **Create the directory structure** under `comp/<bundle>/<component>/`:
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

5. **Create each file** following the patterns from the reference. Key rules:
   - `def/component.go`: Package name = component name, include `// team:` comment, only interfaces
   - `fx/fx.go`: Use `fxutil.ProvideComponentConstructor` (NEVER raw `fx.Provide`), returns `fxutil.Module`
   - `impl/<component>.go`: Plain Go constructor `func NewComponent(deps Requires) (Provides, error)`, **no** `fx.In`/`fx.Out`/`compdef.In`/`compdef.Out` embedding in Requires/Provides, unexported implementation type
   - `go.mod` files: Use `v0.0.0` for inter-module dependencies, match Go version from root `go.mod`

6. **Register the modules** in `modules.yml` — add entries for `def`, `fx`, and `impl` (use `default` or `used_by_otel: true`).

7. **Run `dda inv create-module --path=comp/<bundle>/<component>/def`** (and for `fx`, `impl`) or manually add to `modules.yml` and run `dda inv tidy`.

8. **Wire into a bundle** if appropriate — add the component's `Module()` to the relevant `comp/<bundle>/bundle.go`.

9. **Validate**:
   ```bash
   dda inv lint-components lint-fxutil-oneshot-test github.lint-codeowner
   ```
   Fix any errors and re-run until clean.

## Critical Rules (New-Style vs Legacy)

**DO**: `def/fx/impl` sub-packages, `fxutil.ProvideComponentConstructor`, plain Go constructor, plain Requires/Provides structs, thin def package.

**DON'T**: Single directory, `fx.Provide(newComponent)`, `fx.In`/`fx.Out` embedding, implementation in def package.

## Usage

- `/create-component core/myfeature --team agent-runtimes --with-lifecycle`
- `/create-component metadata/hostinfo --team agent-metrics --with-params`
