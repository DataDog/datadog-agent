# No-Internet Host E2E Tests — Findings

**Branch:** `kfairise/no-internet-hosts-e2e`
**PR:** https://github.com/DataDog/datadog-agent/pull/58058 (draft, experimental — do not merge)
**Pipeline:** GitLab #144280192 — **FAILED** (terminal), 61 failed jobs out of 884
**Author's note:** experimental probe branch; failures are expected and were not fixed.

## Context

The E2E framework's AWS host provisioner (`awshost.Provisioner`) now prepends a
`WithoutInternetAccess()` default: the VM's default security groups are replaced with
`no-internet-access` groups. Tests that need internet must explicitly opt back in with
`ec2.WithInternetAccess()`.

This branch removes **every explicit `WithInternetAccess()` opt-in from host-based new-e2e
tests** (67 files, 78 call sites) so that they run against the no-internet default. The goal
was to inventory:

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

### Caveats discovered during the removal

- **`tests/data-security/postgres_provisioner.go`** called `ec2.NewVM(..., ec2.WithInternetAccess())`
  directly — **not** through `awshost.Provisioner`. `NewVM` has **no no-internet default**
  (`withoutInternetAccess` defaults to `false`), so removing the option there was a no-op:
  the VM keeps internet. To truly test it, `WithoutInternetAccess()` would have to be added.
- Several tests had in-code comments explaining the internet dependency, e.g.
  `tests/discovery/linux_test.go`: *"provision.sh installs packages from pip, npm and gem"*.
- Two code issues were found and fixed on the branch before results were meaningful (they
  are part of the branch, not of the investigation): a now-unused `scenec2` import
  (`run_nix_test.go`, caught by `go_e2e_test_binaries` + flavor lints) and two no-op
  `vmOpts = append(vmOpts)` statements flagged by govet (`install_script_test.go`,
  `ddot_install_test.go`).

## Method

- Draft PR → GitLab branch pipeline (`RUN_E2E_TESTS=auto`): jobs triggered by the changed
  test paths, windows included (touching `agent-runtimes`/`fleet` paths flips
  `SKIP_WINDOWS=false`).
- Every failed job's log was collected automatically and triaged; failing tests and network
  error signatures were extracted.
