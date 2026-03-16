# Windows Dev Env — Benchmark Results

Instance type: `t3.2xlarge` (8 vCPU, 32 GB RAM)
Image: `datadog/agent-buildimages-windows_x64:ltsc2022`
Commands run via `dda inv windows-dev-env.run` (rsync + `Invoke-BuildScript` wrapper)

---

## Tests — `inv test --build-stdlib`

20 614 tests, 62 skipped (Windows-only skips), all passed.

| Run | Wall time | gotestsum time | Notes |
|---|---|---|---|
| Cold (no build cache) | **2987s (~49.8 min)** | 2334.6s | First run, full compilation of all packages |
| Warm (build cache hit) | **1696s (~28.3 min)** | 1534.7s | Go build cache populated, only test binaries re-linked |

Cache speedup: **~1.76×** (saved ~21.8 min)

---

## Linters — `inv linter.go`

0 issues.

| Run | Wall time | golangci-lint time | Notes |
|---|---|---|---|
| Cold (no lint cache) | **1443s (~24.1 min)** | 1305.8s | First run, full analysis of all packages |
| Warm (lint cache hit) | **74s (~1.2 min)** | 39.3s | golangci-lint cache fully warm |

Cache speedup: **~19.5×** (saved ~22.9 min)

---

## Summary

| Command | Cold | Warm | Speedup |
|---|---|---|---|
| `inv test --build-stdlib` | 49.8 min | 28.3 min | 1.76× |
| `inv linter.go` | 24.1 min | 1.2 min | 19.5× |

### Observations

- **Linter cache is extremely effective**: golangci-lint drops from 1305s to 39s on the second run (33×
  speedup on the lint step itself). The remaining warm wall time (74s) is mostly rsync + Invoke-BuildScript
  overhead.
- **Test cache is partial**: Go caches compiled packages but re-runs all test binaries every time (Go does
  not cache test results for `./...` unless explicitly using `go test -count=0` semantics). The 1.76×
  speedup comes purely from not recompiling.
- **rsync + SSH overhead** accounts for the ~160s gap between wall time and gotestsum/golangci-lint time
  on each run. For incremental dev loops (`--only-modified-packages`), this overhead dominates; per-file
  rsync (see file watcher changes) is key to keeping that fast.
