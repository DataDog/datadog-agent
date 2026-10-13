# No-Internet Host E2E Tests — Findings

**Branch:** `kfairise/no-internet-hosts-e2e`
**PR:** https://github.com/DataDog/datadog-agent/pull/58058 (draft, experimental — do not merge)
**Pipeline:** GitLab #144264658
**Status:** IN PROGRESS — updated as failures arrive

## Context

The E2E framework's AWS host provisioner (`awshost.Provisioner`) now prepends a
`WithoutInternetAccess()` default: the VM's default security groups are replaced with
`no-internet-access` groups. Tests that need internet must explicitly opt back in with
`ec2.WithInternetAccess()`.

This branch removes **every explicit `WithInternetAccess()` opt-in from host-based new-e2e
tests** (67 files, 78 call sites) so that they run against the no-internet default. The goal
is not to make them pass, but to inventory:

1. which tests actually depend on internet access,
2. what exactly each one needs from the network, and
3. what would be required to make each pass without internet access.

## Scope

All `ec2.WithInternetAccess()` / `scenec2.WithInternetAccess()` occurrences under
`test/new-e2e/` (host/EC2-VM-based tests) were removed:

| Suite | Files | Notes |
|---|---|---|
| `tests/agent-configuration` | 18 | windows + linux + aws variants |
| `tests/agent-subcommands` | 22 | windows + linux |
| `tests/agent-runtimes` | 12 | incl. infra-mode, procmgr, rtloader |
| `tests/agent-platform` | 9 | install-script, ddot, upgrade, package-signing, macos |
| `tests/installer` | 3 | unix packages, scripts, fapolicyd |
| `tests/fleet` | 3 | suite.go (all platforms), config_macos |
| `tests/cws`, `tests/cspm`, `tests/discovery` | 6 | |
| `tests/ndm/snmp`, `tests/netpath`, `tests/remote-config`, `tests/apm`, `tests/data-security`, `tests/privateactionrunner`, `tests/agent-log-pipelines` | 8 | |
| `examples/` | 2 | host_nopulumi, vm_win |

**Not in scope (left untouched):** the framework's own defaults and helpers
(`test/e2e-framework/...`), the Kubernetes scenarios (EKS / kind / kubeadm — no explicit
opt-ins existed under `test/new-e2e` anyway), and `ec2.ParamsFromEnvironment`'s
append of `WithInternetAccess()` (config-driven path, not a test).

### Known caveats discovered during the removal

- **`tests/data-security/postgres_provisioner.go`** called `ec2.NewVM(..., ec2.WithInternetAccess())`
  directly — **not** through `awshost.Provisioner`. `NewVM` has **no no-internet default**
  (`withoutInternetAccess` defaults to `false`), so removing the option there is a no-op:
  the VM keeps internet. To truly test it, `WithoutInternetAccess()` would have to be added.
- Several tests had in-code comments explaining the internet dependency, e.g.
  `tests/discovery/linux_test.go`: *“provision.sh installs packages from pip, npm and gem”*.
- The macOS fleet config job (`new-e2e-fleet-config-macos`) is manual/nightly-only on
  branch pipelines; its failures won't show up in this run.

## CI coverage of this run

Pipeline jobs relevant to host tests (305 `new-e2e-*` jobs queued in total, incl. k8s ones
not affected by this change). Host-based jobs affected by the removal:

- linux: `agent-configuration`, `agent-subcommands`, `agent-runtimes`, `agent-platform-install-script-*`,
  `agent-platform-ddot-*`, `package-signing-*`, `cspm`, `cws`, `discovery`, `data-security`,
  `ndm-snmp`, `remote-config`, `apm`, `fleet-config` (linux group), `netpath`
- windows: `*-windows` variants (`agent-configuration`, `agent-subcommands`, `agent-runtimes`,
  `cws-windows`, `netpath-windows`, `privateactionrunner-windows`, `fleet-config` windows groups)
- macOS: `agent-platform-install-script-macos-x64` (queued; macOS fleet config is manual)

## Findings per failure

Failures are grouped by root cause. Pipeline #144280192 was still running when this
snapshot was written; more entries may follow (see “monitoring status” at the end).

