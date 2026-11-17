# Testing system-probe and eBPF changes

How to test a change to system-probe or eBPF code, and what you may claim afterward. It applies to
`pkg/ebpf/`, `pkg/network/`, `pkg/collector/corechecks/ebpf/`, `cmd/system-probe/`, `pkg/system-probe/`,
`pkg/util/kernel/`, and `pkg/process/monitor/`, including the eBPF C sources under them.

## Check the host first

Run `uname -sm` before planning any testing. Results only hold for the OS, kernel, and architecture they
ran on.

- **Linux and eBPF code needs a Linux kernel.** Never load eBPF programs or run `bpf`-tagged tests on
  macOS or Windows. The tag is `bpf` (`//go:build linux && bpf`), not the old `linux_bpf`; with the old
  tag a test run reports `[no tests to run]` and exits 0, which looks like a pass.
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
- Building the objects (`system-probe.object-files`) proves the C compiles. The verifier only runs when
  a program loads, so verifier acceptance needs a load on a real kernel, ideally including the oldest
  kernel the program targets; clang can, for example, optimize away a bounds mask the verifier needs.

## Use the lowest layer that proves the claim

| Layer | Proves | Doesn't prove |
| --- | --- | --- |
| Any host | Typechecks; Gazelle and buildifier are clean; tests with no Linux build constraint pass | Anything about eBPF or the kernel. Bazel never runs `bpf`-tagged tests (`bazel/configs/go_tests.bazelrc` filters `tagset_bpf`), so a green `bazel test //pkg/...` says nothing about eBPF code |
| Linux VM you control | Behavior on that VM's kernel and architecture, in each build mode that kernel supports; verifier output; object disassembly; per-program `bpf_stats` | Other kernels, other architectures, and modes that kernel can't run |
| KMT in CI | The KMT image kernels (old through recent; see `dda inv kmt.ls`), x86_64 and arm64, in every mode each kernel supports, including prebuilt | Kernels or patch levels not in the images (a distro patch can break a tracer on a kernel the image predates); cluster-level behavior |
| E2E and manual QA | Behavior in Kubernetes and cloud environments (DaemonSets, service meshes, real intake) | Per-kernel correctness |

- **Linux VM:** build the objects with `dda inv system-probe.object-files`, then run
  `dda inv system-probe.test --packages=./pkg/network/tracer --skip-object-files --run=TestName`.
  - Both tasks need `GOPATH` set.
  - Both use `sudo` to write and read `/opt/datadog-agent`, so a non-interactive session hangs on a
    password prompt; confirm sudo works without one, or ask the user, before starting.
- **Misleading local results:**
  - **Stale objects:** tests load objects from `/opt/datadog-agent/embedded/share/system-probe/ebpf`, not
    from the tree. `file content hash does not match expected value` (runtime) or `couldn't find map ...
    unknown section` (CO-RE), with every mode failing in under a second, means rebuild the objects, not
    debug the code.
  - **Stale test binary:** if new subtests are missing from the output, the binary predates the change.
  - **Environment through sudo:** `sudo` can reset the environment, so check that `TEST_*_OVERRIDE`
    variables reach the test (the subtest names show which modes ran).
- **KMT VMs locally:** `tasks/kernel_matrix_testing/README.md` covers setup, including macOS hosts. It
  pulls build images from Datadog's internal registry, so outside contributors should use any Linux VM
  instead.
- **E2E and manual QA:** see `doc/how-to/test/e2e/` and `doc/how-to/test/manual-qa/`.
- Test categories in general: `doc/guidelines/testing/test-categories.md`.

## Build modes and kernels

