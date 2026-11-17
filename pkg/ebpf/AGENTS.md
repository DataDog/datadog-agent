# Testing system-probe and eBPF changes

How to test a change to system-probe or eBPF code, and what you may claim afterward. It applies to
everything system-probe loads: `pkg/ebpf/`, `pkg/network/`, `pkg/collector/corechecks/ebpf/`,
`cmd/system-probe/`, and the eBPF C sources under them.

## Check the host first

Run `uname -sm` before planning any testing. Kernel code only runs on Linux, and results only hold for
the kernel and architecture they ran on.

- **macOS and Windows can only typecheck.** Never load eBPF programs, run `linux_bpf` tests, or run
  system-probe there.
- **Don't use a dev container as the test kernel.** A container runs on its host's kernel, which you
  don't control and which isn't in the test matrix.
- **No Linux target means no test run.** Say so and ask the user for a Linux VM. Never report a change
  as tested because it typechecks.

### Typecheck from macOS

- Cross-lint the Linux packages with the same build set CI uses for eBPF code:
  `GOOS=linux GOARCH=<amd64|arm64> dda inv linter.go --build system-probe-unit-tests --targets=./pkg/network/tracer`.
  It needs a Linux cross C compiler on `PATH` (for arm64, `aarch64-linux-gnu-gcc` or
  `aarch64-linux-gnu-clang`) or `DD_CC` and `DD_CXX` set, because these packages use cgo; the task
  stops and says so if neither is present.
- Runtime-compilation assets in `pkg/ebpf/bytecode/runtime/` (for example `tracer.go`) are generated
  and gitignored, so packages that import them don't typecheck until they exist. Their Bazel targets
  are Linux-only, so on macOS pass a Linux platform:
  `bazel run --platforms=@rules_go//go/toolchain:linux_arm64 //pkg/ebpf/bytecode:tracer_verify`
  writes `tracer.go` (one `<bundle>_verify` target per bundle in `pkg/ebpf/bytecode/BUILD.bazel`).
- Typechecking proves the Go compiles for Linux. It doesn't compile the eBPF C code or load anything.

## Use the lowest layer that proves the claim

| Layer | Proves | Doesn't prove |
| --- | --- | --- |
| macOS or Windows host | Go typechecks; Gazelle and buildifier are clean; pure-Go unit tests without the `linux` tag pass | Anything about eBPF or the kernel |
| Linux VM (KMT local VMs, or any Linux VM you control) | Behavior on that VM's kernel and architecture, in each build mode that kernel supports; verifier output; object disassembly; per-program `bpf_stats` | Other kernels, other architectures, and modes that kernel can't run |
| KMT on Datadog's CI for PRs | The distro and kernel matrix (old kernels through current), x86_64 and arm64, prebuilt, and the root-only test packages | Cluster-level behavior |
| E2E and manual QA | Behavior in Kubernetes and cloud environments (DaemonSets, service meshes, real intake) | Per-kernel correctness |

- **Linux VM:** build the objects with `dda inv system-probe.object-files`, then run
  `dda inv system-probe.test --packages=./pkg/network/tracer --skip-object-files --run=TestName`. The
  task runs `go test` under `sudo -E` with the system-probe build tags; don't run `go test` directly.
- **KMT locally:** set up and run VMs as described in `tasks/kernel_matrix_testing/README.md`, which
  also covers macOS hosts.
- **E2E and manual QA:** see `doc/how-to/test/e2e/` and `doc/how-to/test/manual-qa/`.
- Test categories in general: `doc/guidelines/testing/test-categories.md`.

## Build modes and kernels

- **List the modes the change touches** before testing: prebuilt, runtime-compiled, CO-RE, fentry, and
  ebpfless. Report evidence for each one, or mark it not run. Most mode- and kernel-specific bugs pass
  in every other mode: a CO-RE loader can drop an option the runtime loader keeps, a verifier on an old
  kernel can reject a program a newer one accepts, a runtime compile can fail `-Werror` on one distro's
  headers, and a program can exceed `optmem_max` on one kernel only.
- **Read which modes actually ran.** `ebpftest.SupportedBuildModes` (in `pkg/ebpf/ebpftest/`) decides
  per host:
  - Prebuilt is skipped where it's deprecated (kernel 6.0 and later, or 5.14 and later on RHEL) unless
    `TEST_PREBUILT_OVERRIDE=true`.
  - Fentry runs only when the kernel supports it, which depends on the kernel patch level, not just the
    version.
  - Ebpfless runs only with `TEST_EBPFLESS_OVERRIDE=true`.

  Check the subtest names in the output instead of assuming a mode ran.
- **Ask for equivalence, not one mode.** If a change must behave the same in every mode, test runtime
  compiled and CO-RE at least, not only CO-RE.
- **Prebuilt on a modern VM:** forcing prebuilt can fail for reasons unrelated to your change, such as
  offset guessing on newer kernels. Say so and rely on KMT for prebuilt.

## KMT in CI

- **KMT runs only for PRs based on `main`** or a release branch. A stacked PR (based on another branch)
  gets a light pipeline without KMT. Retarget the PR to `main`, or run KMT locally.
- **KMT runs only when system-probe paths change.** Otherwise its jobs are manual.
- **KMT waits for `tests_ebpf_<arch>`,** which builds the objects and lints `./pkg` with
  `--build system-probe-unit-tests`. If that job fails, no KMT test runs.
- **Check which jobs ran before reading a result.** A red pipeline where KMT never started says nothing
  about your change, and a pending status can be a lost status update rather than a running job.
- **Only listed packages run.** `system-probe.test` and KMT test only `TEST_PACKAGES_LIST` in
  `tasks/system_probe.py`. Tests that need root and live outside those packages don't run anywhere.
- **Flaky tests:** rerun once, record both results, and report the test as inconclusive. Never retry
  until it's green.

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

## Hygiene

- **Gazelle covers test files too.** A new `_test.go` file or a new test import that Gazelle didn't
  pick up is the usual `bazel:run-gazelle` failure in these packages.
- **Every commit must build and pass,** not only the branch head, so a stacked change can be reviewed
  and reverted commit by commit.
- **Check free disk space before building on a VM.** A full disk produces partial builds and stale
  objects that look like test results.
- **Shared VMs:** `system-probe.test` reads objects from the shared install directory
  (`/opt/datadog-agent/embedded/share/system-probe/ebpf`), so serialize object builds and test runs
  with `flock`. Stop processes by PID, never by name, and never reboot a VM without asking.

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

Ask the user before anything that needs credentials or cloud spend (remote KMT VMs, cloud labs),
rebooting or reconfiguring a VM, pushing, opening or retargeting a PR, or deleting resources you
didn't create.
