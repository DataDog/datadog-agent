# YARA M1 integration: status (WIP checkpoint)

## Done (committed on `yara/m1`)
- `009b5c5e95` fileaccess: inode + ctime check on path fallbacks (`/proc/<pid>/root`, other container PIDs); mismatch → next candidate, else `ReadErrorOther`, identity not marked. Tests use real inode/ctime.
- `c7895c1c0f` rules: refuse `rules_dir` / rule files not owned by uid 0 or group/world writable (`ErrUnsafeRules`, `trustedRuleOwnerUID` var for tests); `versionedScanner.Close()` forwards to an `io.Closer` engine scanner.
- `a41629ea93` doc: `Stats.Matches` = individual rule matches.
- `d8e0b83d41` `pipeline.go`: `NewPipeline(cfg, statsd, PipelineOpts)` + `NewExecScanner(evm, cfg)`; consumer Start/Stop drive pool + metrics; ContainerPIDs from the CWS cgroup resolver; registered in `eventmonitor_linux.go`; `createEventMonitorModule` logs yara errors instead of failing. Rules load failure → consumer not registered.
- Plan M1 checklist boxes ticked.

## Verified
- `dda inv test --targets=./pkg/eventmonitor/consumers/yara,./pkg/system-probe/config --race`: pass (as uid 502 in the dev container); whole yara suite also passes as root via `bazel run --run_under='sudo -n -E'`.
- `dda inv linter.go --targets=./pkg/eventmonitor/consumers/yara,./cmd/system-probe/modules`: 0 issues.
- Gazelle regenerated BUILD.bazel.

## Not done / unverified
- `dda inv system-probe.build` NOT run yet (next step).
- `TestExecScannerEventMonitor` (real event monitor, root + eBPF) always skipped so far: in the dev container even as root, `ProcessEventDataStreamSupported()` is false. Needs a real Linux VM.
- Windows compile not checked (only `eventmonitor.go` changed there, no new symbols).
- Overlayfs: the fstat inode through an overlay mount may differ from the eBPF inode, so container path fallbacks could fail the check (counted as `read_errors reason:other`). Check on the VM.

## Next step
Run `dda inv system-probe.build` in the dev container, then the VM dry run.
