# Testing system-probe and eBPF changes

How to test a change to system-probe or eBPF code, and what you may claim afterward. It applies to
`pkg/ebpf/`, `pkg/network/`, `pkg/collector/corechecks/ebpf/`, `cmd/system-probe/`, `pkg/system-probe/`,
`pkg/util/kernel/`, and `pkg/process/monitor/`, including the eBPF C sources under them.

## Check the host first

Run `uname -sm` before planning any testing. Results only hold for the OS, kernel, and architecture they
ran on.

- **Linux and eBPF code needs a Linux kernel.** Never load eBPF programs or run `linux_bpf` tests on
  macOS or Windows.
- **Windows system-probe code is tested on Windows.** The Windows implementation (`*_windows.go`, the
  network tracer built on the Windows driver) runs its tests natively with `dda inv system-probe.test`;
  macOS can only typecheck it.
- **Don't use a dev container as the test kernel.** A container runs on its host's kernel, which you
  don't control and which isn't in the test matrix.
- **No suitable target means no test run.** Say so and ask the user for one. Never report a change as
  tested because it typechecks.

### Typecheck Linux code from macOS

- Cross-lint with the same build set CI uses for eBPF code:
  `GOOS=linux GOARCH=<amd64|arm64> dda inv linter.go --build system-probe-unit-tests --targets=./pkg/network/tracer`.
  It needs a Linux cross C compiler on `PATH` (for arm64, `aarch64-linux-gnu-gcc` or
  `aarch64-linux-gnu-clang`) or `DD_CC` and `DD_CXX` set, because these packages use cgo. Without one,
  the typecheck is `blocked`; report it that way.
- Runtime-compilation assets in `pkg/ebpf/bytecode/runtime/` (for example `tracer.go`) are generated
  and gitignored, so packages that import them don't typecheck until they exist. Their Bazel targets
  are Linux-only, so on macOS pass a Linux platform:
  `bazel run --platforms=//bazel/platforms:linux_<x86_64|arm64> //pkg/ebpf/bytecode:tracer_verify`
  writes `tracer.go` (one `<bundle>_verify` target per bundle in `pkg/ebpf/bytecode/BUILD.bazel`).
- Typechecking proves the Go compiles for Linux. It doesn't compile the eBPF C code or load anything.

## Use the lowest layer that proves the claim

| Layer | Proves | Doesn't prove |
| --- | --- | --- |
| Any host | Typechecks; Gazelle and buildifier are clean; tests with no Linux build constraint pass | Anything about eBPF or the kernel |
| Linux VM you control | Behavior on that VM's kernel and architecture, in each build mode that kernel supports; verifier output; object disassembly; per-program `bpf_stats` | Other kernels, other architectures, and modes that kernel can't run |
| KMT in CI | The distro and kernel matrix (old kernels through current), x86_64 and arm64, prebuilt | Cluster-level behavior |
| E2E and manual QA | Behavior in Kubernetes and cloud environments (DaemonSets, service meshes, real intake) | Per-kernel correctness |

- **Linux VM:** build the objects with `dda inv system-probe.object-files`, then run
  `dda inv system-probe.test --packages=./pkg/network/tracer --skip-object-files --run=TestName`.
  - Both tasks need `GOPATH` set.
  - Both use `sudo` to write and read `/opt/datadog-agent`, so a non-interactive session hangs on a
    password prompt; confirm sudo works without one, or ask the user, before starting.
- **KMT VMs locally:** `tasks/kernel_matrix_testing/README.md` covers setup, including macOS hosts. It
  pulls build images from Datadog's internal registry, so outside contributors should use any Linux VM
  instead.
- **E2E and manual QA:** see `doc/how-to/test/e2e/` and `doc/how-to/test/manual-qa/`.
- Test categories in general: `doc/guidelines/testing/test-categories.md`.

## Build modes and kernels

- **List the modes the change touches** before testing: prebuilt, runtime-compiled, CO-RE, fentry, and
  ebpfless. Report evidence for each one, or mark it not run. Most mode- and kernel-specific bugs pass
  in every other mode: a CO-RE loader can drop an option the runtime loader keeps, a verifier on an old
  kernel can reject a program a newer one accepts, a runtime compile can fail `-Werror` on one distro's
  headers, and a program can exceed `optmem_max` on one kernel only.
- **Read which modes actually ran.** Tests that call `ebpftest.SupportedBuildModes` (in
  `pkg/ebpf/ebpftest/`) get a per-host list; tests that pass a fixed list run only those modes.
  - Prebuilt is skipped where `prebuilt.IsDeprecated()` is true, unless `TEST_PREBUILT_OVERRIDE=true`.
  - Fentry runs only when the kernel supports it (probed at runtime, since support depends on the patch
    level), never on RHEL 8 based distributions, and never without the `bpf` build tag, unless
    `TEST_FENTRY_OVERRIDE=true`.
  - Ebpfless runs only with `TEST_EBPFLESS_OVERRIDE=true`.

  Check the subtest names in the output instead of assuming a mode ran.
