---
slug: wildcard-file-ordering-stable
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-29
---

# wildcard-file-ordering-stable — Wildcard File Priority Ordering Is Stable Under filesLimit Cap

## What Led to This Property

`sut-analysis.md` §9 (unproven assumption #8) and §6 (bug history) identify a
known bug and skipped test: `applyReverseLexicographicalOrdering()` in
`file_provider.go:361-382` contains a FIXME acknowledging that the function
**assumes `filepath.Glob`/doublestar returns results in lexicographical order**.
This is an undocumented implementation detail of the Go stdlib; the Go issue
tracker (#17153) notes it is not guaranteed. When the assumption breaks, the
reverse-then-stable-sort produces a wrong ordering.

The practical consequence: when more wildcard-matching files exist than the
`filesLimit` cap allows, the `FilesToTail` function selects the top N files by
the computed ordering. If the ordering is wrong (because the input was not
pre-sorted), lower-priority files get tailed instead of higher-priority ones.
The test `"Multiple Directories - Out of order input"` in
`file_provider_test.go:644-660` is explicitly skipped with the comment
`// See FIXME in 'applyOrdering', this test currently fails`.

## Code Paths Involved

**`pkg/logs/launchers/file/provider/file_provider.go:361-382`** —
`applyReverseLexicographicalOrdering()`:

```go
func applyReverseLexicographicalOrdering(files []*tailer.File) {
    // FIXME - this codepath assumes that the 'paths' will arrive in lexicographical order
    // This is true in the current go implementation, but it is unsafe to assume
    // https://cs.opensource.google/go/go/+/refs/tags/go1.19:src/path/filepath/match.go;l=330
    // https://github.com/golang/go/issues/17153
    //
    // Files are sorted because of a heuristic on the filename: often the
    // filename and/or the folder name contains information in the file datetime.
    // Most of the time we want the most recent files.
    // Here, we reverse paths to have stable sort keep reverse lexicographical
    // order w.r.t dir names. Example:
    // [/tmp/1/2017.log, /tmp/1/2018.log, /tmp/2/2018.log] becomes
    // [/tmp/2/2018.log, /tmp/1/2018.log, /tmp/1/2017.log]
    for i := len(files)/2 - 1; i >= 0; i-- {
        opp := len(files) - 1 - i
        files[i], files[opp] = files[opp], files[i]
    }
    // sort paths by descending filenames
    sort.SliceStable(files, func(i, j int) bool {
        return filepath.Base(files[i].Path) > filepath.Base(files[j].Path)
    })
}
```

The algorithm is: (1) reverse the slice in-place, then (2) stable-sort by
descending filename. The intent is that pre-reversing preserves directory
ordering (because `sort.SliceStable` keeps equal elements in their prior relative
order). But this only works if the input from `filepath.Glob`/doublestar arrives
in lexicographical directory order. If it does not, the stable sort sees a wrong
relative order for files with equal basename and produces an incorrect result.

**`pkg/logs/launchers/file/provider/file_provider.go:259-264`** — filesLimit
sentinel at the call site:

```go
if !p.reachedNumFileLimit && len(filesToTail) == p.filesLimit {
    log.Warn("Reached the limit on the maximum number of files in use: ", p.filesLimit)
    p.reachedNumFileLimit = true
}
```

This is where `reachedNumFileLimit` is set to `true`, gating the `Reachable`
sentinel recommended below.

**`pkg/logs/launchers/file/provider/file_provider_test.go:644-660`** — the
skipped test:

```go
t.Run("Multiple Directories - Out of order input", func(t *testing.T) {
    t.Skip() // See FIXME in 'applyOrdering', this test currently fails
    ...
})
```

This is the best-known evidence that the bug exists and is acknowledged.

## Failure Scenario

Under Antithesis, the OS filesystem can return `filepath.Glob` results in
non-lexicographical order (e.g., on a filesystem that sorts by inode rather than
name, or after rapid file creation that populates directory entries out of order).
When there are more matching files than `filesLimit` (the cap, defaulting to
`logs_config.open_files_limit`, typically 200-500), the wrong files are selected
for tailing. The operator expects the most recent files (by name heuristic) to
be tailed; instead, arbitrarily ordered files are tailed. This is a **silent
misconfiguration**: no error is logged, no metric signals wrong selection.

## Assertion Design

**`Reachable`** (SUT-side): At `file_provider.go:259` (the `reachedNumFileLimit`
sentinel), add:

```
antithesis.Reachable(
    "wildcard-file-ordering-stable: filesLimit-cap-reached",
    map[string]any{"filesLimit": p.filesLimit, "numFiles": len(filesToTail)},
)
```

This sentinel confirms Antithesis explored a run with enough wildcard-matching
files to hit the cap, making the ordering decision consequential.

**`Always`** (workload-side): The workload creates N files matching a wildcard
pattern (N > filesLimit) with filenames that encode priority (e.g., date-ordered
names across multiple directories). After the provider runs, assert that the files
selected for tailing are the top-N by the expected reverse-lexicographic ordering.
The workload can read the active tailer list from the agent's `/agent/status`
endpoint or from fakeintake log origins. Any selection that omits a
higher-priority file in favor of a lower-priority file is a violation.

**`AlwaysOrUnreachable`** (SUT-side, optional): At the entry of
`applyReverseLexicographicalOrdering`, assert that the input slice is in
lexicographic order (to confirm the assumption holds in the current environment).
If it ever fires on a non-sorted input, the underlying Go stdlib behavior changed.

## Why It Matters

Production deployments with many log files (Kubernetes nodes with hundreds of
containers) frequently hit `filesLimit`. When they do, which files are tailed is
determined by the ordering heuristic. A wrong ordering means important log files
(by recency/date-naming) are silently not tailed. Operators get no signal.
This is a silent observability gap — the agent reports healthy, but some log
streams are missing. The FIXME and skipped test confirm the maintainers know
about this but have not fixed it.

Under Antithesis, filesystem behavior (inode allocation order, directory entry
ordering) may differ from a standard Linux ext4 mount. This makes the unsorted-
input scenario more likely to be explored than in a standard test environment.

## Relationship to Other Properties

- `backpressure-no-rotation-loss` — covers the rotation/backpressure drop path;
  this property covers a different loss mechanism (wrong file selection before
  tailing begins).
- `per-source-ordering-preserved` — ordering within a pipeline for a single
  source; this property covers which *sources* are selected at all.

## Open Questions

- Does the Antithesis filesystem topology guarantee lexicographic directory
  enumeration from `filepath.Glob`? If yes, this property may be
  `AlwaysOrUnreachable` in the sense that the bug never fires in the planned
  topology. `(needs human input)`
- What is the default `open_files_limit` in the planned test configuration? The
  filesLimit-cap-reached sentinel is only reachable if the workload creates more
  files than this limit.
- Does `doublestar.FilepathGlob` (used when `logs_config.enable_recursive_glob`
  is true) also return results in lexicographic order, or does it have different
  guarantees? `(partial: doublestar is a third-party library; its sort behavior
  is not specified in its README)`

### Investigation Log

#### Does filepath.Glob guarantee lexicographic order in Go 1.19+?

- Examined: `file_provider.go:362-368` (FIXME comment), `file_provider_test.go:644-660` (skipped test), Go stdlib source referenced in the FIXME comment (`go1.19:src/path/filepath/match.go:330`), Go issue #17153.
- Found: The FIXME explicitly cites Go issue #17153. The Go stdlib `filepath.Glob` returns matches in the order returned by `os.ReadDir`, which sorts by name on all current platforms but is not guaranteed by the API contract. The skipped test uses the out-of-order input `[/tmp/1/2018.log, /tmp/2/2018.log, /tmp/3/2016.log, /tmp/3/2018.log, /tmp/1/2017.log]` and currently fails.
- Not found: any evidence that this has triggered a production incident, but the FIXME and skipped test confirm active awareness of the bug.
- Conclusion: the bug is real and acknowledged. Whether Antithesis can trigger it depends on whether the test filesystem returns non-sorted Glob results.
