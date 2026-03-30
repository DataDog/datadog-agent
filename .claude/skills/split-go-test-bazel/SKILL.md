---
name: split-go-test-bazel
description: Split a monolithic go_test target in a BUILD.bazel into per-test-file targets with correct srcs, data, deps, and gotags.
allowed-tools: Read, Write, Edit, Glob, Grep, Bash
---

Split a single `go_test` target that bundles many test files into distinct per-test-file
targets. This improves build parallelism and makes test failures easier to attribute.

## Step 1 — Audit the source files

Read the existing `go_test` target to find all `srcs`. For each `_test.go` file, determine:

1. **What test data paths does it open?** Grep for string literals matching `"test/`:
   ```bash
   grep -n '"test/' pkg/some/package/*_test.go
   ```
   Files that open no paths need no `data` entries.

2. **What symbols does it define that other test files call?**
   Grep for function/type definitions, then check which other test files call them.
   Common patterns: shared `testProvider`, mock types, helper constructors.

3. **Does it use any `//go:build test`-gated symbols?**
   Check dependencies: if the test calls functions only compiled under `//go:build test`
   (look for that tag in the dep packages), the target needs `gotags = ["test"]`.

## Step 2 — Map dependencies between test files

Go test files in the same package freely call each other's symbols, but Bazel `go_test`
targets require every file providing a needed symbol to be listed in `srcs`. Build a
dependency graph:

- If `foo_test.go` calls `helperFunc` defined in `bar_test.go`, both must be in the same
  `go_test` target's `srcs`.
- Follow the chain: if `bar_test.go` calls `baz` defined in `baz_test.go`, all three
  must appear together.

**Common pattern**: a shared helper file (e.g. `metrics_translator_test.go`) defines
constructors/mocks used by many other test files. Any target that includes one of those
other files must also include the helper — and any file the helper itself depends on.

## Step 3 — Handle shared helper data with a named constant

If a helper test file (e.g. `testhelper_test.go`) has its own data needs and appears in
many targets, capture its data files in a Starlark list constant near the top of
`BUILD.bazel`:

```python
# TESTHELPER_DATA lists data files required by testhelper_test.go's own tests.
# Every go_test target that includes testhelper_test.go must also include these files.
TESTHELPER_DATA = [
    "test/otlp/hist/simple-delta.json",
    "test/datadog/hist/simple-delta_nobuckets-cs.json",
]
```

Then use `+ TESTHELPER_DATA` in each target's `data` attribute rather than duplicating
the list.

## Step 4 — Write the targets

For each logical test file (or small cluster of tightly-coupled files):

```python
go_test(
    name = "foo_test",
    srcs = [
        "foo_test.go",
        "testhelper_test.go",          # included because foo_test.go calls its helpers
        "shared_helpers_test.go",      # included because testhelper_test.go calls its symbols
    ],
    data = glob([
        "test/otlp/foo/**",
        "test/datadog/foo/**",
    ]) + TESTHELPER_DATA,
    embed = [":the_library"],
    gotags = ["test"],                 # only if //go:build test symbols are needed
    deps = [...],
)
```

**`data` discipline**: only include glob patterns for directories that a file in `srcs`
actually opens. Don't copy the full data list from the monolithic target.

**`glob()` consolidation**: merge sibling globs into one call:
```python
# Instead of:
data = glob(["test/otlp/foo/**"]) + glob(["test/datadog/foo/**"]),
# Write:
data = glob(["test/otlp/foo/**", "test/datadog/foo/**"]),
```

## Step 5 — Verify

```bash
bazel test //pkg/your/package/...
```

Interpret results:

| Result | Meaning | Action |
|--------|---------|--------|
| `PASSED` | ✅ | — |
| `FAILED TO BUILD` | Missing `srcs`, `deps`, or `gotags` | Fix BUILD.bazel |
| `FAILED` (test ran) | Missing `data` file, or real test bug | Check test.log |

**Build errors → srcs/deps problem.** Read the error: `undefined: someSymbol` means
`someSymbol`'s defining file is missing from `srcs`. Add it (and follow its own deps).

**Test failures → data problem.** `open test/foo/bar.json: no such file or directory`
means that path is missing from the target's `data`. Add the appropriate glob.

**`gotags` hint**: if you see `undefined: SomeFunction` and that function lives in a file
with `//go:build test`, add `gotags = ["test"]` to the target.

## Step 6 — Guard against Gazelle regeneration

The hand-crafted targets must survive future Gazelle runs. Two annotations are required:

**`# gazelle:exclude <filename>`** — one per test file, collected in a block near the top
of the `BUILD.bazel` (before the first `go_test`). Tells Gazelle not to auto-generate a
`go_test` for that file, which would conflict with the hand-crafted target.

**`# keep`** — placed on the line immediately before each hand-crafted `go_test`. Tells
Gazelle not to delete targets it didn't generate.

```python
# List all tests that are managed by hand-crafted targets.
# gazelle:exclude histograms_test.go
# gazelle:exclude testhelper_test.go
# gazelle:exclude nan_metrics_test.go
# ... one line per _test.go file covered by hand-crafted targets

# keep
go_test(
    name = "histograms_test",
    ...
)

# keep
go_test(
    name = "unit_tests",
    ...
)
```

Every `_test.go` file whose package is covered by a hand-crafted target needs a
`gazelle:exclude` line, and every hand-crafted `go_test` needs `# keep`. Missing either
annotation means a future `bazel run //:gazelle` will silently undo the split.

## Common pitfalls

- **Forgetting cascading srcs**: `a_test.go` needs `b_test.go` which needs `c_test.go` —
  all three must be in `srcs` together.
- **Copying mixed/other data globs from the monolithic target**: audit each file; most
  in-memory unit tests need no data at all.
- **The Bazel formatter reformats the file between reads and writes**: if you see
  "file modified since read" errors, re-read before editing, or write the whole file
  atomically via a shell heredoc / Python `open().write()`.
