# pkg/util/crashreport

## Purpose

`pkg/util/crashreport` provides Windows-only helpers for detecting and deduplicating Windows Blue Screen of Death (BSOD / kernel crash) events. It coordinates with the `system-probe` crash detection module, reads crash dump metadata via the `WindowsCrashDetectModule`, and persists a "already reported" marker in the Windows registry so the same crash is not emitted more than once across agent restarts.

This package is compiled only on Windows (`//go:build windows`).

---

## Key elements

### `WinCrashReporter`

The central type. Holds the registry hive and key path used for deduplication, a reference to the system-probe client, and bookkeeping state for startup-error grace periods.

**`NewWinCrashReporter(hive registry.Key, key string) (*WinCrashReporter, error)`** — constructs a reporter that reads/writes the deduplication marker under `hive\key` in the Windows registry, and connects to the system-probe via its configured socket.

**`(*WinCrashReporter).CheckForCrash() (*probe.WinCrashStatus, error)`** — the main polling method, intended to be called on each check run:

1. Returns `(nil, nil)` after the first successful query (`hasRunOnce = true`), so crash data is only reported once per agent lifetime.
2. Queries `system-probe` via `WindowsCrashDetectModule`. Returns `(nil, nil)` if the probe is still starting up (startup errors below `maxStartupWarnings = 20` are swallowed).
3. Returns `(nil, nil)` if the probe reports `WinCrashStatusCodeBusy` (dump analysis in progress).
4. Returns `(nil, error)` if the probe reports `WinCrashStatusCodeFailed`.
5. Returns `(nil, nil)` if no crash dump file is present.
6. Checks the registry for a previously stored `"<filename>_<datestring>"` marker. If it matches the current crash, logs and returns `(nil, nil)` (duplicate suppression).
7. Writes the new marker to the registry and returns a pointer to the `WinCrashStatus`.

### Registry deduplication

The marker value stored under `<baseKey>\lastReported` has the form `<FileName>_<DateString>` (both fields from `probe.WinCrashStatus`). This persists across agent restarts so the same BSOD is not reported after an agent upgrade or service restart.

### Startup error grace period

When system-probe has not yet started, `CheckForCrash` returns a retryable error (`retry.IsErrWillRetry`). The reporter silently swallows up to `maxStartupWarnings` (20) consecutive such errors before surfacing them, giving system-probe time to come up before raising an alert.

---

## Usage

**`pkg/collector/corechecks/system/wincrashdetect/wincrashdetect.go`** and **`comp/checks/agentcrashdetect/impl/agentcrashdetect.go`** — both Windows agent checks instantiate a `WinCrashReporter` on startup and call `CheckForCrash()` on each check run. When a non-nil `WinCrashStatus` is returned, they emit a Datadog event describing the BSOD.

Typical usage:

```go
reporter, _ := crashreport.NewWinCrashReporter(registry.LOCAL_MACHINE, `SOFTWARE\Datadog\Datadog Agent\crash`)

// inside the check Run() method:
crash, err := reporter.CheckForCrash()
if err != nil {
    return err
}
if crash != nil {
    sender.Event(metrics.Event{
        Title: "Windows crash detected",
        Text:  crash.FileName,
        // ...
    })
}
```

---

## Relationship to other packages

| Package / component | Relationship |
|---|---|
| `pkg/windowsdriver` ([docs](../windowsdriver.md)) | `pkg/windowsdriver` provides Go bindings for Datadog's custom Windows kernel drivers (`ddprocmon`, `DDInjector`) via IOCTL communication. `pkg/util/crashreport` consumes the `WindowsCrashDetectModule` exposed by `system-probe` over its HTTP socket — it does not use `pkg/windowsdriver` directly. If a crash is caused by a kernel driver, `crashreport` reports it but relies on the module for raw dump metadata. |
| `pkg/util/winutil` ([docs](winutil.md)) | `pkg/util/winutil` manages Windows Service lifecycle, SCM monitoring, and the Event Log. `pkg/util/crashreport` uses the Windows registry (via `golang.org/x/sys/windows/registry`) for deduplication markers, not the Event Log; however the checks that consume `WinCrashReporter` typically call `sender.Event` (Datadog events), not the Windows Event Log. For writing Windows Event Log entries from an agent service, use `winutil.LogEventViewer` instead. |