- **List the modes the change touches** before testing: prebuilt, runtime-compiled, CO-RE, fentry, SK,
  and ebpfless. These are two axes: the object build (prebuilt, runtime-compiled, CO-RE) and the
  tracer (kprobe, fentry, SK, ebpfless); fentry and SK are CO-RE builds with a different tracer. Report
  evidence for each one, or mark it not run. Most mode- and kernel-specific bugs pass
  in every other mode: a CO-RE loader can drop an option the runtime loader keeps, a verifier on an old
  kernel can reject a program a newer one accepts, a runtime compile can fail `-Werror` on one distro's
  headers, and a program can exceed `optmem_max` on one kernel only.
  - **`optmem_max`:** a socket filter is charged its size after JIT constant blinding, which
    `net.core.bpf_jit_harden=2` (set on some hardened distros) can nearly double. Size socket filters
    with `sysctl net.core.bpf_jit_harden=2` as well as the default.
  - **Runtime `-Werror`:** old kernel headers (CentOS 7's 3.10) don't mark `inline` functions
    `__maybe_unused`, so a static inline helper that loses its last caller fails the runtime compile
    there and nowhere else.
- **Read which modes actually ran.** Tests that call `ebpftest.SupportedBuildModes` (in
  `pkg/ebpf/ebpftest/`) get a per-host list; tests that pass a fixed list run only those modes.
  - Prebuilt is skipped where `prebuilt.IsDeprecated()` is true, unless `TEST_PREBUILT_OVERRIDE=true`.
  - Fentry runs only when the kernel supports it (probed at runtime, since support depends on the patch
    level), never on RHEL 8 based distributions, and never without the `bpf` build tag, unless
    `TEST_FENTRY_OVERRIDE=true`.
  - Ebpfless is added by `SupportedBuildModes` only with `TEST_EBPFLESS_OVERRIDE=true`, but some suites
    add modes themselves: `TestTracerSuite` always adds ebpfless and adds SK when the kernel supports
    it, and USM's `testutil.SupportedBuildModes` leaves out fentry, which only
    `SupportedBuildModesForConnectionTracer` adds.

  Check the subtest names in the output instead of assuming a mode ran.
- **Ask for equivalence, not one mode.** If a change must behave the same in every mode, test runtime
  compiled and CO-RE at least, not only CO-RE.
- **Protocol classification is off on RHEL-family hosts with kernel 5.14 or later** (`kernel.Family()`
  reports `rhel` for RHEL, CentOS, Rocky, Amazon Linux, Oracle and others), in both the kprobe and
  fentry tracers. Classifier changes show no effect there; test
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
- **Read results from the job's JUnit artifact, not its log.** KMT job logs can hit GitLab's log size
  limit (verifier output alone can do it), and tests don't run with `-v`.
- **Long verifier logs get truncated** before the failing instruction in Go's test output. To see the
  whole log, write `fmt.Sprintf("%+v", verr)` of the `*ebpf.VerifierError` to a file.
- **A pipeline created through the API** (not by a push) posts per-job statuses but not
  `dd-gitlab/default-pipeline`, so an older failure stays the PR's latest status until the next push.
- **KMT tests only `TEST_PACKAGES_LIST`** in `tasks/system_probe.py` for system-probe; CWS has its own
  KMT jobs. A root-only test in a package outside that list never runs in system-probe KMT.
- **Flaky tests:** KMT already reruns failing tests twice (`RETRY` in `.kmt_run_tests`) and tolerates
  tests in `flakes.yaml`, so a green job can hide a flake; check the JUnit artifact for retried tests.
  To rerun KMT, trigger a new pipeline (maintainers can also use `dda inv kmt.retry-failed-pipeline`);
  retrying a single KMT job fails once its VM is gone. Locally, measure a failure rate (5 to 10 runs,
  alone and inside the full suite) on both the base and your branch before attributing a failure to
  your change. Report it as inconclusive, and never retry until it's green.

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
  An alternative is a private object directory: copy the build output to a directory root owns (the
  CO-RE and fentry loaders reject objects with "incorrect permissions") and pass
  `DD_SYSTEM_PROBE_BPF_DIR=<dir>` to the test through `sudo -E env`. `object-files` updates `/opt` only
  on the machine it runs on, so a tree shared between two VMs leaves the other VM on its old objects.
- Stop processes by PID, never by name, and never reboot a VM without asking.

## Performance claims

- **End to end:** interleave the variants (for example A B C B C A C A B), warm up before each window,
  use fixed windows, run each variant several times, and normalize against a control. Treat differences
  inside the run-to-run spread as noise, and write down the limitations.
- **Per call:** load both variants at once so every call runs both, alternate the load order, pin CPUs,
  and repeat each pair. Unpaired runs mislead on hybrid CPUs.
- **Across nodes:** rotate variants so each runs on every node (a Latin square), restart system-probe
  per run, let it settle, and confirm the load hit its target rate. Measure kprobe and fentry
  separately.
- **Normalize by work done.** On a saturated node, lower overhead shows up as more throughput, not less
  CPU; check that the achieved load (calls/s, MB/s) matches between variants.
- **Keep `kernel.bpf_stats_enabled` the same in every arm.** It adds tens of nanoseconds to every BPF
  call, node-wide.
- **Confirm you measured the code you built.** The loaded program's tag should differ between variants,
  and the tracer that loaded should match the configuration on every node.
- **BPF run-time counters miss kprobe dispatch** (trampolines, the kretprobe instance pool) and the
  softirq cost of socket filters, so `bpf_stats` undercounts kprobe overhead compared with fentry; use
  `perf` for the gap.
- **Loopback** runs the send and receive hooks on the same CPUs, and socket filters see each packet
  twice: fine for per-call comparisons, not for node-level percentages.
- **Overhead includes the application's kernel time.** eBPF programs run in the context of the process
  that triggers them, so system-probe's own CPU metrics can miss most of the cost.

## Stop and ask

Ask the user before anything that needs credentials, `sudo` you don't already have, or cloud spend
(remote KMT VMs, cloud labs); rebooting or reconfiguring a VM; pushing, opening, or retargeting a PR; or
deleting resources you didn't create.
