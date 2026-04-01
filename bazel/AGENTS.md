# Bazel in the Datadog Agent

Bazel is the primary build system for this repository. This file covers operational patterns, common pitfalls, and
conventions for working with Bazel in this codebase. The repo targets **Bazel 9** and uses **Bzlmod** exclusively.

## Tooling

Use `bazelisk` (or the `bazel` symlink it installs) to invoke Bazel. Bazelisk automatically selects the version
specified in `.bazelversion`. Never invoke a pinned `bazel` binary directly — the version must match.

```sh
# Format and lint all BUILD/.bzl files
bazel run //bazel/buildifier

# Update the lockfile after any MODULE.bazel change
bazel mod deps

# Enable the internal remote cache (Datadog network only)
# Add to user.bazelrc at the workspace root (gitignored):
echo 'common --config=cache' >> user.bazelrc
```

The `.bazelrc` is managed by `@DataDog/agent-build`. Do not edit it without their review. Per-user options belong in
`user.bazelrc`, which is `.gitignore`d and auto-imported via `try-import %workspace%/user.bazelrc`.

## Exit codes — CRITICAL

Always check the exit code explicitly. A zero exit means success; anything non-zero is an error. The most commonly
missed codes:

| Code | Meaning |
|------|---------|
| 0 | success |
| 1 | build/test failed |
| 2 | bad flags or command syntax |
| 3 | **build succeeded but some tests failed or timed out** |
| 4 | build succeeded but no tests were found |

Exit code 3 is the main trap: build succeeded but tests failed — scripts that check `[ $? -eq 1 ]` would miss it.

## Build phases

A build has three phases. Understanding them prevents a whole class of mistakes:

- **Loading phase** — BUILD files and `.bzl` files are evaluated. Macros expand. Rules are instantiated but not
  executed. No actions run, no files are read except BUILD/.bzl. `bazel query` operates at this level.
- **Analysis phase** — Rule `implementation` functions run. Actions are *registered* (not yet executed). The action
  graph is built. `bazel cquery` and `bazel aquery` operate at this level.
- **Execution phase** — Actions whose outputs are needed are executed in dependency order. Tests run here.

Implications: you cannot read arbitrary files or run tools during loading or analysis. Any file I/O must be expressed as
an action (execution phase) or as a repository rule / module extension (fetch time).

## Bazelrc and per-user config

Load order: system rc → workspace rc (`.bazelrc`) → home rc (`~/.bazelrc`) → `BAZELRC` env var → `--bazelrc=` flags.

The workspace `.bazelrc` ends with:
```
try-import %workspace%/user.bazelrc
```

Add personal overrides (cache config, local tweaks) to `user.bazelrc` at the workspace root. This file is `.gitignore`d.
Personal `--config` names should start with `_` to avoid conflicts.

## Bazel client/server

Bazel runs as a long-lived server per output base (one per workspace + user). Each server holds the **analysis cache** —
the loaded BUILD graph and analysis results. Losing the server loses the cache, forcing a full cold reload.

Analysis cache invalidation traps:
- Changing flags between invocations discards it (e.g., mixing `bazel build -c opt` with `bazel cquery` invalidates
  both ways).
- Pressing Ctrl-C **multiple times** kills the server. Press it once to request a graceful shutdown.
- Use `--noallow_analysis_cache_discard` (Bazel 6.4+) to turn cache discard into an error during development.
- Use separate `--output_base` directories when you need multiple flag sets simultaneously.

In scripts:
- Add `--max_idle_secs=5` to avoid accumulating idle servers across many automated invocations.
- Run `bazel shutdown` when the script is done.
- If a script needs concurrent Bazel runs, use separate `--output_base` directories.

## `bazel clean` — never use it

`bazel clean` discards the incremental build state and is almost never the right solution. Instead:

- For suspected stale outputs: run the build again; Bazel's action graph is correct if all deps are declared.
- For server issues: `bazel shutdown` then rebuild.
- For cache corruption (very rare): investigate the root cause rather than wiping state.

## MODULE.bazel and dependency management

This repo uses **Bzlmod** (MODULE.bazel). WORKSPACE is fully removed in Bazel 9 — never add WORKSPACE-based patterns.

- After any change to `MODULE.bazel` (adding/updating a `bazel_dep`, using a new module extension tag, etc.), run
  `bazel mod deps` to regenerate `MODULE.bazel.lock`.
- `MODULE.bazel.lock` **must be committed** to version control. It is the source of truth for reproducible builds;
  never delete or ignore it.
- Never resolve lockfile conflicts manually (except `registryFileHashes` sections, which are safe). Instead: reset to
  one side, fix `MODULE.bazel`, then re-run `bazel mod deps`.
- Bazel uses **Minimal Version Selection** (MVS, like Go): no version ranges, no "latest" aliases. The root module
  controls final versions through `single_version_override` / `multiple_version_override`.
- `MODULE.bazel` does not support `load()`. Use `include()` (root module only) to split a large file, and module
  extensions for complex logic.
- Keep `use_repo(...)` lists accurate. Run `bazel mod tidy` after extension changes to update them automatically.
- `bazel mod explain <module>` shows why a version is selected. `bazel mod graph` visualises the full dependency graph.
- In CI, pass `--lockfile_mode=error` to fail the build if the lockfile would need updating — prevents stale
  lockfiles from silently merging. Only `registryFileHashes` sections are safe to resolve manually in merge conflicts.

## Module extensions

Module extensions are the Bzlmod equivalent of WORKSPACE macros — they generate repos by reading tags from across the
dependency graph. The `go_deps` extension from `gazelle` and the `rules_python` pip extension are the main ones in this
repo.