- **Baseline:** each failing job was compared against the same job on unrelated branches'
  pipelines that ran the same day (e.g. #144275341, #144275372, #144276865, #144277884,
  #144280607, #144286162). Identical failures on other branches ⇒ **pre-existing**;
  new/extra subtests only on this branch ⇒ **no-internet-caused**.

## CI coverage of this run

- **Not run on branch pipelines (coverage gaps):** `new-e2e-agent-configuration` (linux, manual
  — the linux sgc/secrets/configrefresh/inventory/api/gui suites were not exercised; only the
  windows variant ran), `new-e2e-agent-platform-install-script-macos-x64` (skipped),
  `new-e2e-fleet-config-macos` (skipped/manual). 129 new-e2e jobs were manual on this pipeline.
- **Passed without internet:** `new-e2e-privateactionrunner` (+ windows), `new-e2e-cws: [TestECSFargate]`,
  `new-e2e-data-security` (note: no-op removal, see caveat above), `new-e2e-discovery`,
  `new-e2e-ndm-snmp`, `new-e2e-netpath` (linux — the Windows variant failed, bucket 5),
  `new-e2e-apm: [TestDockerFakeintakeSuiteTCP/UDS]` (the docker variants pass — their images
  come from the ECR pull-through cache, unlike the VM variants that pull from ghcr.io), and
  `TestLinuxFlareSuite/TestFlareProfiling`.
- **Did not run on this pipeline (manual/skipped):** `new-e2e-agent-health`, `new-e2e-amp`,
  `new-e2e-amp-adp`, `new-e2e-ha-agent`, `new-e2e-ha-agent-failover`, `new-e2e-agent-telemetry`,
  `new-e2e-agent-configuration` (linux), `new-e2e-fleet-config-macos`,
  `new-e2e-agent-platform-install-script-macos-x64` — 129 new-e2e jobs were manual in total.

## Results — 61 failed jobs, classified

| Class | Jobs |
|---|---|
| **No-internet-caused** | **54** |
| Pre-existing (also fail with internet today) | 2 (`new-e2e-cws: [TestAgentSuiteGCP]`, `new-e2e-cws: [TestKindSuite]`) |
| CI/infra noise, unrelated to this branch | 5 (4× `kmt_*_cleanup_*_manual`, `upload_secagent_tests_arm64` — "Instance NOT found") |

The 54 no-internet failures group into **7 root-cause buckets** below. (Jobs can appear in
several buckets when several dependencies are blocked; the *first* blocking one is usually
the reported cause.)

### Bucket 1 — Datadog *testing*-repo GPG keys served from the public host `apttesting.datad0g.com` — 31 jobs

The testing apt/yum repos themselves use **S3-backed `baseurl`s**
(`s3.amazonaws.com/apttesting.datad0g.com/...`, `s3.amazonaws.com/yumtesting.datad0g.com/...`)
which stay reachable without internet — but the **signing keys** are fetched from the
CloudFront-backed public host `apttesting.datad0g.com/test-keys/...`. Every package
installation from a testing repo fails at key retrieval/import. This is the single biggest
blocker of the run.

| Job family | Failing tests | Mechanism |
|---|---|---|
| `new-e2e-agent-platform-ddot-{amazonlinux×2, ubuntu×2, debian×2, suse×2, centos}` (9 jobs) | `TestDDOTInstallScript/*/TestDDOTInstall` | RPM `gpgkey=` URLs / `curl …/test-keys/DATADOG_(APT\|RPM)_KEY_CURRENT.public` for the deb keyring, written by `ddot_install_test.go` |
| `new-e2e-agent-platform-install-script-{centos, centos-iot, centos-fips, centos-fips-iot, amazonlinux, suse, suse-iot, ubuntu, debian, debian-iot}` (10 jobs) | `TestInstallScript/*/TestInstallAgent` | testing install script's key import: `Failed to retrieve GPG key for repo 'datadog'`, `Gpg Keys not imported, cannot verify repomd.xml`, `Failed to download one or more APT GPG keys` |
| `new-e2e-installer: [TestPackages/{agent_, ddot_, apm_inject_, upgrade_scenario_}]` + `[TestPackageAgentFapolicyd]` (5 jobs) | all `TestPackages/*` subtests | same key fetch from the installer bootstrap |
| `new-e2e-installer-ansible: [TestPackages/{agent_, apm_inject_, upgrade_scenario_}]` + `[TestPackageAgentFapolicyd]` (4 jobs) | all | same |
| `new-e2e-installer-script` (1 job) | `TestScripts/*` (all distros) | `Download failed with curl … Failed to connect to installtesting.datad0g.com.internal.dda-testing.com port 443` (also bucket 4) |
| `new-e2e-fleet-config: [linux]`, `new-e2e-fleet-extensions: [linux]` (2 jobs) | all platform subtests | `TESTING_KEYS_URL=apttesting.datad0g.com/test-keys` (also buckets 4/5) |

**To pass without internet:** publish the APT/RPM test-keys on an S3/internal endpoint
(e.g. `s3.amazonaws.com/apttesting.datad0g.com/test-keys/…` next to the packages) and point
every `gpgkey`/keyring/`TESTING_KEYS_URL` reference (test code *and* the testing install
scripts) there. Mechanical, test-side change — the single highest-leverage fix of this run.

### Bucket 2 — Datadog *production* public key host `keys.datadoghq.com` — 2 jobs

- **Jobs:** `new-e2e-package-signing-debian-a7-x86_64`, `-suse-a7-x86_64`
- **Failing test:** `TestPackageSigningComponent/*/TestPackageSigning`
- **Root cause:** `curl https://keys.datadoghq.com/DATADOG_APT_KEY_CURRENT.public` /
  `DATADOG_RPM_KEY_E09422B3.public` (`package_signing_test.go`) — connection timed out.
- **To pass:** vendor the production signing keys in the test fixtures or fetch them from the
  internal artifacts bucket (they are static public keys).

### Bucket 3 — public base-OS mirrors / third-party apt-yum repos — 6+ jobs

| Job | Failing point | What needs internet |
|---|---|---|
| `new-e2e-agent-platform-install-script-docker` | `TestInstallScript/docker_(using_SysVInit)/TestInstallAgent` | `apt-get update && apt-get install -y curl sudo` **inside** the throwaway Ubuntu 22.04 container (`install_script_test.go:272`); `archive.ubuntu.com`/`security.ubuntu.com` blocked. The container image itself comes from the ECR pull-through cache (OK) |
| `new-e2e-cspm: [TestHostBenchmarksRHEL8/RHEL9/RHEL10]` (3 jobs) | suite fails during **provisioning** (pulumi `remote-aws-vm-cmd-dnf-install-<agent>.rpm`) | dnf install of the pipeline agent RPM refreshes **every repo enabled on the e2e AMI**; the pre-configured `docker-ce-stable` repo (`download.docker.com`) times out |
| `new-e2e-cspm: [TestHostBenchmarksAlmaLinux9]` | same provisioning step | AlmaLinux base repo `appstream` mirrorlist (`mirrors.almalinux.org`) is public — no AWS-internal mirror exists for Alma |
| `new-e2e-agent-platform-install-script-ubuntu/debian/debian-iot` (3 jobs) | agent install | Ubuntu/Debian base mirrors (`us-east-1.ec2.archive.ubuntu.com`, `security.ubuntu.com`, `deb.debian.org`) **and** `docker-ce` repo on the e2e AMI are all public |
| `new-e2e-agent-platform-ddot-centos-a7-x86_64` (+centos install-script jobs) | package install | CentOS 7 is EOL — base repos live on `archive.kernel.org/centos-vault` (public) |
| `new-e2e-installer-ansible: [TestPackageAgentFapolicyd]` (+ `new-e2e-installer` variant) | `TestInstallWithFapolicyd` | ansible playbook installs docker at runtime from `docker-ce-stable` |

**To pass:** internal mirrors of base-OS repos + docker-ce (or pre-baked repo state) in the QA
VPC; disable/avoid the `docker-ce-stable` repo during agent RPM installs
(`--disablerepo=docker-ce-stable`); pre-bake `curl`/`sudo` in install-target container images.

### Bucket 4 — “internal” testing OCI registry alias unreachable from no-internet VMs — 9 jobs

`installtesting.datad0g.com.internal.dda-testing.com` (the "internal" alias of the testing
package registry) **still routes via internet-facing infrastructure**: connections time out
(`network timeout`, `Failed to connect … port 443`). The installer cannot pull agent packages.

| Job family | Failing tests | Symptom |
|---|---|---|
| `new-e2e-fleet-config: [windows ×2 groups]`, `new-e2e-fleet-extensions: [windows ×2]`, `new-e2e-fleet-upgrade: [windows ×2]` | all windows-server 2016/2019/2022 subtests | `could not download agent package pipeline-144280192 — installtesting.datad0g.com.internal.dda-testing.com/agent-package:pipeline-144280192: network timeout` |
| `new-e2e-fleet-config/extensions/upgrade: [linux]` (3 jobs) | all platform subtests | same registry + `TESTING_KEYS_URL` (bucket 1) |
| `new-e2e-installer-script` | `TestScripts/*` | same registry (`DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE=…internal.dda-testing.com`) |

**To pass:** host the testing OCI registry on genuinely VPC-reachable infrastructure
(private DNS + internal LB / S3-backed). This is infrastructure work (agent-devx), not
test-code work.

### Bucket 5 — tests/agents talking to real Datadog public endpoints — 7 jobs

| Job | Failing tests | Endpoint needed | Fix direction |
|---|---|---|---|
| `new-e2e-remote-config` | `TestRcTracerSuite/TestRemoteConfigTracerUpdate` (test doc: "Requires a valid Datadog API key") | agent RC client → real RC API (`/api/v0.1/configurations`, TUF roots/targets never retrieved) | point remote config at an internal RC backend/mock |
| `new-e2e-cws: [TestAgentSuiteEC2]`, `new-e2e-cws-windows` | `Test00RulesetLoadedDefaultFile`, `Test01RulesetLoadedDefaultRC`, `Test02Selftests`, `Test99CWSEnabled` (baseline branches only fail `Test03OpenSignal`/`Test03CreateFileSignal` — a **pre-existing** backend `attributes` issue) | `GET https://api.datadoghq.com/api/v2/remote_config/products/cws/policy/download` — the default CWS ruleset comes from remote config | internal RC route/mock |
| `new-e2e-agent-subcommands` (+ `-windows`) | `TestLinuxDiagnoseSuite`/`TestWindowsDiagnoseSuite` (all subtests), `TestLinuxFlareSuite/TestFlareDefaultFiles` | diagnose performs live endpoint checks: `POST https://resources-intake.datadoghq.com./api/v2/genresources` times out → "diagnose reported a transient endpoint failure that did not clear on retry" | point the agent at internal intake endpoints (fakeintake) for diagnose, or tolerate unreachable endpoints |
| `new-e2e-fleet-upgrade: [linux]` | `TestFleetUpgrade$`/`TestFleetIntegrationPreservation$` | **production** yum repo `https://yum.datadoghq.com/stable/7/...` (the upgrade test deliberately pulls production packages) + `instrumentation-telemetry-intake.datadoghq.com` | internal mirror of production package repos for QA, or vendor the "previous release" packages |
| `new-e2e-netpath-windows` | `TestWindowsNetworkPathIntegrationSuite/TestWindowsNetworkPathIntegrationMetrics` | monitored destinations in `fixtures/network_path.yaml` are **external**: `api.datadoghq.eu:443` (TCP) and `8.8.8.8` (UDP); no path is ever collected | monitor internal destinations (e.g. the fakeintake LB or an in-VPC service) |
| `new-e2e-fleet windows` (all) | — | installer bootstrap also hits `https://install.datadoghq.com/v2/` and `https://gcr.io/v2/` (`dial tcp 108.138.64.x:443 i/o timeout` — CloudFront) | production bootstrap endpoints must be avoidable/mirrored for QA |

### Bucket 6 — docker images from registries without an internal route — 2 jobs

- **`new-e2e-apm: [TestVMFakeintakeSuiteTCP]` / `[TestVMFakeintakeSuiteUDS]`** — SetupSuite
  fails: `Error response from daemon: Get "https://ghcr.io/v2/"`. The tests pull
  `ghcr.io/datadog/apps-tracegen:<version>` (`tests/apm/vm_test.go:60`). **GHCR is not
  proxied** by the ECR pull-through cache (which supports only DockerHub / Public ECR / Quay,
  per `test/new-e2e/codereview_guideline.md`).
- **To pass:** mirror `ghcr.io/datadog/apps-tracegen` into a QA-reachable registry and update
  the pull commands.

### Bucket 7 — unclear / needs deeper investigation — 2 jobs

- **`new-e2e-agent-runtimes-windows`** — only `TestAuthArtifactIPCSecurityWindowsSuite/TestServersideIPCCertUsage`
  fails (both attempts): `Stop-Service datadogagent` fails ("cannot be stopped … Cannot stop
  dd-procmgr-service"), error at `auth_artifact_common_test.go:117`. The sibling suite
  `TestIPCSecurityWindowsSuite` passes; no other branch failed this job today. The internet
  dependency (if any) is unclear — the agent may be stuck during shutdown on unreachable DD
  endpoints. **Needs a dedicated Windows debug run.**
- **`new-e2e-agent-subcommands` — `TestLinuxSecretSuite/TestAgentConfigRefresh`** (and the
  Windows secret suite counterpart) — `could not reach agent: Get "https://localhost:5001/agent/secret/refresh":
  local error: tls: bad record MAC`. A localhost TLS error does not obviously relate to
  internet access; may be a flake amplified by a slow agent. Needs investigation.

## Follow-up: fixes applied on this branch (2026-10-13)

After the triage above, the **bucket 1 / bucket 2 failures that are fixable in test code**
were fixed on this same branch, following the pattern already used by
`test/e2e-framework/components/datadog/agent/host_linuxos.go` (S3-backed URLs for the
Datadog testing infrastructure):

1. **`apttesting.datad0g.com/test-keys` → `s3.amazonaws.com/apttesting.datad0g.com/test-keys`**
   everywhere (the S3 bucket serves the same files — verified):
   - `TESTING_KEYS_URL` values: `tests/fleet/agent/install.go`,
     `tests/installer/unix/package_definitions.go`, `tests/installer/script/default_script_test.go`,
     `tests/agent-platform/tests/install_script_test.go`, `upgrade_test.go`, `persisting_integrations_test.go`
   - hardcoded `gpgkey`/`curl` URLs: `ddot_install_test.go`, `step_by_step_test.go`
   - `rpm_test.go` also pointed its `gpgkey` at `keys.datadoghq.com` (production keys) while
     using the S3 yumtesting pipeline repo — the pipeline RPMs are signed with the **test**
     `DATADOG_RPM_KEY_CURRENT` (verified: fingerprint `83661EE4…`), so it now uses the S3
     test-keys like its sibling tests.
2. **`tests/agent-platform/tests/package_signing_test.go`**: ~~keys vendored~~ **reverted** —
   the production signing keys stay fetched at runtime from `keys.datadoghq.com` (which has
   no S3 equivalent), and the suite keeps its explicit `WithInternetAccess()` opt-in. (The
   vendored-keys approach was tried and validated green — 2/2 jobs — but was reverted to keep
   the repo free of vendored public keys.)
3. **`test/e2e-framework/components/datadog/updater/install_script.sh`**:
   `TESTING_KEYS_URL`/`TESTING_APT_URL`/`TESTING_YUM_URL` switched from the public hosts
   (`keys.datadoghq.com`, `apttesting.datad0g.com`, `yumtesting.datad0g.com`) to their
   S3 equivalents, consistent with the rest of the framework.

**Not fixable with S3 URLs** (still open, from the buckets above): the testing OCI registry
`installtesting.datad0g.com.internal.dda-testing.com` (infra), public base-OS/docker mirrors,
remote-config/CWS/diagnose real-backend endpoints, netpath external destinations, the
production `yum.datadoghq.com` repo used by fleet-upgrade, and `ghcr.io` for the apm
tracegen image.

### Fix validation (pipeline #144433353, commit 066da616dac — FAILED, 48 failed / 530 success)

The keys fix was validated by a full pipeline. **The `apttesting` test-keys blocker is
eliminated** — every previously key-blocked job now either passes or fails on a later,
distinct dependency:

| Job family | Before | After |
|---|---|---|
| `ddot` (9 distros) | 0/9 pass | **8/9 pass** — only centos-7 still fails (EOL `archive.kernel.org/centos-vault` base repos) |
| `package-signing` (debian, suse) | 0/2 | **2/2 pass** (vendored keys) |
| `install-script` iot/fips-iot variants (centos-iot, centos-fips-iot, debian-iot, suse-iot) | all failed | **all pass** |
| `installer: [TestPackageAgentFapolicyd]`, all `installer-windows` jobs | failed / n-a | **pass** |
| `install-script` (amazonlinux, centos, centos-fips, debian, ubuntu, suse, docker) | failed at key import | still fail, but now at the **integration wheels download**: `agent-integration-wheels.datadoghq.com` / `dd-integrations-core-wheels-build-stable.datadoghq.com` (public production wheels hosts) — a newly-revealed dependency; plus per-distro base mirrors (centos-vault, ubuntu/debian mirrors + docker-ce) |
| `installer: [TestPackages/*]` + ansible variants, `installer-script`, `fleet-*` (linux + windows) | keys + registry | still fail — **internal OCI registry** `installtesting.datad0g.com.internal.dda-testing.com` (infra work, not test-fixable) |

New findings revealed by this run (the framework-script change made extra jobs auto-run):

- **`new-e2e-agent-configuration` (linux) + `-windows`**: `TestSGC*/TestPullAWSSecret` fails —
  `secret_backend_type: aws.secrets` needs the **public Secrets Manager endpoint**
  (`secretsmanager.us-east-1.amazonaws.com`), unreachable without internet: the agent can't
  decrypt `api_key`, so it never becomes ready. `TestApiSuite` (no fakeintake) also fails
  (agent never ready — needs investigation). Fix: VPC interface endpoints for Secrets
  Manager (or a non-AWS secret backend in the test).
- `new-e2e-ssi-openshift`, `new-e2e-containers-openshift`, `new-e2e-ssi-gke-autopilot`:
  k8s jobs that auto-ran because `test/e2e-framework/**` changed; they fail during **GCP
  provisioning** (unrelated to the script content, these jobs rarely run on branch pipelines).

**Open dependencies (not test-fixable with S3 URLs):**
1. internal OCI registry for testing packages (fleet, installer TestPackages, installer-script) — infra
2. public integrations-wheels hosts (install-script suites) — needs an internal wheels mirror or pre-baked wheels in AMIs
3. base-OS mirrors (centos-vault, ubuntu/debian mirrors) + docker-ce repo on e2e AMIs — internal mirrors / pre-baked AMIs
4. real Datadog endpoints (RC/CWS policy, diagnose, remote-config, fleet-upgrade prod repo) + AWS Secrets Manager endpoint
5. ghcr.io (apm tracegen image), netpath external destinations

## Branch state after triage: no-internet where proven, opt-in elsewhere

Following the triage above, the branch was adjusted so that **internet access is only
removed where it is proven to work without it** (validated on pipeline #144433353), and
**every suite that still needs internet (or has no run data) keeps its explicit
`WithInternetAccess()` opt-in** — restoring main's behavior for those suites while the
remaining dependencies (see list at the end) are addressed.

**Now running without internet (opt-in removed, verified green):**
- `agent-configuration`: configrefresh (linux/windows/api-key-refresh), gui (windows),
  inventory, secrets (linux/windows), sgc multi-backend (embedded)
- `agent-subcommands`: check, config, configcheck, dogstatsd-adpguard/replay, health,
  hostname-ec2, run, status (linux+windows), flare (windows)
- `agent-runtimes`: infra-mode (basic/eudm/none), procmgr (+par), ipc-security (windows),
  rtloader multiprocessing, shared library, auth artifact (linux)
- `agent-platform`: ddot on all distros except CentOS 7 (conditional opt-in), install-script
  on the iot-agent flavors (conditional opt-in)
  *(package-signing was validated no-internet with vendored keys, then reverted to keep the
  internet opt-in — see the follow-up section)*
- `discovery`, `ndm-snmp`, `netpath` (remote-config windows suite), `privateactionrunner`,
  `data-security` (postgres, removal was a no-op anyway)

**Kept with explicit `WithInternetAccess()` (still failing without internet, or not run
in the triage pipelines):** agent-configuration (api, gui linux, sgc aws secrets-manager
suites), agent-subcommands (diagnose/flare linux, diagnose/secret windows), cws (ec2,
windows — remote-config policy download), cspm host benchmarks, netpath integration
(windows), remote-config tracer, apm VM (ghcr.io), agent-runtimes auth artifact (windows,
unclear), all fleet suites (testing OCI registry), installer package suites (registry),
install-script full-agent flavors (integration wheels) + docker variant, ddot centos-7
(vault repos), and the suites not exercised by a branch pipeline (upgrade, step-by-step,
persisting-integrations, rpm, macos install, fleet macos config, file-tailing, examples).

All the S3 URL fixes from the previous section are kept (they work with or without
internet). The branch is rebased on main and complies with the ACIX-2067
`e2e-internet-access` allowlist (`test/new-e2e/internet-access.yaml`): the 39 entries
matching removed opt-ins were dropped (83 → 44), and every remaining opt-in stays listed.
`dda inv --dep tree-sitter==0.26.0 --dep tree-sitter-go==0.25.0 linter.e2e-internet-access`
passes.

## Summary

Removing the explicit `WithInternetAccess()` opt-ins from host E2E tests breaks **54 of the
jobs that run on a branch pipeline**, for essentially **six real dependencies on the public
internet** (plus 2 unexplained), in order of impact:

1. **`apttesting.datad0g.com` test-keys (31 jobs)** — keys must move to S3/internal. Small,
   mechanical, unblocks ddot/install-script/installer/fleet-linux suites.
2. **"Internal" testing OCI registry is internet-facing (9+ jobs)** — needs genuinely
   VPC-reachable endpoints; blocks all fleet/installer suites. Infra work.
3. **Public base-OS/docker mirrors (cspm, install-script deb/ubuntu, fapolicyd)** — needs
   internal mirrors of Ubuntu/Debian/RHEL-extra/Alma/centos-vault/docker-ce, or pre-baked
   repo state/AMIs, plus `--disablerepo` hygiene during agent installs.
4. **Real Datadog endpoints used by tests** (remote-config, CWS ruleset via RC policy,
   diagnose/flare connectivity checks, netpath destinations, production yum repo for
   upgrades) — needs internal RC/intake mocks or internal mirrors, and test destinations
   moved in-VPC.
5. **`keys.datadoghq.com` (package-signing)** — vendor the static keys.
6. **`ghcr.io` (apm tracegen image)** — mirror to a QA registry.

Not covered by this run: the linux `agent-configuration` suites and the macOS host tests
(manual/skipped on branch pipelines) — they should be spot-checked separately (e.g. via the
`no-internet-kubernetes`-style follow-up or a nightly run).
