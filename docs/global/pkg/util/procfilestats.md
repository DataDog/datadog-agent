# pkg/util/procfilestats

## Purpose

Provides a thin, Linux-only wrapper around `gopsutil` to report two numbers about the running agent process: how many file descriptors it currently has open, and what the OS-level `RLIMIT_NOFILE` soft limit is. The data is surfaced in the agent status page for troubleshooting (e.g. to detect fd-leak or misconfigured ulimit).

## Key Elements

| Symbol | Description |
|--------|-------------|
| `ProcessFileStats` | Struct with two `uint64` fields: `AgentOpenFiles` (current open-fd count) and `OsFileLimit` (soft `RLIMIT_NOFILE`). JSON-serialisable. |
| `GetProcessFileStats() (*ProcessFileStats, error)` | **Linux only** — reads stats via `gopsutil/v4/process` using the agent's own PID. Returns `ErrNotImplemented` on non-Linux platforms. |
| `ErrNotImplemented` | Sentinel error returned on macOS/Windows builds (mirrors the internal gopsutil error). |

**Build behaviour:** `process_file_stats_linux.go` is compiled under the `linux` build constraint; `process_file_stats_others.go` (constraint `!linux`) provides a no-op stub. There are no special build tags.

## Usage

Called in two places in the codebase:

- `pkg/logs/launchers/file/launcher.go` — the file-log launcher checks open-fd stats to detect situations where the agent is approaching its descriptor limit.
- `pkg/logs/status/builder.go` — the log-agent status builder includes `ProcessFileStats` in the JSON status payload, making the numbers visible in `agent status` output.

Typical call pattern:

```go
stats, err := procfilestats.GetProcessFileStats()
if err != nil {
    // handle or ignore on non-Linux
}
// use stats.AgentOpenFiles, stats.OsFileLimit
```
