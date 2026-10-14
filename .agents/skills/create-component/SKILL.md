---
name: create-component
description: Create a new Fx component using the modern def/fx/impl pattern (NOT legacy)
allowed-tools: Bash, Read, Write, Edit, Glob, Grep
argument-hint: "<bundle>/<component-name> [--team team-name] [--with-params] [--with-lifecycle] [--with-mock]"
model: sonnet
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
   │   ├── component.go     # Interface definition + team tag
   │   └── params.go        # (optional) Params struct
   ├── fx/
   │   └── fx.go            # Module() function
   └── impl/
       └── <component>.go   # Requires, Provides, NewComponent()
   ```

5. **Create each file** following the patterns from the reference. Key rules:
   - `def/component.go`: Use the component name as the package name, include the `// team:` comment, and define the interface and its public types here.
   - `fx/fx.go`: Use `fxutil.ProvideComponentConstructor` (NEVER raw `fx.Provide`), returns `fxutil.Module`
   - `impl/<component>.go`: Use a plain Go constructor returning `Provides` or `(Provides, error)` and an unexported implementation type. Prefer plain exported `Requires` and `Provides` structs for new constructors. The constructor's outer dependency and result structs do not need `compdef.In` or `compdef.Out`. A nested struct needs the corresponding marker when its fields should be resolved as individual dependencies or exposed as individual results. Do not embed `fx.In` or `fx.Out` in the structs passed to the adapter.

6. **Choose module boundaries only when external consumers need them.** Components can use the repository's existing modules. For external consumers, create separate modules for the required `def` and implementation packages. Follow the [component module guidelines](../../../doc/guidelines/components.md#go-modules) for new components: do not create a module at the component root or in an Fx wrapper package. Existing components may have different module layouts.

7. **If new modules are needed, follow the `create-go-module` skill for each selected path.** Register only those modules in `modules.yml`, use the repository's Go version, and run `dda inv tidy` through that workflow.

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