Usage pattern in `MODULE.bazel`:

```python
go_deps = use_extension("@gazelle//:extensions.bzl", "go_deps")
go_deps.from_file(go_work = "//:go.work")
use_repo(go_deps, "com_github_some_dep", ...)
```

- Extensions are **lazy**: they only run when a repo they generate is actually needed. Force evaluation with
  `bazel mod deps`.
- Extension identity is the combination of the **`.bzl` file path and the exported name**. Re-exporting an extension
  from a different file creates a new identity; both versions may run separately. Keep one canonical import path.
- Only the root module should control repo names generated by an extension; non-root modules naming repos risk
  collisions.
- Generated repos have canonical names in the form `<module>+<extension>+<repo_name>`. This format is internal and
  subject to change — never hardcode it.
- If an extension always produces the same repos given the same inputs, mark it `reproducible = True` in
  `extension_metadata` — this keeps `MODULE.bazel.lock` small and reduces merge conflicts.
- Set `os_dependent = True` / `arch_dependent = True` when the extension result differs by platform.
- Use `override_repo` / `inject_repo` (root module only) to patch or replace repos generated by an extension.
- `bazel mod show_extension <ext>` inspects what repos an extension generates and which modules use them.

## Repository rules

Repository rules (`repository_rule`) generate external repos by running arbitrary logic at fetch time. They are invoked
either from a module extension or directly from `MODULE.bazel` via `use_repo_rule()` (the lighter-weight option when no
tag-based configuration is needed).

```python
my_repo = repository_rule(
    implementation = _impl,
    attrs = {
        "url": attr.string(mandatory = True),
        "sha256": attr.string(mandatory = True),
    },
    environ = ["MY_ENV_VAR"],  # re-fetch when this env var changes
)
```

The implementation runs in the loading phase and may use `repository_ctx` to download files, execute commands, read the
filesystem, and create the repo's contents. Return `None` if the result is fully determined by the attrs (makes the repo
reproducible); return a dict of attrs to pin the result (e.g., replace a branch name with a commit hash).

Re-fetch triggers:
- Attrs change.
- The Starlark implementation code changes.
- A watched env var (via `environ` or `repository_ctx.getenv()`) changes.
- A watched path (via `repository_ctx.watch()`, `repository_ctx.read()`, etc.) changes.
- `bazel fetch --force`.

Two special flags on `repository_rule`:
- `local = True` — also re-fetches on Bazel server restart (for rules that probe the local machine).
- `configure = True` — only re-fetches on `bazel fetch --force --configure` (not plain `--force`).

## BUILD file style

