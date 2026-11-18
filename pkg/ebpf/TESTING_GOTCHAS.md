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

## Local KMT VMs on macOS

Verified on Apple Silicon with Homebrew QEMU 11 and libvirt 12. The first `kmt.launch-stack` took
about 17 minutes, mostly Bazel building the Pulumi program, and later ones under a minute. Once the
test binaries were built, `kmt.test` of one `pkg/ebpf` test took about 90 seconds.

- **`kmt.selfcheck` reports the macOS services as failed without cached `sudo`:** its checks run
  `sudo -n`. Run `sudo -v` first, or check directly (`nfsd status`, `sysctl net.inet.ip.forwarding`,
  `launchctl print system/com.apple.bootpd`).
- **`kmt.gen-config` prompts for confirmation;** pass `--yes` in a non-interactive shell. A run that
  stopped at the prompt has already created the stack, so rerun with `--no-init-stack`.
- **`no resource plugin 'pulumi-resource-command' found`** from `kmt.launch-stack` means the Pulumi
  plugins are older than the repo pins; `kmt.selfcheck` passes as long as any are installed. Install
  the pinned set with `bazel run @pulumi_plugins//:install -- --destdir=$HOME/.pulumi/plugins`.
- **`HVF does not support GICv2 emulation`** from `kmt.launch-stack`: recent QEMU on Apple Silicon
  needs GICv3, and the arm64 distro template
  (`test/e2e-framework/scenarios/aws/microVMs/microvms/resources/distro/domain-arm64.xsl`) doesn't
  set it, so libvirt defaults to GICv2. Adding `<gic version='3' />` inside the `features` element
  fixes it. A failed launch leaves the domain defined; `virsh undefine` it before retrying.
- **The compiler container keeps the checkout it was started from.** It's named per architecture and
  reused, so after switching clones run `kmt.start-compiler` from the new one, or `kmt.test` builds
  the old tree.
- **Don't clone with `--reference` or `--shared`:** the container can't see the alternate object
  directory, so `go build` fails with `error obtaining VCS status`. Use a plain clone, or run
  `git repack -a -d` and delete `.git/objects/info/alternates`.
- **The first `kmt.test` in a fresh clone runs `sudo mkdir`** for `kmt-deps/<arch>/tools`, because
  `setup_runtime_clang` treats a missing directory as unwritable. Create it first with
  `mkdir -p kmt-deps/arm64/tools`.
- **`too many open files in system`** while the compiler unpacks Go toolchains means the Docker VM's
  shared filesystem holds a host file descriptor per cached file and hit macOS's per-process limit.
  Drop the Docker VM's page cache, or restart it, and rerun.
- **VM networking worked without Internet Sharing:** the VM was reachable from the host on the KMT
  bridge as soon as the stack was up.

## Manual runs outside KMT

These apply when running `dda inv system-probe.test` directly on a Linux VM.

- **Stale objects:** tests load objects from `/opt/datadog-agent/embedded/share/system-probe/ebpf`, not
  from the tree. `file content hash does not match expected value` (runtime) or `couldn't find map ...
  unknown section` (CO-RE), with every mode failing in under a second, means rebuild the objects with
  `dda inv system-probe.object-files`.
- **`sudo` can reset the environment,** so check that `TEST_*_OVERRIDE` variables reach the test.