> Baseline methodology: for each failing job, the same job was checked on other branches'
> pipelines running today (e.g. #144275341, #144275372, #144276865, #144277884, #144280607,
> #144286162). A failure that reproduces identically on unrelated branches is classified
> **pre-existing**; a failure whose subtests differ from the baseline (or that appears only on
> this branch) is classified **no-internet-caused**.

### Bucket 1 — Datadog *testing* package-repo GPG keys served from the public endpoint `apttesting.datad0g.com` (13 jobs)

The testing apt/yum repos use **S3-backed `baseurl`s** (`s3.amazonaws.com/apttesting.datad0g.com/...`,
`s3.amazonaws.com/yumtesting.datad0g.com/...`) which remain reachable without internet, but the
**signing keys** are fetched from the public CloudFront-backed host `apttesting.datad0g.com/test-keys/...`.
Package installation fails at key retrieval/import.

| Job | Failing test(s) | Key fetch that fails |
|---|---|---|
| `new-e2e-agent-platform-ddot-amazonlinux-a7-x86_64` / `-arm64` | `TestDDOTInstallScript/*/TestDDOTInstall/install_ddot` (all Amazon Linux versions) | `gpgkey=https://apttesting.datad0g.com/test-keys/DATADOG_RPM_KEY_*.public` in the test-written `/etc/yum.repos.d/datadog.repo` (`ddot_install_test.go`, `ddotRhelTest`) |
| `new-e2e-agent-platform-ddot-ubuntu-a7-x86_64` | `TestDDOTInstallScript/*/TestDDOTInstall/create_/usr/share_keyring_and_source_list` | `curl https://apttesting.datad0g.com/test-keys/DATADOG_APT_KEY_CURRENT.public` for the deb keyring |
| `new-e2e-agent-platform-ddot-debian-a7-x86_64` / `-arm64` | same as ubuntu | same |
| `new-e2e-agent-platform-ddot-suse-a7-x86_64` / `-arm64` | same | same |
| `new-e2e-agent-platform-ddot-centos-a7-x86_64` | same (+ centos-vault base repo problem, bucket 3) | same |
| `new-e2e-agent-platform-install-script-centos-a7-x86_64` / `-centos-iot-agent` / `-centos-fips` / `-centos-fips-iot-agent` / `-amazonlinux-a7-x64` / `-suse-a7-x86_64` / `-suse-iot-agent-a7-x86_64` | `TestInstallScript/*/TestInstallAgent` — `Failed to retrieve GPG key for repo 'datadog'` / `Gpg Keys not imported, cannot verify repomd.xml` | install script's testing repo key import (`apttesting.datad0g.com/test-keys/…`) |

**To pass without internet:** publish the APT/RPM test-keys on an S3/internal endpoint
(e.g. `s3.amazonaws.com/apttesting.datad0g.com/test-keys/…` next to the packages) and point
every `gpgkey`/keyring URL (test code *and* install scripts) there. The change is small and
mechanical; alternatively the keys could be vendored onto the VMs via the S3 artifact bucket.

### Bucket 2 — Datadog *production* public key host `keys.datadoghq.com` (1 job)

- **Job:** `new-e2e-package-signing-debian-a7-x86_64`
- **Failing test:** `TestPackageSigningComponent/Test_package_signing_on_debian-x86-64-11_/TestPackageSigning`
- **Root cause:** the test runs
  `curl -o /tmp/DATADOG_APT_KEY_CURRENT.public https://keys.datadoghq.com/DATADOG_APT_KEY_CURRENT.public`
  (`package_signing_test.go`) — `keys.datadoghq.com:443` connection timed out.
- **To pass without internet:** vendor the production signing keys in the repo/test fixtures or
  fetch them from the internal artifacts bucket.

### Bucket 3 — public base-OS mirrors / third-party repos reachable only via internet (7 jobs)

| Job | Failing test | What needs the internet |
|---|---|---|
| `new-e2e-agent-platform-install-script-docker` | `TestInstallScript/test_install_script_on_a_docker_container_(using_SysVInit)/TestInstallAgent` | `apt-get update && apt-get install -y curl sudo` **inside** the throwaway Ubuntu 22.04 container (`install_script_test.go:272`); `archive.ubuntu.com`/`security.ubuntu.com` blocked. Image itself comes from the ECR pull-through cache (OK). Fix: pre-bake curl+sudo in the target image, or use an internal apt mirror |
| `new-e2e-cspm: [TestHostBenchmarksRHEL8/RHEL9/RHEL10]` | suite fails during **provisioning** (pulumi `remote-aws-vm-cmd-dnf-install-...rpm`) | the dnf install of the pipeline agent RPM refreshes **every repo enabled on the e2e AMI**; the pre-configured `docker-ce-stable` repo points at `download.docker.com` → metadata fetch times out. Fix: `--disablerepo=docker-ce-stable` (or `--cacheonly`) for the agent RPM install, or drop/disable that repo from the AMI, or mirror docker-ce internally |
| `new-e2e-cspm: [TestHostBenchmarksAlmaLinux9]` | same provisioning step | AlmaLinux base repo `appstream` mirrorlist (`mirrors.almalinux.org`) is public — no AWS-internal mirror exists for Alma. Fix: internal mirror of Alma base repos in the QA VPC (or pre-bake the agent RPM deps in the AMI) |
| `new-e2e-agent-platform-ddot-centos-a7-x86_64`, `install-script-centos-*` (CentOS 7.9) | package install | CentOS 7 is EOL: base repos live on `archive.kernel.org/centos-vault` (public). Fix: internal mirror of centos-vault, or pre-baked repo state in the AMI |
| `new-e2e-installer-ansible: [TestPackageAgentFapolicyd]` | `TestPackageAgentFapolicyd/TestInstallWithFapolicyd` | the ansible playbook installs **docker** at runtime from `docker-ce-stable` (`download.docker.com`). Fix: same as cspm (pre-baked docker in AMI — docker is already pre-baked in the `*-e2e` AMIs — or internal docker-ce mirror) |

### Bucket 4 — “internal” testing OCI registry alias unreachable from no-internet VMs (4+ jobs)

`installtesting.datad0g.com.internal.dda-testing.com` (the “internal” alias of the testing
package registry) **resolves to public/internet-facing infrastructure**: connections from
no-internet VMs time out (`network timeout`, `Failed to connect … port 443`).

| Job | Failing test(s) | Symptom |
|---|---|---|
| `new-e2e-installer-script` | `TestScripts/*` (all distros) | install script sets `DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE=installtesting.datad0g.com.internal.dda-testing.com`; `Download failed with curl … Failed to connect to installtesting.datad0g.com.internal.dda-testing.com port 443` |
| `new-e2e-fleet-config` (windows groups ×2) | all `TestFleetConfig/windows-server:*` subtests | installer cannot fetch packages: `could not download agent package pipeline-144280192 — installtesting.datad0g.com.internal.dda-testing.com/agent-package:pipeline-144280192: network timeout` |
| `new-e2e-fleet-extensions` (windows ×2) | all `TestFleetExtensions/windows-server:*` subtests | same |
| `new-e2e-fleet-upgrade` (windows ×2) | all `TestFleetUpgrade/windows-server:*` subtests | same + bootstrap hits `https://install.datadoghq.com/v2/` and `https://gcr.io/v2/` (`dial tcp 108.138.64.x:443: i/o timeout` — CloudFront) |

**To pass without internet:** the testing OCI registry needs genuinely VPC-reachable
endpoints (private hosted zone + internal LB / S3-backed). For the Windows fleet bootstrap,
the production endpoints (`install.datadoghq.com`, `gcr.io`) must also be avoided or mirrored.
This is infrastructure work (agent-devx), not test-code work.

### Bucket 5 — tests/agents talking to real Datadog public endpoints (6 jobs)

| Job | Failing test | Endpoint needed | Fix direction |
|---|---|---|---|
| `new-e2e-remote-config` | `TestRcTracerSuite/TestRemoteConfigTracerUpdate` (explicitly “Requires a valid Datadog API key”, `ProvisionerNoFakeIntake`) | agent RC client → `api.datadoghq.com` RC API; expected log lines (`/api/v0.1/configurations`, `roots`, `targets`) never appear; also `couldn't get the repositories state … expired at 1970-01-01` | point remote config at an internal RC backend (fakeintake-style RC mock) |
| `new-e2e-cws: [TestAgentSuiteEC2]` | `Test00RulesetLoadedDefaultFile`, `Test01RulesetLoadedDefaultRC`, `Test02Selftests`, `Test99CWSEnabled` (baseline on other branches only `Test03OpenSignal` fails — pre-existing backend issue) | `GET https://api.datadoghq.com/api/v2/remote_config/products/cws/policy/download` — default CWS ruleset comes from remote config | internal RC route/mock; `Test03OpenSignal` (“attributes” missing in signal) is pre-existing |
| `new-e2e-cws-windows` | same ruleset subtests (`Test00RulesetLoadedDefaultFile`, `Test01RulesetLoadedDefaultRC`, `Test02Selftests`, `Test99CWSEnabled`; baseline only `Test03CreateFileSignal` fails) | same RC policy download | same |
| `new-e2e-agent-subcommands` | `TestLinuxDiagnoseSuite` (all subtests) | diagnose performs live endpoint checks: `Post https://resources-intake.datadoghq.com./api/v2/genresources` times out → “diagnose reported a transient endpoint failure that did not clear on retry” | point the agent at internal intake endpoints (fakeintake) or make diagnose tolerate unreachable endpoints |
| `new-e2e-agent-subcommands` | `TestLinuxFlareSuite/TestFlareDefaultFiles` | flare output includes failing endpoint/connectivity checks (same genresources timeout) | same |
| `new-e2e-netpath-windows` | `TestWindowsNetworkPathIntegrationSuite/TestWindowsNetworkPathIntegrationMetrics` | the monitored destinations in `fixtures/network_path.yaml` are **external**: `api.datadoghq.eu:443` (TCP) and `8.8.8.8` (UDP); without internet no network path is ever collected (“could not find matching netpath”) | monitor internal destinations instead (e.g. the fakeintake LB or an in-VPC service) |

### Bucket 6 — docker images pulled from registries without an internal route (2 jobs)

- **`new-e2e-apm: [TestVMFakeintakeSuiteTCP]` and `[TestVMFakeintakeSuiteUDS]`** — SetupSuite
  fails with `Error response from daemon: Get "https://ghcr.io/v2/"`. The test pulls
  `ghcr.io/datadog/apps-tracegen:<version>` (`tests/apm/vm_test.go:60`) — **GHCR is not
  proxied** by the ECR pull-through cache (which only supports DockerHub / Public ECR / Quay,
  and this is documented as intentional in `test/new-e2e/codereview_guideline.md`).
- **To pass without internet:** mirror `ghcr.io/datadog/apps-tracegen` into an internally
  reachable registry (QA ECR) and update the pull commands.

### Bucket 7 — unclear / needs deeper investigation (3 jobs)

- **`new-e2e-agent-runtimes-windows`** — only `TestAuthArtifactIPCSecurityWindowsSuite/TestServersideIPCCertUsage`
  fails (both attempts): `Stop-Service datadogagent` fails (“cannot be stopped … Cannot stop
  dd-procmgr-service”), error at `auth_artifact_common_test.go:117`. The sibling suite
  `TestIPCSecurityWindowsSuite` passes. Not failing on other branches today. The internet
  dependency (if any) is unclear — the agent may be stuck during shutdown due to unreachable
  DD endpoints, but nothing in the log pins it. **Needs a dedicated Windows debug run.**
- **`new-e2e-agent-subcommands` — `TestLinuxSecretSuite/TestAgentConfigRefresh`** —
  `could not reach agent: Get "https://localhost:5001/agent/secret/refresh": local error: tls: bad record MAC`.
  A localhost TLS error is odd and does not obviously relate to internet access; may be a
  flake amplified by the slow agent. Needs investigation.
- **`new-e2e-agent-configuration`** — failed on another branch (13:38) while ours is still
  running; classification pending.

### Pre-existing failures unrelated to this branch (2 jobs)

- **`new-e2e-cws: [TestAgentSuiteGCP]`** — `Test03OpenSignal` only: signal lacks the
  `attributes` field — same on several unrelated branches today; GCP suite is untouched by
  this diff; job is `allow_failure: true`.
- **`new-e2e-cws: [TestKindSuite]`** — identical subtest failures (`Test00RulesetLoadedDefaultFile`,
  `Test01RulesetLoadedDefaultRC`, `Test02Selftests`, `Test03…`, `Test04…`) on unrelated branches
  (e.g. #144277884) — the Kind suite already runs (with internet) on main and fails the same way.

### Notable: jobs that PASS without internet (so far)

- `new-e2e-privateactionrunner` (✅), `new-e2e-cws: [TestECSFargate]` (✅),
  `new-e2e-data-security` (✅ — note: its `postgres_provisioner.go` builds the VM with
  `ec2.NewVM()` which has **no** no-internet default, so the removal was a no-op there),
  `new-e2e-discovery` (✅ — despite the in-code comment that `provision.sh` installs from
  pip/npm/gem; those either run against pre-baked tooling or are not exercised),
  `TestLinuxFlareSuite/TestFlareProfiling` (✅ within agent-subcommands).

### Coverage gaps of this run (jobs that did not execute on a branch pipeline)

- `new-e2e-agent-configuration` (linux) — **manual** on branch pipelines: the linux
  agent-configuration suites (sgc, secrets, configrefresh, inventory, api, gui) were not
  exercised. Only the `-windows` variant ran.
- macOS host tests — `new-e2e-agent-platform-install-script-macos-x64` and
  `new-e2e-fleet-config-macos` are **skipped/manual** on branch pipelines; the macOS
  fleet-config suite (whose `suite.go` opt-in was removed) is not exercised by this run.

## Summary

(interim — final summary at pipeline completion)
