---
slug: offset-no-regression-on-seek-error
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# offset-no-regression-on-seek-error — Seek Error Does Not Cause Offset Regression to Zero

## What Led to This Property

SUT failure mode #4 from the SUT analysis: `tailer_nix.go:36` discards the
error return from `f.Seek()` with `ret, _ := f.Seek(offset, whence)`. If the
seek fails, `ret` is 0 (the zero value), and `t.lastReadOffset.Store(ret)` and
`t.decodedOffset.Store(ret)` both store 0. The tailer then begins reading from
the beginning of the file — a **duplicate storm**.

## Code Paths Involved

**The exact bug location** — `pkg/logs/tailers/file/tailer_nix.go:36`:
```go
ret, _ := f.Seek(offset, whence)
```

The error is silently discarded. The return value `ret` is the new file offset
on success; on failure, `ret` is unspecified (typically 0 on Linux for most
seek errors).

**Downstream effect:**
- `t.lastReadOffset.Store(ret)` — stores 0 if seek failed.
- `t.decodedOffset.Store(ret)` — stores 0.
- `readForever()` calls `read()` which calls `t.osFile.Read()` → reads from
  byte 0.
- All lines from the beginning of the file are re-decoded and re-forwarded.

**Auditor interaction:**
- The registry for this source still holds the old (correct) offset.
- But the tailer is now reading from 0, so it will generate messages with
  offsets 0, 1, 2, … up to the real last-read offset.
- The auditor `updateRegistry()` uses `IngestionTimestamp` monotonicity to
  avoid regression: it will *not* store a new offset if the new
  `IngestionTimestamp` is less than the current one (`auditor.go:386-389`).
- **However**: on a fresh tailer start after a restart, the in-memory registry
  is recovered from disk. The new tailer's messages will have fresh ingestion
  timestamps that are *greater* than those in the recovered registry. So the
  auditor *will* update the offset — regressing from the correct value to
  wherever the seek-error restart landed.

**Identifier collision** (related risk from FIXME at `tailer.go:260`):
- If two tailers share the same registry key (e.g., during container rotation),
  a seek error on the new tailer writes offset 0, overwriting the old tailer's
  valid offset in the registry.

## Failure Scenario

1. Agent starts and recovers offset=10000 for `/var/log/app.log`.
2. Tailer calls `f.Seek(10000, io.SeekStart)`.
3. The underlying disk has a transient I/O error (injectable via Antithesis
   filesystem fault).
4. `Seek()` returns `(0, EIO)`. The error is silently discarded; `ret=0` is
   stored.
5. Tailer reads from byte 0: all 10000 bytes are re-decoded and re-forwarded.
6. Intake receives 10000 bytes of duplicate logs.
7. The auditor, seeing fresh timestamps, updates the registry offset to wherever
   the re-read lands — possibly back to 10000, or further, depending on timing.

In the worst case: the seek error happens every restart, causing an O(N²) replay
storm as the file grows.

## Why It Matters

A seek error causes unbounded duplicate delivery — potentially re-reading entire
log files on every restart. In production, this leads to:
- Massive duplicate log volume (cost impact).
- Potential infinite loop if the disk error is persistent.
- Auditor offset confusion that may prevent recovery even after the disk heals.

The silent error discard (`_, _`) is a Go anti-pattern that the codebase's own
review guidelines flag. This is a regression target (the FIXME comment at
`tailer.go:260` acknowledges the general class of problem).

## Workload Instrumentation

- Workload writes N distinct log lines to a file.
- After the tailer has processed and the auditor has flushed (verifiable via
  sequence number tracking), inject a filesystem fault that causes the next
  `Seek()` call on that file to return an error.
- Restart the tailer (or the whole agent).
- Fakeintake counts duplicate deliveries of lines 1..N.
- Assertion: duplicates exist (expected under at-least-once), but no line is
  delivered more than `ceil(N / chunk_size)` times (i.e., no full re-read
  storm).
- SUT-side: an `Unreachable` assertion at the seek error discard site, or a
  `Sometimes` assertion confirming the seek error is observed and handled (not
  silently swallowed) — currently **missing**.

## Open Questions

- What does `f.Seek()` return on Linux when it fails? POSIX says the file
  position is unspecified on error. `afero.File.Seek()` wraps the OS `lseek()`
  syscall — the Go `os.File.Seek()` returns 0 and the error on failure (not -1).
  So `ret=0` is stored, causing re-read from byte 0. `(partial: confirmed ret=0 is stored on error from tailer_nix.go:36; full lseek syscall return value on POSIX error still defers to OS behavior)`
- Does the `fileOpener.OpenLogFile()` return an afero `File` whose `Seek` can
  return a meaningful error in test environments? If the test uses an in-memory
  filesystem (e.g., `afero.MemMapFs`), seek errors may be uninjectable and this
  property may require a custom fault-injection hook.

### Investigation Log

#### What does `f.Seek()` return on Linux when it fails, and what does storing `ret=0` cause?

- Examined: `pkg/logs/tailers/file/tailer_nix.go:36-43`.
- Found: `ret, _ := f.Seek(offset, whence)`. In Go, `os.File.Seek()` returns `(newOffset int64, err error)`. On error, Go's stdlib returns the new offset as reported by the kernel — for `lseek()` failure, POSIX says the file offset is unspecified; Linux's `lseek(2)` returns -1 on error and does not change the file offset, but Go's `os.File.Seek` wraps this: it returns `(0, err)` when the syscall returns -1 (the 0 comes from casting -1 to int64 after the error path, but the standard afero wrapper for `os.File` would return whatever the OS reports). The bottom line: on failure, `ret` is likely 0 (Go wraps -1 kernel return to 0 in the error case for `os.File`), which gets stored in `lastReadOffset` and `decodedOffset`, causing the tailer to read from byte 0.
- Conclusion: tagged `(partial)` — the exact return value on error is OS/wrapper dependent, but the safe assumption is `ret=0` causes full re-read from beginning.

#### Is there Seek error handling in `tailer_windows.go`?

- Examined: `pkg/logs/tailers/file/tailer_windows.go:37` — `filePos, _ := f.Seek(offset, whence)`. Same pattern: error is discarded. Additionally Windows re-opens file on each read (line 69-70), so seek error in `setup()` stores 0; but the re-seek in `readAvailable()` at line 100 does return the error, which causes a `return bytes, err` — that error is handled by `read()` at line 146-151 (logs it and returns).
- Found: Windows setup seek (`tailer_windows.go:37`) also discards error. The read-loop seek (`readAvailable():100`) does NOT discard error — it returns it, and `read()` logs and stops the tailer. So the Windows setup path has the same bug, but the in-loop seek is handled.
- Conclusion: Windows does not provide a clean model to copy. The nix path is the primary concern. Resolved; removed from Open Questions (replaced by the two remaining questions).