- **Ask for equivalence, not one mode.** If a change must behave the same in every mode, test runtime
  compiled and CO-RE at least, not only CO-RE.
- **Protocol classification is off on RHEL 9+ family kernels** (5.14 and later, including Amazon
  Linux 2023), in both the kprobe and fentry tracers. Classifier changes show no effect there; test
  them on another distro.
- **Prebuilt on a modern VM:** forcing prebuilt can fail for reasons unrelated to your change, such as
  offset guessing on newer kernels. Say so and rely on KMT for prebuilt.

## KMT in CI

KMT runs on Datadog's CI for pull requests. Outside contributors can't see its jobs or logs; ask a
maintainer for the results.

- **When it runs:** the rules are `.on_system_probe_or_e2e_changes_or_manual` and
  `.system_probe_change_paths` in `.gitlab-ci.yml`; read them instead of assuming.
  - A stacked PR (based on a branch other than `main` or a release branch) and the merge queue get a
    light pipeline with no KMT. Ask the user to retarget the PR to `main`, or test on a Linux VM.
  - When no listed path changed, the KMT jobs are manual and allowed to fail, so **a green pipeline
    can mean KMT never ran**. Some packages KMT tests, such as `pkg/system-probe/`, aren't in the path
    list.
- **KMT waits for `tests_ebpf_x64` and `tests_ebpf_arm64`.** They build the objects, lint with
  `--build system-probe-unit-tests`, and run the CWS and dyninst unit tests. If either fails, no KMT
  test runs.
- **Check which jobs ran before reading a result.** A red pipeline where KMT never started says nothing
  about your change, and a pending status can be a lost status update rather than a running job.
- **KMT tests only `TEST_PACKAGES_LIST`** in `tasks/system_probe.py` for system-probe; CWS has its own
  KMT jobs. A root-only test in a package outside that list never runs in system-probe KMT.
- **Flaky tests:** rerun once (in CI, a maintainer retries the job), record both results, and report
  the test as inconclusive. Never retry until it's green.

## Evidence and honest claims

- **Report each claim per mode and kernel** as `passed`, `failed`, `blocked`, or `inconclusive`, and
  say which were not run. "Typechecks" and "compiles" never mean tested. When nothing ran locally, say
  so plainly, for example "none of the eBPF code was compiled or run locally; KMT is the first run".
- **Prove a regression test catches the bug:** revert the fix, show the test fails for the expected
  reason, then restore it.
- **Never weaken a test, an expectation, or a gate to get green.** Fix the code instead.
- **Watch for local configuration in results.** A `system-probe.yaml` on the test machine can enable
  features a test relies on. Name any non-default configuration a result depends on.
- **For refactors meant to be behavior-identical,** use the strongest cheap evidence: byte-identical
  objects, equivalence tests with zero differences, and unchanged verifier instruction counts.

## VM hygiene

- **Check free disk space before building.** A full disk produces partial builds and stale objects
  that look like test results.
- **Shared VMs:** `system-probe.test` reads objects from the shared install directory
  (`/opt/datadog-agent/embedded/share/system-probe/ebpf`), so serialize object builds and test runs
  with one agreed lock, for example `flock -o -x /tmp/system-probe-test.lock dda inv system-probe.test ...`.
  Keep `-o`: without it the Bazel server started by the build inherits the lock and holds it after the
  command exits.
  Stop processes by PID, never by name, and never reboot a VM without asking.

## Performance claims

- **End to end:** interleave the variants (for example A B C B C A C A B), warm up before each window,
  use fixed windows, run each variant several times, and normalize against a control. Treat differences
  inside the run-to-run spread as noise, and write down the limitations.
- **Per call:** load both variants at once so every call runs both, alternate the load order, pin CPUs,
  and repeat each pair. Unpaired runs mislead on hybrid CPUs.
- **Across nodes:** rotate variants so each runs on every node (a Latin square), restart system-probe
  per run, let it settle, and confirm the load hit its target rate. Measure kprobe and fentry
  separately.
- **Overhead includes the application's kernel time.** eBPF programs run in the context of the process
  that triggers them, so system-probe's own CPU metrics can miss most of the cost.

## Stop and ask

Ask the user before anything that needs credentials, `sudo` you don't already have, or cloud spend
(remote KMT VMs, cloud labs); rebooting or reconfiguring a VM; pushing, opening, or retargeting a PR; or
deleting resources you didn't create.
