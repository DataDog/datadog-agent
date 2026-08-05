Apply these when the PR touches files under `bazel/`, any `BUILD.bazel`, `MODULE.bazel`, or `.bzl` file anywhere in the repo.

## File naming

Never name a file `BUILD` — always `BUILD.bazel`. The repo enforces `BUILD.bazel` exclusively via Gazelle
(`-build_file_name=BUILD.bazel`). Using `BUILD` causes silent failures on macOS/Linux container bind-mounts due to
case-insensitive filesystem collisions.

## Formatting

`buildifier` is mandatory before committing. Flag any PR that modifies `BUILD.bazel` or `.bzl` files with no evidence
of having run `bazel run //bazel/buildifier`. Missing formatting indicates the file was edited without the required
toolchain step.

## Dependencies

Declare only **direct** deps. Relying on transitive deps is a layering violation that breaks strict dep checking. Flag
any `deps` list that appears to include packages not directly imported by the target's source files.

No recursive globs: `glob(["**/*.ext"])` skips subdirectories containing BUILD files, defeats remote caching, and
breaks incremental builds. Flag any `glob` with a `**` pattern. Instead, add a `BUILD.bazel` to each subdirectory.

## `print()` statements

`print()` in `.bzl` files is for debugging only. Flag any committed `.bzl` file that contains a `print()` call not
guarded by `if DEBUG:` with `DEBUG = False`.

## Depsets over list accumulation

Accumulating deps with `+=` or list concatenation in a loop is O(n²) memory. Flag any rule implementation that
accumulates deps/files with:

```python
# Bad
all_files = []
for dep in ctx.attr.deps:
    all_files += dep[MyProvider].files.to_list()
```

The correct pattern is:

```python
all_files = depset(transitive = [dep[MyProvider].files for dep in ctx.attr.deps])
```

Also flag `depset.to_list()` called in non-terminal rules (only `*_binary` / `*_test` level is acceptable).

## Runfile paths

Never hardcode canonical repo name paths (e.g., `external/<repo>/...` or `<module>+<ext>+<name>`). These formats are
unstable across Bazel versions. Flag any hardcoded runfile path string; the correct approach is the language-specific
runfiles library or `$(rlocationpath :target)` in `genrule`/test `args`.

## MODULE.bazel lock file

`MODULE.bazel.lock` must be committed. Flag any PR that modifies any module extension (implementation or invocation)
without a corresponding update to `MODULE.bazel.lock`.

## No WORKSPACE patterns

This repo uses Bzlmod exclusively. Flag any new `WORKSPACE` or `WORKSPACE.bazel` file, or any `workspace()` call.
Do **not** flag `load("@repo//...")` labels — these are normal Bzlmod usage and the repo relies on them extensively
(e.g. `@gazelle//:def.bzl`, `@linux_headers//:defs.bzl`). Only flag a `load` if the repo it references is declared
solely in a `WORKSPACE` file and is absent from `MODULE.bazel`.

## `use_repo(...)` accuracy

After a module extension changes, `use_repo(...)` lists must be updated (run `bazel mod tidy`). Flag PRs that add or
remove extension tags without a corresponding `use_repo(...)` update.

## Labels must not be split across lines

Labels must be string literals and must never be split across lines. Automated tools (buildozer, Code Search) cannot
handle split or computed label values. Flag any `deps`, `srcs`, or other label lists that construct label strings via
`+`, `%`, or line continuation.

## Shell portability

`genrule`, `sh_binary`, `sh_test`, and `ctx.actions.run_shell` require Bash and are non-portable:

- Windows: Bash is not installed by default.
- macOS: system Bash is 3.2 (lacks `mapfile`, `declare -A`, etc.).
- CI containers: may ship Dash as `/bin/sh`.

Flag new `genrule`, `sh_binary`, or `sh_test` targets without a documented reason. The preferred alternatives are
`run_binary()` from `@bazel_lib` (Bash-free) or `ctx.actions.run` in Starlark rules.

## Exit code 3 trap in scripts

Bazel exit code 3 means "build succeeded but tests failed or timed out." Scripts that check only for exit code 1
(`[ $? -eq 1 ]`) will silently miss test failures. Flag any wrapper script that calls `bazel test` and does not
handle exit code 3 explicitly (e.g., `[ $? -ne 0 ]`).

## `bazel clean` in scripts

`bazel clean` discards incremental build state and is never correct in an automated script. Flag any script or CI step
that invokes `bazel clean`.

## Windows path portability

Rules that detect absolute paths by checking for a leading `/` will silently fail on Windows (paths start with a drive
letter like `C:\`). Flag such patterns in Starlark rule implementations. Similarly, executable outputs on Windows must
have an extension (`.exe` or `.bat`) — flag `ctx.actions.run` calls that use a shell script as `executable`.

## Internal macro targets

Internal targets created by macros (not meant to be referenced directly) must have `tags = ["manual"]` to exclude
them from `:all` and `//...` wildcards. Flag macro-generated targets without `tags = ["manual"]` if they are
implementation details not intended for direct use.