Follow the upstream [BUILD style guide](https://bazel.build/build/style-guide). Highlights:

### Gazelle — the primary BUILD file author

The repo uses a **custom `gazelle_binary`** (defined in the root `BUILD.bazel`) that bundles multiple language
extensions, starting with Go and `go_stringer`. More extensions will be added over time — for other Go-specific
generators and for other languages, sourced from third-party rulesets or written in-repo.

**The workflow for any new package is always:**

```sh
bazel run //:gazelle -- update ./path/to/package   # generate or update BUILD.bazel
bazel run //bazel/buildifier                       # format
```

Do not hand-write `BUILD.bazel` content that Gazelle can infer. A Gazelle extension's job is precisely to keep that
content in sync with source files automatically. When a new language or generator is added to the binary, its rules
become part of the same automated workflow — you do not need to know which extensions are loaded.

Only add rules manually when they express something that no Gazelle extension can derive from source (e.g., integration
test targets that wire together multiple packages, or targets with non-standard attributes).

### Always name files `BUILD.bazel`, never `BUILD`

This repo enforces `BUILD.bazel` exclusively:

- Gazelle is configured with `-build_file_name=BUILD.bazel` and will only read and write files with that name.
- `REPO.bazel` ignores all `**/build` directories via `ignore_directories()`.

Both constraints exist to prevent a collision between `BUILD` (the file) and `build/` (a common output directory name)
on **macOS's case-insensitive filesystem**. When that workspace is bind-mounted into a Linux container — which has a
case-sensitive filesystem — Docker Desktop can expose the two as the same inode, corrupting the build. Using
`BUILD.bazel` sidesteps the collision entirely.

`BUILD` files that somehow still exist in the tree must be renamed to `BUILD.bazel`.

### Formatting and structure

- `buildifier` is mandatory. Run `bazel run //bazel/buildifier` before committing. It is the single source of truth
  for formatting — do not debate style in code review.
- File structure order: package description comment → `load()` statements → `package()` → rules (leaves first).
- Standalone comments (not attached to a specific rule) require an empty line after them; attached comments do not.
- Single blank line between top-level definitions.
- No strict line length limit — labels can be long and tools generate BUILD files.

### Syntax restrictions

BUILD files are more restricted than `.bzl` files:
- No function definitions, no `for` or `if` statements at the top level (list comprehensions and `if` expressions
  are allowed).
- No `*args` or `**kwargs` — list all arguments explicitly.
- No arbitrary I/O — BUILD files must be hermetic.
- Encode files in UTF-8.

### References and labels

- Within the current package: source files use bare names (`"x.cc"`), generated files and rules use `:` prefix
  (`:gen_header`, `:lib`).
- Prefer the short form for eponymous targets: `//x` instead of `//x:x`, `:foo` instead of `//pkg/foo:foo`.
- **Labels must never be split across lines** and must be string literals — automated tools (buildozer, Code Search)
  cannot handle split or computed label values.
- Avoid reserved names: `all`, `__pkg__`, `__subpackages__` have special semantics.

### `load()` statements

- Use a leading `:` for relative loads: `load(":my_rules.bzl", "some_rule")`.
- Aliases are supported: `load(":file.bzl", nice_alias = "some_other_rule")`.
- `.bzl` symbols starting with `_` cannot be loaded from other files.
- Use load visibility to restrict who may load internal `.bzl` files.

### DAMP over DRY

BUILD files are configuration, not code — prioritize readability over deduplication:
- Do not create `COMMON_DEPS` variables or shared lists. List deps explicitly on every target.
- Do not use a `java_library` with `exports` as a dep aggregator.
- Let [Gazelle](https://github.com/bazel-contrib/bazel-gazelle) and buildozer maintain deps. Repetition is fine.
- No list comprehensions at the top level — use a macro or write each target explicitly.
- Prefer literal strings over `%` formatting or `+` concatenation, especially in `name` and `deps`.

### Target naming

- `name` must be a **literal string constant** (not computed) except inside macros — tools find targets by name
  without evaluating code.
- Use `snake_case` generally; `UpperCamelCase` for Java `*_binary` and `*_test` (enables `test_class` inference).
- Name a `cc_library` / `py_library` after its single source file when there is one.
- Suffix test targets with `_test`, `_unittest`, `Test`, or `Tests`.
- Avoid meaningless suffixes like `_lib` or `_library` unless needed to disambiguate from a `_binary`.
- Proto targets: `_proto` for `proto_library`, `_cc_proto` for `cc_proto_library`, `_java_proto` for
  `java_proto_library`.
- Do not create an eponymous target (same name as the directory) unless it genuinely describes the package's purpose.
- Use boolean values (`True`/`False`) for boolean attributes, not `0`/`1`.

### Dependencies and globs

- Declare only **direct** deps. Relying on transitive deps is a layering violation and breaks strict dep checking.
- **No recursive globs** (`glob(["**/*.java"])`): they skip subdirectories containing BUILD files, are harder to
  reason about, and defeat remote caching and parallelism. Put a BUILD file in each directory instead.
- Use `glob(["testdata/**"])` (not bare directory labels) in `data`. A directory label makes the target depend on the
  directory *node*, not its contents, breaking incremental builds.
- Use `[]` to express "no targets", not a glob that matches nothing.

## Starlark language

Starlark is Python-like but with deliberate restrictions for hermeticity and parallelism. Key divergences:

- No recursion, no `while`, no `yield`, no `class`, no `import` (use `load`), no `try`/`except`/`finally`.
- No float or set types. No generator expressions. No implicit string concatenation.
- `for` and `if` statements are not allowed at the **top level** of a file — only inside functions. In BUILD files
  list comprehensions are allowed at top level.
- Global variables become **immutable** once the file finishes loading. Values loaded from another `.bzl` file are
  frozen immediately — calling a function that modifies a loaded list is a runtime error.
- `int` is 32-bit signed. Dict literals may not have duplicate keys.
- Strings are not iterable. Use `==` instead of `is`. Comparison operators (`<`, `>`) are not defined across types.
- Each rule, `.bzl` file, and `BUILD` file gets its own execution context; mutation inside a context is fine, but
  mutations from a context to another's values are rejected.

## .bzl (Starlark) style

- Four-space indentation (PEP 8).
- Module-level docstring in every `.bzl` file; docstring for every public function.
- Use the `doc` argument on `rule()`, `aspect()`, and all `attr.*()` calls.
- Private values (not exported) start with one underscore: `_my_helper`. Bazel enforces that private symbols cannot be
  used outside their file.
- Rule implementation functions are always private: `_myrule_impl` for `myrule`.
- `print()` is for debugging only. Never leave `print()` calls in committed code unless guarded by `if DEBUG:` with
  `DEBUG = False`.
- **Prefer rules over macros.** Macros expand before analysis, making `bazel query` output hard to interpret and
  aspects unaware of them. Use macros only for targets intended to be referenced directly at the CLI.
- Internal macro targets (not meant to be used directly) must have:
  - names prefixed with `<name>_` or `<name>.` or `<name>-`,
  - `visibility = ["//visibility:private"]`,
  - `tags = ["manual"]` (excluded from `:all` and `//...` wildcards).

### .bzl file organisation

- Limit exported symbols per `.bzl` file — broad "utility" files cause wide rebuilds when anything changes.
- Each file should export multiple symbols only when they are always used together. Otherwise split into separate
  files, each wrapped with `bzl_library` from `bazel-skylib`.

## Labels and references

- `@@canonical_name//pkg:target` — canonical label (stable, preferred in generated code).
- `@apparent_name//pkg:target` — apparent label (resolved relative to the consuming module's `MODULE.bazel`).
- From inside an external repo or macro, `@@//pkg:target` refers to the *main* repo. Use this when you need an
  absolute reference to the root.
- `//my/pkg` is shorthand for `//my/pkg:pkg` — it is **not** a wildcard for all targets in that package (use
  `//my/pkg:all` or `//my/pkg:*`).
- `BUILD.bazel` takes precedence over `BUILD` when both exist; prefer `BUILD.bazel`.

## Visibility

- `//visibility:private` is the default for targets that are implementation details.
- Prefer `__subpackages__` over `__pkg__` for cross-package access within the same team's subtree.
- Avoid `default_visibility = ["//visibility:public"]` at the package level — it makes every target in the package
  globally accessible.
- Call `exports_files([...])` for any source file that external targets need to reference. Source files are
  package-private by default.
- In symbolic macros, visibility is scoped to the **`.bzl` file's package**, not the `BUILD` file that calls the
  macro.

## Rule implementation patterns

Key patterns every rule implementation should follow:

**Implicit dependencies (private attrs):** Use a `_`-prefixed attr with a default `Label` to hard-wire a tool dependency
the user cannot override. The tool is always available via `ctx.attr._compiler` without caller involvement. If the tool
needs to be overridable, use a public attr with a default.

**Declaring outputs:** Use `ctx.actions.declare_file(ctx.label.name + ".ext")` for generated files. Each generated file
must be the output of exactly one action. Use `ctx.actions.declare_directory` for tree artifacts.

**Actions:** Register with `ctx.actions.run`, `ctx.actions.run_shell`, `ctx.actions.write`, or
`ctx.actions.expand_template`. Actions must list **all** inputs and must produce **all** declared outputs. The set of
inputs/outputs must be determined at analysis time — it cannot depend on action results. Give every action a `mnemonic`
(e.g. `"MyCompile"`) for filtering with `aquery` and UI display.

**Providers:** Return a list of provider objects (not a legacy `struct`). Always provide `DefaultInfo` with the `files`
depset for outputs that should be built by default. Rules that perform actions but don't set `DefaultInfo` make
debugging harder — those actions are pruned when the target is built in isolation.

**Runfiles:** Merge runfiles from all dep attributes that may carry runtime files (`srcs`, `deps`, `data`):

```python
runfiles = ctx.runfiles(files = ctx.files.data)
for attr in (ctx.attr.srcs, ctx.attr.deps, ctx.attr.data):
    for t in attr:
        runfiles = runfiles.merge(t[DefaultInfo].default_runfiles)
return [DefaultInfo(runfiles = runfiles)]
```

**C++ interop:** To depend on or integrate with C++ rules, use `@rules_cc//cc:find_cc_toolchain.bzl`:
`use_cc_toolchain()` in `toolchains` + `find_cpp_toolchain(ctx)` to get `CcToolchainInfo`. Rules consuming C++ deps
receive `CcInfo` (contains `CompilationContext` and `LinkingContext`). If your rule propagates `CcInfo` through non-C++
rules (e.g. a Java rule with native deps), wrap it in a custom provider — do not expose raw `CcInfo` through rules where
the C++ semantic doesn't hold.

## Symbolic macros

- Declared with `macro()`. The framework auto-injects `name` and `visibility`; include them in the signature.
- All targets created by the macro must be named `name`, `name_*`, `name.*`, or `name-*`.
- Attrs inspected in the implementation body (strings, lists, bools) need `configurable = False`; otherwise they
  arrive as `select`-compatible wrappers and calls like `.join()` fail.
- `attr.label()` attrs need not be `configurable = False` if only passed through to child rules.
- No `glob()` inside symbolic macros — move glob calls to BUILD files.
- Use `finalizer = True` when the macro needs to inspect `native.existing_rules()`.
- Call `native.exports_files([output])` for any checked-in file the macro exposes, so the `_diff_test` (or other
  generated targets) can access it. Package-private files are invisible to macro-generated targets.
- Use `inherit_attrs = native.cc_library` (or any rule/macro symbol) to forward the wrapped symbol's attrs via
  `**kwargs`. Inherited non-mandatory attrs default to `None` — always guard with `(tags or []) + [...]`.
- Macro visibility is checked based on the **declaring** macro's package, not the calling BUILD file. Internal targets
  are invisible to callers unless the macro forwards its own `visibility` parameter to them.
- Targets declared without forwarding `visibility` are private to the macro; do not declare them with
  `["//visibility:public"]` — that overrides whatever the caller specified.

## Legacy macros

Prefer symbolic macros. Use legacy macros only when a parameter type isn't representable as a Starlark `attr`.

- Label strings in legacy macros resolve relative to the **BUILD file**, not the `.bzl` file. Wrap with `Label()`
  to make cross-repo references resolve correctly:
  ```python
  Label("@dep_of_my_ruleset//tools:foo")  # resolves within the ruleset, regardless of caller's repo
  ```
- Debug by inspecting the expanded form: `bazel query --output=build //my/path:all`
- Filter by origin: `bazel query 'attr(generator_function, my_macro, //my/path:all)'`

## Aspects

Aspects augment the build graph by propagating additional information and actions along dependency edges without
modifying the targets themselves. Typical uses: IDE integrations, cross-cutting linting, protobuf code generation.

```python
MyAspectInfo = provider(fields = ["count"])

def _my_aspect_impl(target, ctx):
    # target — the Target the aspect is applied to; provides access to its providers
    # ctx.rule.attr — the rule attributes of that target (after aspect propagation)
    count = 0
    if hasattr(ctx.rule.attr, "srcs"):
        for src in ctx.rule.attr.srcs:
            count += len(src.files.to_list())
    for dep in ctx.rule.attr.deps:
        count += dep[MyAspectInfo].count
    return [MyAspectInfo(count = count)]

my_aspect = aspect(
    implementation = _my_aspect_impl,
    attr_aspects = ["deps"],          # propagate along "deps"; use ["*"] for all attrs
    required_providers = [SomeInfo],  # only apply to targets providing SomeInfo
    attrs = {
        "_tool": attr.label(          # private label attrs for tools
            default = "//tools:my_tool",
            executable = True,
            cfg = "exec",
        ),
    },
)
```

- Aspect implementations take **two** arguments: `target` and `ctx` (unlike rules which take only `ctx`).
- Aspects may **never** return `DefaultInfo`. Returning a provider already returned by the underlying rule is an
  error (except `OutputGroupInfo`, which is merged, and `InstrumentedFilesInfo`, taken from the aspect).
- Public aspect attrs (`bool`, `int`, `string`) serve as parameters. For rule-propagated aspects, values come from
  the attribute of the same name on the calling rule; `int`/`string` params require `values = [...]`.
- When propagating along `attr_aspects`, `ctx.rule.attr.deps` holds the *aspect applications* of those deps
  (i.e., `[A(Y), A(Z)]`), not the raw targets — access the aspect's providers directly.
- Aspects can declare `toolchains = [...]` just like rules; same AEG rules apply.
- Invoke from the CLI: `bazel build //my:target --aspects=path/to/file.bzl%aspect_name`
- Attach to a rule's attribute: `attr.label_list(aspects = [my_aspect])` — aspect parameters are read from
  the rule's attribute of the same name.

## Build settings and transitions

### Custom build settings (flags)

User-defined build settings replace `--define`. Declare with `build_setting` on `rule()`:

```python
# bazel/flags/flags.bzl
string_flag = rule(
    implementation = lambda ctx: [FlagInfo(value = ctx.build_setting_value)],
    build_setting = config.string(flag = True),  # flag=True → user-settable from CLI
)
```

```python
# bazel/flags/BUILD.bazel
string_flag(name = "mode", build_setting_default = "opt")
```

Use on the command line with the full target path:

```sh
bazel build //... --//bazel/flags:mode=dbg
```

For `select()`, reference via `flag_values`:

```python
config_setting(name = "dbg", flag_values = {"//bazel/flags:mode": "dbg"})
```

Predefined flag rules (string, bool, int enums) are in `@bazel_skylib//rules:common_settings.bzl`.

### Transitions

A transition maps one build configuration to one or more output configurations. Use them to build deps in a different
configuration than their parent (e.g., compile a dep for a specific CPU).

```python
# 1:1 outgoing edge transition
def _arm_impl(settings, attr):
    return {"//command_line_option:cpu": "arm"}

to_arm = transition(implementation = _arm_impl, inputs = [], outputs = ["//command_line_option:cpu"])

my_rule = rule(
    implementation = _impl,
    attrs = {"dep": attr.label(cfg = to_arm)},  # outgoing edge
)
```

- **Incoming** edge transition: attached to `rule(cfg = ...)`. Must be 1:1.
- **Outgoing** edge transition: attached to `attr.label(cfg = ...)`. Can be 1:1 or 1:N.
- The `outputs` list must be returned in full even for no-ops; return `{}` / `[]` / `None` as shorthand for
  "keep all outputs unchanged".
- Transitions cannot be attached to native rules (only to Starlark rules).
- Do not transition on `--define` (unsupported) or `--config` (it is an expansion flag).
- **Performance:** every new configuration multiplies the build graph. A 1:2 transition at depth N creates
  2ⁿ configured instances of its transitive deps. Prefer single-platform builds; add transitions only when
  cross-compilation is a core requirement.

## Toolchains

The toolchain framework decouples rule logic from platform-specific tool selection.

- Rules declare a dependency on a `toolchain_type` (not a concrete tool):
  ```python
  my_rule = rule(
      toolchains = ["//tools:toolchain_type"],
      ...
  )
  ```
- The impl accesses the resolved toolchain via `ctx.toolchains["//tools:toolchain_type"]`, which returns a
  `ToolchainInfo` provider.
- Register toolchains in `MODULE.bazel`: `register_toolchains("//tools:my_toolchain")`
- In toolchain rule attrs:
  - `cfg = "exec"` — artifacts that run *during* the build (compilers, code generators). Built for the execution
    platform.
  - `cfg = "target"` — artifacts that end up *in* the final output (runtime libraries). Built for the target platform.
- **`select()` on a `cfg = "exec"` attr resolves under the *target* configuration**, not the exec configuration.
  Workaround: wrap the `select()` in an `alias()` target; the alias evaluates under exec.
- The `_toolchain` rule (by convention) **must not create build actions**; it only collects artifacts and returns them
  via `platform_common.ToolchainInfo(field = ...)`. Actions are created by the consuming rule.
- Optional toolchains: use `config_common.toolchain_type("//tools:type", mandatory = False)` in `toolchains = [...]`.
  If resolution fails, `ctx.toolchains["//tools:type"]` returns `None` instead of erroring.
- Toolchain priority order (earlier = higher priority): `--extra_toolchains` → root module's
  `register_toolchains` → non-root modules. Within a `register_toolchains` call, first listed wins.
- Debug toolchain resolution: `bazel build //... --toolchain_resolution_debug=.*`
- `cquery 'deps(//my:target, 1)' --transitions=lite | grep toolchain` shows which deps came from toolchain resolution.

## Execution groups

Execution groups allow a single rule to run different actions on different execution platforms (e.g., compile on a
remote Linux worker, link/sign on a local macOS machine). Each group has its own toolchain resolution.

```python
my_rule = rule(
    _impl,
    exec_groups = {
        "link": exec_group(
            exec_compatible_with = ["@platforms//os:linux"],
            toolchains = ["//foo:toolchain_type"],
        ),
    },
    attrs = {
        "_linker": attr.label(cfg = config.exec("link")),  # built for the "link" exec group
    },
)
```

In the implementation, assign actions to a group and access its toolchain:

```python
def _impl(ctx):
    foo_info = ctx.exec_groups["link"].toolchains["//foo:toolchain_type"].fooinfo
    ctx.actions.run(
        inputs = [foo_info],
        exec_group = "link",   # action runs on the "link" execution platform
        ...
    )
```

Built-in exec groups: `test` (test runner actions) and `cpp_link` (C++ linking).

Use `exec_properties` with `<group>.<key>` syntax to allocate extra resources per action group without affecting the
rest of the target:

```python
my_rule(name = "foo", exec_properties = {"mem": "4g", "link.mem": "32g"})
```

### Automatic execution groups (AEGs)

From Bazel 7 onward, Bazel automatically creates an exec group per toolchain type registered on a rule — you no longer
need explicit `exec_groups` for the common case of one toolchain per action.

When calling `ctx.actions.run` or `ctx.actions.run_shell` with an executable from a toolchain, you **must** pass
`toolchain =` so Bazel knows which exec group to use:

```python
ctx.actions.run(
    executable = ctx.toolchains["//tools:toolchain_type"].tool,
    toolchain = "//tools:toolchain_type",  # required for AEGs
    ...
)
```

If the action does not use any toolchain tool, pass `toolchain = None`.

Only define manual `exec_groups` when a single action needs tools from **two or more** toolchains on the **same**
execution platform — that case cannot be expressed with AEGs alone.

## Platforms and `select()`

Three distinct platform roles: **host** (where Bazel runs), **execution** (where build actions run), **target** (what
the output runs on). Defaults to `@platforms//host` for all three unless `--platforms` is specified.

Platform-conditional attributes:

```python
config_setting(name = "on_linux",
    constraint_values = ["@platforms//os:linux"])
my_rule(deps = select({":on_linux": [":linux_dep"],
                        "//conditions:default": [":generic_dep"]}))
```

- `//conditions:default` is the mandatory fallback.
- Matches must be unambiguous: if two `config_setting`s both match, one must be a strict superset of the other, or
  both must resolve to the same value.
- `--enable_platform_specific_config` (set in `.bazelrc`) auto-activates `build:linux`, `build:macos`,
  `build:windows` configs based on host OS.

### Incompatible targets

Use `target_compatible_with` to declare that a target only makes sense on certain platforms. Incompatible targets are
silently skipped in wildcard builds (`//...`, `:all`) but cause an error if named explicitly.

```python
cc_library(
    name = "win_driver_lib",
    srcs = ["win_driver_lib.cc"],
    target_compatible_with = ["@platforms//os:windows", "@platforms//cpu:x86_64"],
)
```

For OR logic (compatible with macOS or Linux but nothing else):

```python
target_compatible_with = select({
    "@platforms//os:osx": [],
    "@platforms//os:linux": [],
    "//conditions:default": ["@platforms//:incompatible"],
})
```

## Depsets and rule performance

Accumulating deps with plain lists is O(n²). Use depsets.

Depset ordering is determined at construction and affects `to_list()` traversal. Choose `order` deliberately:

- `postorder` — leaves before roots (typical for classpath-style flags where a library must precede its consumers).
- `preorder` — roots before leaves.
- `topological` — all parents before their children; useful for linkers that require this ordering.
- `default` — no ordering guarantee (cheapest; use when order truly doesn't matter).

Depsets compare by **identity**, not contents — two separately constructed depsets with the same elements are not equal.
Do not use depsets as dict keys or compare them with `==`.

```python
# Bad — O(n²) memory
all_files = []
for dep in ctx.attr.deps:
    all_files += dep[MyProvider].files.to_list()

# Good — O(n)
all_files = depset(transitive =
    [dep[MyProvider].files for dep in ctx.attr.deps])
```

- Never call `depset.to_list()` in non-terminal rules (not even at `*_binary` level, as building `//...` makes it
  O(n²) again).
- Never call `depset()` inside a loop — collect the transitive list first, then create one depset at the end.
- Use `ctx.actions.args()` for command-line building; it defers depset expansion to execution time, avoiding memory
  spikes.
- Pass a depset directly to `ctx.actions.run(inputs = …)` — do not flatten it to a list.

## Runfiles

**Never hardcode runfile paths.** Canonical repo name format is unstable across Bazel versions and workspace
configurations.

Use the language-specific runfiles library instead:

```go
// Go: github.com/bazelbuild/rules_go/go/runfiles
r, _ := runfiles.New()
path, _ := r.Rlocation(filepath.Join(runfiles.CallerRepository(), "path/to/file"))
```

In `genrule` or test `args`, use the `$(rlocationpath :target)` Make variable, not `$(location)`.

## Shell portability — use Bash as a last resort

`genrule`, `sh_binary`, `sh_test`, and `ctx.actions.run_shell()` all require Bash. Since Bazel 1.0, every other rule
type is Bash-free. Bash is the single biggest portability hazard in Bazel builds:

- **Windows**: Bash is not installed by default. Building those rules requires MSYS2, configured via `BAZEL_SH` /
  `--shell_executable` in `.bazelrc`. This dependency is increasingly absent on developer machines.
- **macOS**: the system shell (`/bin/bash`) is version 3.2 (2007, last GPLv2 release — Apple does not ship later
  versions for licensing reasons). It lacks `declare -A` (associative arrays), `mapfile`/`readarray`, and many
  features added in Bash 4/5. Users may have Homebrew Bash 5.x, but that is not guaranteed and must not be assumed.
- **Linux CI containers**: minimal images often omit Bash or ship Dash as `/bin/sh`.

Preferred order for anything that would otherwise use a shell:

1. **`run_binary()` / `native_binary()`** from `bazel-lib` (or `bazel-skylib` as a fallback) — Bash-free on all
   platforms, pre-built binaries for platform-specific helpers. Use them in BUILD files and macros before reaching
   for `genrule`.
   ```python
   load("@bazel_lib//lib:run_binary.bzl", "run_binary")
   run_binary(
       name = "gen_foo",
       tool = "//tools:my_tool",
       srcs = [":input"],
       outs = ["foo.out"],
       args = ["$(location :input)", "$(location foo.out)"],
   )
   ```

2. **`ctx.actions.run`** — explicit executable, no shell involvement at all. The right default for Starlark rules.

3. **`py_binary` + `rules_python`** — when scripting logic is complex enough to warrant a real language but a
   compiled binary is overkill. Python is available on all platforms via `rules_python` which manages the
   interpreter hermetically. More heavyweight than options 1–2 but far more portable than shell scripts.

4. **`genrule` / `ctx.actions.run_shell` / `sh_binary`** — last resort, only when no alternative exists. If you
   must use them: target only POSIX sh (`#!/bin/sh`), never Bash-specific syntax, and document the requirement.
   In Starlark rules, `ctx.actions.run_shell` is preferable to `genrule` because Bazel controls the shell
   invocation; in repository rules, there is no principled way to invoke Bash at all — avoid it there.

## bazel-lib and bazel-skylib

**`bazel-lib`** (`@bazel_lib`, formerly `@aspect_bazel_lib` before 3.0) is the preferred Bash-free toolkit. It is a
superset of `bazel-skylib` in most areas, evolves much faster through community contributions, and ships dedicated
Windows fixes (tools are pre-built `.exe` binaries rather than shell scripts). `bazel-skylib`'s declared scope is frozen
and no longer accepts feature requests. This repo uses `bazel_lib` 3.x — loads are `@bazel_lib//lib:...`.

| Task | `bazel-lib` | `bazel-skylib` fallback |
|------|-------------|------------------------|
| Run a binary as a build action | `run_binary()` — adds directory outputs, richer makevar expansion | `run_binary()` |
| Copy a file | `copy_file()` | `copy_file()` |
| Copy a directory | `copy_directory()` | — |
| Copy files/dirs to an output dir | `copy_to_directory()` | — |
| Copy source files to `bazel-bin` | `copy_to_bin()` | — |
| Expand a template (hermetic toolchain) | `expand_template()` | `expand_template()` (shell-based) |
| Write text with correct line endings | — | `write_file()` |
| Diff two files or directories | `diff_test()` | `diff_test()` (files only) |
| Write generated files back to source | `write_source_files()` | — |
| Wrap a pre-built native binary | — | `native_binary()` / `native_test()` |
| Platform detection utilities | `platform_utils()` | `selects.bzl` |
| Stamping (version embedding) | `stamping.bzl` | — |
| Params files for long command lines | `params_file()` | — |

Note: `tar`, `jq`, and `yq` were split into separate modules in 3.0 — they are no longer part of `bazel_lib`.

## Hermeticity

A build is hermetic when it depends only on declared inputs. Violations cause incorrect incremental builds and cache
poisoning.

Common sources of non-hermeticity to avoid:
- Embedding timestamps, build IDs, or host-specific strings in outputs.
- Invoking tools from `PATH` or system directories. Use toolchain rules or `ctx.executable` from a `tools` attribute.
- Writing to the source tree during build actions (only outputs declared in `ctx.declare_file()` are allowed).

To verify hermeticity: run `bazel build //...` twice in a row with no source changes. The second build must perform zero
actions.

For remote execution (RBE), the same constraints apply with no exceptions:
- Never reference tools via `PATH`, `JAVA_HOME`, or env vars — remote executors run each action in isolation.
- Test locally with `--strategy=linux-sandbox` to simulate RBE isolation before enabling remote execution.

## Sandboxing

The `.bazelrc` enables `--strategy=sandboxed` on Linux and macOS. Windows uses `--strategy=standalone` (no sandbox
available).

- `linux-sandbox` uses Linux namespaces (similar to Docker). It does not work inside an unprivileged container; Bazel
  automatically falls back to `processwrapper-sandbox`.
- `processwrapper-sandbox` works on any POSIX system.
- Use `--reuse_sandbox_directories` to reduce sandbox setup overhead (also helps on Windows and macOS).

Debugging failed sandboxed builds:

```sh
bazel build //target --verbose_failures --sandbox_debug
```

This keeps the sandbox directory on disk for inspection. Disable `--sandbox_debug` immediately after debugging — it
fills disk quickly.

## Persistent workers

Persistent workers are long-running processes that handle multiple compilation requests, avoiding per-action JVM/tool
startup cost. Enabled by default for Java and Kotlin; also available for Scala and others.

- Speed-up is 2–4× for Java, ~2.5× for Bazel itself.
- Workers run with sandboxing when dynamic execution is active.
- Tune concurrency with `--worker_max_instances=<N>` (default 4 per mnemonic).
- To explicitly use workers: `--strategy=Javac=worker,local` (with `local` as fallback).

## Dynamic execution

Dynamic execution races local and remote execution for the same action, using whichever finishes first. Requires both
local and remote execution to be configured.

- Enable with `--internal_spawn_scheduler` and then `--strategy=<mnemonic>=dynamic`.
- `--dynamic_local_execution_delay=<ms>` (default 1000 ms) delays local start after a remote cache hit to avoid
  redundant local work — tune to slightly above typical cache-hit round-trip time.
- Persistent workers automatically sandbox when used with dynamic execution.

## Windows

All Windows developers are expected to have **Developer Mode enabled**, which grants the necessary privileges for
symlink creation without administrator elevation. The `.bazelrc` sets `--enable_runfiles` accordingly.

**No sandbox.** Windows uses `--strategy=standalone`. Builds are less hermetic by default — undeclared dependencies that
happen to be present locally will succeed locally and fail in CI or RBE.

**Path separators.** Bazel stores paths with `/` internally. When constructing command lines or environment variables
for actions, replace `/` with `\` for Windows tools that don't accept forward slashes:

```python
def as_path(p, is_windows):
    return p.replace("/", "\\") if is_windows else p
```

**Absolute paths.** On Windows they start with a drive letter (`C:\...`), not `/`. Rules that detect absolute paths by
looking for a leading `/` will fail silently on Windows.

**Environment variables.** Windows env var names are case-insensitive. Use UPPERCASE names throughout for portability.
Minimize action environments — env vars are part of the cache key.

**Executable extensions.** Every executable output must have an extension (`.exe` or `.bat`). Shell scripts (`.sh`) are
not executable on Windows and cannot be used as `ctx.actions.run`'s `executable`. Empty `.bat` files cannot be executed
— write at least one space if you need a no-op script.

**Bash.** The `.bazelrc` configures MSYS2 bash (`BAZEL_SH`, `--shell_executable`) for the rules that require it. Avoid
those rules — see the [Shell portability](#shell-portability--use-bash-as-a-last-resort) section above.

**Run from `cmd.exe` or PowerShell**, not MSYS2/Git Bash. MSYS2 auto-converts path arguments like `//foo:bar` to Windows
paths, breaking Bazel label syntax.

**File deletion.** Open files cannot be deleted on Windows ("Access Denied"). Close handles eagerly. A running process
also holds its working directory open, preventing deletion.

**Path length.** The hard limit is 32,767 characters (Developer Mode removes the legacy 260-character limit). Keep
workspace names, target names, and directory structures short to stay well within this.

## Testing

### Testing rules (Starlark unit tests)

Use `bazel-skylib`'s `unittest.bzl` to test rule analysis behavior (providers, actions) without running a full build:

```python
load("@bazel_skylib//lib:unittest.bzl", "asserts", "analysistest")

def _my_rule_test_impl(ctx):
    env = analysistest.begin(ctx)
    target_under_test = analysistest.target_under_test(env)
    asserts.equals(env, "expected", target_under_test[MyInfo].val)
    return analysistest.end(env)
```

- Assertions are stored in a generated script and fail at test execution time (not analysis time).
- Analysis tests are limited to targets with at most ~500 transitive dependencies.
- Test failures from `fail()` show as build errors, not test failures — use `asserts.*` instead.

### Test runner contract

- A test passes if and only if its process exits with code 0. Writing `PASS` or `FAIL` to stdout has no effect.
- Tests must be hermetic: they may only access declared runfiles and resources guaranteed by the runner.
- The `$TEST_TMPDIR` environment variable points to a writable scratch directory (unique per test run).
- The `$TEST_SRCDIR` variable points to the runfiles tree root; access test data via the runfiles library, not
  hardcoded paths.
- `size` (`small`/`medium`/`large`/`enormous`) controls timeouts; `--test_timeout` overrides per invocation.

## Querying the build graph

Three query tools, each at a different build phase:

| Tool | Phase | Understands `select()` | Sees actions |
|------|-------|------------------------|--------------|
| `bazel query` | loading | no | no |
| `bazel cquery` | analysis | yes | no |
| `bazel aquery` | analysis | yes | yes |

Common patterns:

```sh
# All targets in a package
bazel query //my/pkg/...

# Dependencies of a target (without toolchain noise)
bazel query --noimplicit_deps 'deps(//my:target)'

# Reverse dependencies: what depends on //lib:foo?
bazel query "rdeps(//..., //lib:foo)"

# Why does A depend on B?
bazel query "somepath(//A, //B)"   # one path
bazel query "allpaths(//A, //B)"   # all paths

# Targets with a specific tag
bazel query 'attr(tags, "my_tag", //...)'

# Visualise as a graph
bazel query --noimplicit_deps 'deps(//my:target)' --output graph | dot -Tpng > graph.png

# Note: tags = ["manual"] excludes targets from //... in build/test,
# but bazel query does NOT filter them.

# cquery: resolve select() under a specific configuration
bazel cquery "deps(//my:target)" --define species=excelsior --noimplicit_deps

# cquery: inspect which deps came from toolchain resolution
bazel cquery 'deps(//my:target, 1)' --transitions=lite | grep toolchain

# cquery: filter out incompatible targets
bazel cquery //... --output=starlark --starlark:expr='target.label if "IncompatiblePlatformProvider" not in providers(target) else ""'

# aquery: inspect actions for a target
bazel aquery '//src:target_a'

# aquery: filter actions by input filename pattern
bazel aquery 'inputs(".*\\.go", deps(//my:target))'

# aquery: filter by mnemonic (e.g., all GoCompile actions)
bazel aquery 'mnemonic("GoCompile", deps(//my:target))'
```

`cquery` understands configuration; use it when `select()` matters. `aquery` shows the action graph (inputs, outputs,
command lines); use it to debug what commands Bazel will run.

## Performance profiling

```sh
# JSON trace — open in chrome://tracing or perfetto.dev
bazel build //... --profile=/tmp/bazel.profile

# Starlark CPU profile (pprof format)
bazel build //... --starlark_cpu_profile=/tmp/starlark.prof
pprof -text -lines /tmp/starlark.prof

# Memory: dump heap to pprof
bazel dump --skylark_memory=$HOME/prof.gz
pprof -flame $HOME/prof.gz
```

Use `query` / `cquery` to investigate build size regressions before profiling execution:
- Many new packages loaded → dependency graph growth (check `deps()` for new transitive deps).
- Many new targets configured → diamond dependencies or platform proliferation.
- Many new actions created → check `aquery --output=summary`.

## See also

- [Rust in the Datadog Agent](../docs/public/guidelines/languages/RUST.md)
- [eBPF Core Checks](../pkg/collector/corechecks/ebpf/AGENTS.md)
