# Safe-Shell Integration Test Coverage Gaps

Analysis of shell features implemented or blocked in `pkg/shell/interp/` that are
**not covered** by integration test scenarios in `pkg/shell/tests/scenarios/`.

## Supported features missing tests

### 1. Background processes (`&`)

The runner handles `st.Background` (`runner.go:156-173`) by spawning a goroutine
subshell, but no scenario exercises `cmd &` syntax. This also means there is no
test for `wait` semantics or the background PID list.

**Suggested scenario:** `echo hello &` followed by assertions on stdout/exit code.

### 2. Shell options (`set -e`, `-a`, `-n`, `-f`, `-u`, pipefail)

All six shell options in `shellOptsTable` (`api.go:484-493`) are wired into the
runner but have zero integration tests. Since `set` is not a builtin in
safe-shell, options can only be configured via the `Params()` runner option at
construction. The test harness YAML schema (`input.envs`, `input.script`,
`input.allowed_paths`) has **no field to set runner params/options**, so testing
these currently requires either:

- Extending the YAML `input` schema with an `options` field, or
- Writing Go-level unit tests outside the scenario framework.

| Option | Runner constant | Effect | Location |
|--------|----------------|--------|----------|
| `errexit` (`-e`) | `optErrExit` | Exit on first failure | `runner.go:198-201` |
| `allexport` (`-a`) | `optAllExport` | Auto-export all vars | `vars.go:141-143` |
| `noexec` (`-n`) | `optNoExec` | Parse only, skip exec | `runner.go:146` |
| `noglob` (`-f`) | `optNoGlob` | Disable glob expansion | `runner.go:34-36` |
| `nounset` (`-u`) | `optNoUnset` | Error on unset vars | `runner.go:44` |
| `pipefail` | `optPipeFail` | Propagate pipe failures | `runner.go:293-295` |

### 3. Bash options (`globstar`, `nocaseglob`, `nullglob`)

Three bash-style options in `bashOptsTable` (`api.go:495-512`) have no tests.
Same harness limitation as above.

### 4. Glob expansion (general)

Glob patterns `*`, `?`, `[...]` are supported via the `ReadDir2` handler
(`runner.go:37-39`). The only glob test is `shell/allowed_paths/glob_inside_allowed.yaml`
which tests glob **within an allowed-path sandbox**. There is no scenario for
basic glob expansion without `AllowedPaths` enabled (e.g. `echo *.txt` in a dir
with matching files).

### 5. Tilde expansion (`~`)

`HOME` is initialized in `api.go:627-629` and tilde expansion is handled by the
`expand` package, but no scenario tests `echo ~` or `echo ~/subdir`.

### 6. `cat` with multiple files (success case)

All multi-file `cat` tests are error scenarios (`cmd/cat/errors/multiple_*.yaml`).
There is no scenario for `cat file1 file2` where both files exist and succeed.

### 7. `for i; do` (bare for, iterating positional params)

The for-loop implementation (`runner.go:300-316`) supports `for i; do` which
iterates over `r.Params`. No scenario tests this form (all for-loop tests use
`for i in ...`). Same harness limitation — no way to set `Params`.

### 8. `<&-` (close stdin redirection)

`runner.go:434-441` handles `DplIn` with arg `"-"` to close stdin, but no
scenario tests `cat <&-` or similar.

### 9. Unsupported fd redirect (fd ≥ 3)

`runner.go:401` returns an error for redirect fds other than 0, 1, 2. No scenario
tests that `3>&1` or similar produces the expected error.

### 10. Context cancellation / timeout

`runner.go:137-149` checks `ctx.Err()` on every statement. No scenario tests
runner behavior when the context is cancelled mid-execution.

### 11. Empty or comment-only scripts

No scenario runs an empty script (`""`) or a script containing only comments
(`# just a comment`). These are valid inputs that should produce exit code 0.

### 12. Escaped characters in double quotes

No scenario tests backslash escaping inside double quotes (`echo "hello \"world\""`,
`echo "path\\to\\file"`).

### 13. Word splitting / IFS behavior

IFS is set to `" \t\n"` in `api.go:632`. No scenario tests how IFS affects field
splitting (e.g., a variable containing spaces being split into multiple args).

### 14. Inline variable assignment edge cases

Only one test exists (`shell/environment/inline_assignment.yaml`). Missing:
- Multiple inline vars: `A=1 B=2 echo $A $B`
- Var restoration after inline: `X=old; X=new echo $X; echo $X`

## Blocked features missing tests

### 15. Process substitution (`<()` / `>()`)

Blocked in `validate.go:29-31` but no scenario asserts the error message
"process substitution is not supported".

### 16. `typeset` keyword

`DeclClause` in `validate.go:65-66` blocks `declare`, `local`, `export`,
`readonly`, **and** `typeset`. Tests exist for the first four but not `typeset`.

### 17. `TestDecl` (Bash test declarations)

Blocked in `validate.go:76-78` ("test declarations are not supported") — this is
the Bash `test -v` style declaration, distinct from `[[ ]]`. No dedicated test.

## Summary table

| # | Gap | Category | Severity |
|---|-----|----------|----------|
| 1 | Background `&` | Supported, untested | High |
| 2 | Shell options (`-e`, `-a`, `-n`, `-f`, `-u`, pipefail) | Supported, untested (harness limitation) | High |
| 3 | Bash options (globstar, nocaseglob, nullglob) | Supported, untested (harness limitation) | Medium |
| 4 | Glob expansion (general) | Supported, minimal testing | Medium |
| 5 | Tilde expansion `~` | Supported, untested | Medium |
| 6 | `cat file1 file2` (success) | Supported, untested | Low |
| 7 | `for i; do` (bare for) | Supported, untested (harness limitation) | Low |
| 8 | `<&-` (close stdin) | Supported, untested | Low |
| 9 | Unsupported fd redirect | Error path, untested | Low |
| 10 | Context cancellation | Supported, untested | Medium |
| 11 | Empty/comment scripts | Supported, untested | Low |
| 12 | Escaped chars in double quotes | Supported, untested | Low |
| 13 | Word splitting / IFS | Supported, untested | Medium |
| 14 | Inline var assignment edge cases | Supported, minimal testing | Low |
| 15 | Process substitution (blocked) | Blocked, untested | Medium |
| 16 | `typeset` (blocked) | Blocked, untested | Low |
| 17 | `TestDecl` (blocked) | Blocked, untested | Low |
