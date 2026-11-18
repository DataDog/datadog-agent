# Testing system-probe and eBPF changes

How to test a change to system-probe or eBPF code, and what you may claim afterward. It applies to
`pkg/ebpf/`, `pkg/network/`, `pkg/collector/corechecks/ebpf/`, `cmd/system-probe/`, `pkg/system-probe/`,
`pkg/util/kernel/`, and `pkg/process/monitor/`, including the eBPF C sources under them.

This file holds the workflow and the rules for claims. CI behavior and other pitfalls that the test
framework doesn't handle for you are in [`TESTING_GOTCHAS.md`](TESTING_GOTCHAS.md).

## Workflow

Run `uname -sm` first. Results only hold for the OS, kernel, and architecture they ran on.

1. **Typecheck on any host.** Cross-lint with the build set CI uses for eBPF code:
   `GOOS=linux GOARCH=<amd64|arm64> dda inv linter.go --build system-probe-unit-tests --targets=./pkg/network/tracer`.
   It needs a Linux cross C compiler (`DD_CC` and `DD_CXX`, or for arm64 `aarch64-linux-gnu-gcc` on
   `PATH`) because these packages use cgo; without one the typecheck is `blocked`. Typechecking proves
   the Go compiles for Linux, nothing more: it doesn't compile the eBPF C code or load anything.
2. **Develop against local KMT VMs.** KMT launches VMs with the right kernels, and `kmt.test` builds
   the objects and test binaries from your tree and copies them to the VM, so each run tests your
   current change. Setup, including macOS hosts, is in `tasks/kernel_matrix_testing/README.md`; the
   minimal loop from it is:

   ```bash
   dda inv -e kmt.init --images=ubuntu_22.04          # once; resets existing KMT state
   dda inv -e kmt.gen-config --vms=ubuntu_22.04-local-distro
   dda inv -e kmt.launch-stack
   dda inv -e kmt.test --vms=ubuntu_22.04-local-distro --packages=./pkg/network/tracer --run=TestName
   dda inv -e kmt.destroy-stack                       # when done
   ```

   - **An existing Linux VM** (Parallels, VMware, a cloud host) works through an alien profile: pass
     `--alien-vms=<profile.json>` to `kmt.test` and `kmt.build` instead of `--vms`. The profile format
     is in the README.
   - **`kmt.build`** builds system-probe itself and copies it to the VMs, for runs outside the test
     suite.
   - **Local VMs match the host architecture.** Behavior specific to the other architecture needs
     remote KMT VMs (EC2 metal instances, so ask first) or CI.
   - `kmt.build` and `kmt.test` compile inside a Docker container (`tasks/kernel_matrix_testing/compiler.py`),
     so Docker must be running and responsive; if `docker ps` hangs, the KMT run is `blocked`.
   - The compiler image comes from Datadog's internal registry. Outside contributors can use any Linux
     VM with `dda inv system-probe.object-files` and `dda inv system-probe.test`.
   - Never load eBPF programs on macOS or Windows, and don't use a dev container as the test kernel:
     it runs on a host kernel you don't control. Windows system-probe code (`*_windows.go`) is tested
     natively on Windows with `dda inv system-probe.test`.
3. **Run KMT in CI once the change is stable.** CI runs the full matrix: the KMT image kernels (old
   through recent; `dda inv kmt.ls`), x86_64 and arm64, in every mode each kernel supports. It doesn't
   cover kernels or patch levels outside the images, or cluster behavior. Don't use it as the
   development loop. Read [`TESTING_GOTCHAS.md`](TESTING_GOTCHAS.md) before trusting a CI result.
4. **Cluster and cloud behavior** (DaemonSets, service meshes, real intake) needs E2E or manual QA:
   `doc/how-to/test/e2e/` and `doc/how-to/test/manual-qa/`.

If no suitable target exists, say so and ask the user for one. Never report a change as tested because
it typechecks.

## Build modes

- **List the modes the change touches** before testing. There are two axes: the object build
  (prebuilt, runtime-compiled, CO-RE) and the tracer (kprobe, fentry, SK, ebpfless); fentry and SK are
  CO-RE builds with a different tracer. Mode- and kernel-specific bugs usually pass in every other mode.
- **Read which modes ran from the subtest names.** The modes a test runs depend on the host
  (`ebpftest.SupportedBuildModes` in `pkg/ebpf/ebpftest/`) and on the suite; don't assume.
- **For a change that must behave the same in every mode,** test runtime-compiled and CO-RE at least.
- **The verifier runs only when a program loads.** Building the objects proves the C compiles;
  verifier acceptance needs a load on a real kernel, ideally including the oldest kernel the program
  targets.

## Evidence and honest claims

- **Report each claim per mode and kernel** as `passed`, `failed`, `blocked`, or `inconclusive`, and
  say which were not run. "Typechecks" and "compiles" never mean tested. When nothing ran locally, say
  so plainly, for example "none of the eBPF code was compiled or run locally; KMT is the first run".
- **Prove a regression test catches the bug:** revert the fix, show the test fails for the expected
  reason, then restore it.
- **Never weaken a test, an expectation, or a gate to get green.** Fix the code instead.
- **Name any non-default configuration a result depends on,** such as a `system-probe.yaml` on the
  test machine.

## Performance claims

- **Interleave variants and normalize against a control.** Warm up, use fixed windows, run each
  variant several times, and treat differences inside the run-to-run spread as noise. For per-call
  costs, load both variants at once so every call runs both, and alternate the load order.
- **Normalize by work done.** On a saturated node, lower overhead shows up as more throughput, not less
  CPU; check that the achieved load matches between variants.
- **Confirm you measured the code you built:** the loaded program's tag should differ between
  variants, and the tracer that loaded should match the configuration.
- **Resolve probe misses; don't average them.** kprobe and fentry programs can both miss, under
  different conditions depending on the attach point and kernel. A run with misses did less work, so
  find and fix the cause before comparing. The `ebpf` check reports them
  (`ebpf.kprobes.kprobe_nesting_misses`, `ebpf.kprobes.kretprobe_maxactive_misses`,
  `ebpf.programs.recursion_misses`).
- **Keep `kernel.bpf_stats_enabled` the same in every arm**; it adds cost to every BPF call. BPF
  run-time counters miss kprobe dispatch and socket-filter softirq time, so use `perf` for those.
- **Loopback and cross-container traffic are valid modes.** Measure them as their own scenario, not as
  a stand-in for cross-host traffic.
- **Overhead includes the application's kernel time.** eBPF programs run in the context of the process
  that triggers them, so system-probe's own CPU metrics can miss most of the cost.

A fuller guide to load-testing CNM probes (probe layers, the traffic that drives each one, and the
surrounding kernel's concurrency) is future work.

## Stop and ask

Ask the user before anything that needs credentials, `sudo` you don't already have, or cloud spend
(remote KMT VMs, cloud labs); rebooting or reconfiguring a VM; pushing, opening, or retargeting a PR; or
deleting resources you didn't create.
