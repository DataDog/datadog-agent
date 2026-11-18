# system-probe and eBPF testing gotchas

Pitfalls the test framework doesn't handle for you. The workflow is in [`AGENTS.md`](AGENTS.md). Add
new entries under the closest heading, one short bullet each.

## Typechecking

- **The build tag is `bpf`** (`//go:build linux && bpf`), not the old `linux_bpf`. With the old tag a
  test run reports `[no tests to run]` and exits 0, which looks like a pass.
- **Bazel never runs `bpf`-tagged tests** (`bazel/configs/go_tests.bazelrc` filters `tagset_bpf`), so a
  green `bazel test //pkg/...` says nothing about eBPF code.
- **Runtime-compilation assets** in `pkg/ebpf/bytecode/runtime/` are generated and gitignored, so
  packages that import them don't typecheck until they exist. On macOS, generate them with a Linux
  platform, for example
  `bazel run --platforms=//bazel/platforms:linux_<x86_64|arm64> //pkg/ebpf/bytecode:tracer_verify`.

## Mode- and kernel-specific failures

- **`optmem_max`:** a socket filter is charged its size after JIT constant blinding, which
  `net.core.bpf_jit_harden=2` (set on some hardened distros) can nearly double. Size socket filters
  with that setting as well as the default.
- **Runtime `-Werror`:** old kernel headers don't mark `inline` functions `__maybe_unused`, so a static
  inline helper that loses its last caller fails the runtime compile there and nowhere else. To catch
  it on a modern kernel, temporarily add `-DKBUILD_EXTRA_WARN1` (headers 5.4 to 6.17) or
  `-DKBUILD_EXTRA_WARN2` (6.18 and later) to `defaultFlags` in
  `pkg/ebpf/bytecode/runtime/runtime_compilation_helpers.go`, compare against the base commit, and
  revert.
- **Long verifier logs get truncated** before the failing instruction in Go's test output. To see the
  whole log, write `fmt.Sprintf("%+v", verr)` of the `*ebpf.VerifierError` to a file.

## KMT in CI

Outside contributors can't see KMT jobs or logs; ask a maintainer for the results.

- **A green pipeline can mean KMT never ran.** When no path in `.system_probe_change_paths`
  (`.gitlab-ci.yml`) changed, the KMT jobs are manual and allowed to fail. Stacked PRs (not based on
  `main` or a release branch) get no KMT at all.
- **System-probe KMT tests only `TEST_PACKAGES_LIST`** in `tasks/system_probe.py`. A root-only test in
  a package outside that list never runs there.
- **Check which jobs ran before reading a result.** A red pipeline where KMT never started says nothing
  about the change, and a pending status can be a lost status update rather than a running job.
- **A pipeline created through the API** (not by a push) doesn't post `dd-gitlab/default-pipeline`, so
  an older failure stays the PR's latest status until the next push.
- **Read results from the JUnit artifact, not the job log.** Logs can hit GitLab's size limit and tests
  don't run with `-v`.
- **A green job can hide a flake.** KMT reruns failing tests (`RETRY` in `.kmt_run_tests`) and
  tolerates tests in `flakes.yaml`; check the JUnit artifact for retried tests. Report a suspected
  flake as inconclusive, measure its failure rate on the base and your branch locally, and never retry
  until green.
- **Retrying a single KMT job fails once its VMs are gone.** Trigger a new pipeline instead
  (maintainers can use `dda inv kmt.retry-failed-pipeline`).

## Manual runs outside KMT

These apply when running `dda inv system-probe.test` directly on a Linux VM.

- **Stale objects:** tests load objects from `/opt/datadog-agent/embedded/share/system-probe/ebpf`, not
  from the tree. `file content hash does not match expected value` (runtime) or `couldn't find map ...
  unknown section` (CO-RE), with every mode failing in under a second, means rebuild the objects with
  `dda inv system-probe.object-files`.
- **`sudo` can reset the environment,** so check that `TEST_*_OVERRIDE` variables reach the test.
