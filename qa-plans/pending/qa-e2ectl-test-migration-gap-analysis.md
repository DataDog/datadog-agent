# e2ectl migration gap analysis: existing test patterns vs the attach model

> **Category C — pending analysis and evolution plan.** Which existing E2E patterns
> the current e2ectl model covers, which it cannot express today, and the minimal
> framework evolution to make every test migratable — with genuine exceptions
> classified honestly rather than forced. Grounded in source at `424e4364be4`
> (plus uncommitted local `package-core` support). See the
> [plan status index](../qa-e2ectl-plans-index.md).

## 1. The inventory

310 `_test.go` files across 39 test areas. Pattern density measured over the tree:

| Pattern | Files | Share |
|---|---:|---:|
| Mid-suite env mutation (`UpdateEnv`) | 58 | 19% |
| Fakeintake flush/reset between tests | 48 | 15% |
| Raw remote command execution | 127 | 41% |
| Parallel entry points/subtests | 151 | 49% |
| Custom inline Pulumi provisioner | 7 | 2% |
| No-agent initial state (`WithoutAgent`) | 8 | 3% |
| Agent client restart | 5 | 2% |
| **Existing e2ectl attach entry points** | **3** | **1%** |

The three attached tests today (`agent-subcommands/local_test.go`,
`agent-metric-emission`, `containers/local_kind_test.go`) all follow one shape:
attach, assert against a healthy, running, pre-configured agent. Every other
shape in the tree is currently out of reach — not because attach is wrong, but
because the framework's mutation surface is a single static snapshot.

## 2. The taxonomy: what "migratable" means

The user's goal is that all existing tests migrate to the e2ectl approach —
provisioning described by config, tests attaching to live environments. Three
classes emerge:

- **Class A — bodies unchanged, framework seam missing** (the majority):
  provisioning syntax in the test hides a *mutation* semantic
  (config-apply+restart, reset, rewire). The assertion never cared about
  Pulumi; `UpdateEnv` is just today's only mutator.
- **Class B — body unchanged, environment adapter missing** (~a quarter):
  Windows/macOS-pool/ECS-Fargate/GPU/multi-VM. The Pulumi scenarios already
  exist; e2ectl needs the environment type registration and installer
  adapters (MSI, pkg), not an architecture change.
- **Class C — genuine exceptions, and why they are still migratable**:
  Driver Verifier, Fargate runtime identity, GPU hardware presence. The
  *provisioned thing itself* is part of the test subject. These migrate as
  cloud-backed e2ectl environment types with capability declarations — they
  simply can never be local, and refusing to run them locally is correct
  behavior, not a migration failure.

No pattern found in the audit is an architecture blocker for the e2ectl model
itself. Every blocker is either a missing mutator seam (A), a missing adapter
(B), or a capability that must stay cloud-backed (C).

## 3. Pattern-by-pattern analysis

### 3.1 Mid-suite config mutation — the 58-file blocker

**Evidence:** `agent-configuration` (13 files), `agent-subcommands` (16),
`agent-runtimes` (6), `agent-health/resilience_test.go:141,169`,
`agent-configuration/api_test.go:549,597,659`, `configrefresh_nix_test.go:63,131`.

**Today:** `UpdateEnv` swaps provisioners; `reconcileEnv`
(`testing/e2e/suite.go:325-337`) re-applies the agent component. The test only
needs *config-apply + restart* — provisioning syntax hiding a mutation
semantic.

**Blocker:** `StaticStackProvisioner.ProvisionEnv`
(`testing/provisioner/static_stack.go:71-87`) reads only the snapshot file;
`UpdateEnv` on an attached env is a no-op against the live host. Config never
changes, so the unhealthy-then-healthy assertions cannot run.

**Evolution — `e2ectlenv.ApplyAgentConfig`:** a public mutator on the attach
path: apply config through the shared installer
config/restart logic (`testing/installers/host/configure`), persist the changed
agent params via `UpdateSnapshotResource`, and rebuild `env.Agent.Client` so the
next `Init` sees the new state. The configrefresh case
(`agent-configuration/configrefresh_nix_test.go`) proves the mutator must also
rewrite the *client wiring* (ports, auth-token path, `agentclientparams`) in the
snapshot resource — not just the YAML.

**Pilot:** `TestDefaultInstallUnhealthy` (health suite) — the FakeIntake 403
override is already an attach-compatible runtime call; only the `UpdateEnv`
line needs `ApplyAgentConfig`.

### 3.2 Per-test reset — the silent-isolation hazard

**Evidence:** `BaseSuite.BeforeTest` (`testing/e2e/suite.go:631-637`) re-runs
`reconcileEnv(bs.originalProvisioners)`, restoring the suite baseline before
every subtest.

**Blocker:** under attach, re-converge reads a file; host-side drift from the
previous subtest (edited `datadog.yaml`, stopped units, installed packages)
persists. Subtest isolation is *silently lost* — worse than a hard error, and
the single biggest correctness hazard for attach-mode adoption.

**Evolution:** environment-owned reset points — `e2ectl env save/restore`
(rolling back agent config, integrations and runtime state to a named
checkpoint; the owned runtime volume is already the right substrate for
binary) plus `e2ectlenv.ResetEnv(t, name)`. Tests that install and purge
themselves opt out. For the config dimension this is exactly equivalent; for
package state it is not (3.3).

### 3.3 Install/uninstall/upgrade as the test subject

**Evidence:** `installer/script/default_script_test.go:26-71` (`WithoutAgent()`,
asserts package install, file modes, **systemd units running**),
`installer/unix/upgrade_scenario_test.go:104-133` (experiment packages, stable
symlinks, rollback), `fleet/suite/suite.go:112-120` (per-platform `WithoutAgent`),
`agent-platform/tests/upgrade_test.go:75-126`.

**Blocker:** twofold. (a) `e2ectl install` always installs an agent — there is
no attach-with-empty-host entry. (b) The installation itself — dpkg/apt +
postinst + systemd units — *is* the assertion target; the local `package-core`
path deliberately does not attest those semantics, and replacing the test
subject with a foreground core process would be a dishonest equivalent.

**Evolution:** `agent: install: none` / `e2ectlenv.AttachBareHost` (the CLI
already permits infrastructure-only configs at `start`), the installer script
invocation exposed as an action on the attached host, and per-subtest env
reset (3.2) so destructive install/purge sequences stay isolated. Unit and
package-state assertions require a host-class environment (systemd VM);
that is a Class-B adapter, not a local capability.

### 3.4 Service-manager control plane

**Evidence:** `agent-runtimes/procmgr/procmgr_nix_test.go:133-145` (systemctl
stop/restart of ddot and agent units), `agent-platform/common/agent_behaviour.go:186-448`
(`SvcManager.Restart` after config edits).

**Blocker:** the local binary agent runs a foreground process in a container —
no service manager, no `systemctl`. `Client.Restart()` exists but is not the
same event as a unit restart (unit restarts exercise systemd dependencies:
sysprobe/ddot units, journald windows).

**Evolution:** an attach-mode `SvcManager` implementation ("single-process
manager": supervisor kill + restart, config path remap) for the simple cases,
with **declared non-equivalence** in capability metadata. Tests that depend on
unit graphs stay host-class. Mixing an honest weaker capability with automatic
skip (not silent pass) is the pattern here.

### 3.5 Complex topology — multi-agent/multi-intake scenarios

**Evidence:** `ha-agent/haagent_failover_test.go:42-48` (two agents, one
fakeintake, shared RC `config_id`), `agent-runtimes/forwarder_nss_failover_test.go:42-50`
(one agent, two fakeintakes, switchable logical hostname), `agent-health/provisioner.go:22-77`
(inline Pulumi with a compose `DependsOn` ordering constraint).

**Blocker:** these map directly onto the pending
[custom-environments plan](qa-e2ectl-custom-environments-plan.md) — scenario
params + scenario-exposed installer. Two concrete gaps stand between the plan
and the code: (a) the attach/publish helpers (`attachHostForInstall`,
`writeAgentToSnapshot` in `cmd/e2ectl/internal/installer/routing.go:205`,
`installer.go:332`) are **unexported package internals** — scenario code
outside `cmd/` cannot install or publish today; (b) `workloads.Deploy` supports
only kind/local (`internal/workloads/workloads.go:26-46`), so EC2 docker-compose
apps cannot be expressed.

**Evolution:** export the attach/publish path as a public component-level
seam (`installscript.InstallOnHost(host, fakeIntake, params)` beside the
existing whole-env `Install`), and make the `DependsOn(compose)` ordering a
scenario-installer responsibility (deploy the workload before installing the
agent). Test bodies (election, failover, restart sequencing) stay unchanged as
Go action tools per the plan.

### 3.6 Receiver-side reset — already compatible

**Evidence:** 48 files call `FlushServerAndResetAggregators()` in `BeforeTest`
(all 11 npm tests, process, otel, netpath…).

**Coverage:** this pattern attaches cleanly today — the fakeintake client is
part of the snapshot and the flush is a runtime HTTP call. No framework change
needed; it should be documented as the canonical per-test isolation mechanism
for capture assertions, complementary to (and usually cheaper than) env reset.

### 3.7 Platform adapters — Windows, macOS, ECS, GPU

**Evidence and gaps** (from the platform audit, all Class B except the noted
Class C cores):

| Platform | Representative | Missing |
|---|---|---|
| Windows | `windows/install-test`, `fips-test` (MSI, registry, code signing) | Windows host env type, `agent.msi` installer section, registry capability on RemoteHost |
| Windows rollback | `installer/windows/installer_rollback_test.go` (WiX deferred custom actions) | MSI adapter; semantics stay cloud — local-infeasible |
| Driver Verifier | `windows/service-test/startstop_test.go:1124-1190` | Capability flag; genuine Class C — real Windows kernel required |
| Multi-VM Windows | `windows/windows-certificate` (Defender disabled, self-signed cert) | Multi-VM env type with per-VM roles |
| macOS | `agent-platform/tests/macos_install_test.go` + pool machinery `testing/e2e/suite.go:803-895` | macOS pool env type with register/reconcile/release lifecycle hooks (the `PoolLeaseToken` write pattern is the contract to port) |
| ECS/Fargate | `cws/fargate_test.go:51` | ECS env type; Fargate runtime-identity assertions are genuine Class C — managed runtime is part of the subject |
| Kernel-pinned eBPF | `npm/ec2_1host_selinux_test.go:24` (pinned AMI), tcp_congestion (netem) | AMI/kernel pinning fields + capability vector (selinux, ebpfPrivileged, netem) |
| GPU | `gpu/provisioner.go` (g4dn, CUDA matrix) | GPU env type + capability flags; hardware stays cloud |

The cross-cutting contract for all of these: an e2ectl **environment capability
vector** (OS, kernel version, systemd, package-manager, GPU, verifier…) that
tests assert against — so `e2ectl test` skips non-equivalent subtests *visibly*
instead of silently passing weaker checks, and environment selection can match
tests to capable environments.

## 4. What is genuinely impossible locally — and why that is fine

Three families where provisioning/hardware is part of the test subject:

1. **Driver Verifier** — kernel verifier policy on Windows services.
2. **Fargate runtime identity** — ECS task metadata/awsvpc is what produces the
   asserted behavior.
3. **GPU/hardware presence** — physical NVML/driver/CUDA matrix.

These migrate to e2ectl *as cloud-backed environment types with capability
declarations*. The framework's answer to "not covered locally" must be an
explicit, visible incompatibility — never a silent weaker substitute. The
capability vector in §3.7 is the mechanism.

## 5. Evolution roadmap (assertions unchanged everywhere)

| Step | Seam | Unblocks | Effort |
|---|---|---|---|
| 1 | `e2ectlenv.ApplyAgentConfig(t, envName, agentparams...)` — config apply + restart + snapshot/client update, incl. `agentclientparams` rewiring | §3.1 (58 UpdateEnv files) | medium |
| 2 | `e2ectl env save/restore` + `e2ectlenv.ResetEnv` + opt-out for self-cleaning tests | §3.2, §3.3 isolation | medium |
| 3 | `agent: install: none` + `e2ectlenv.AttachBareHost` + installer action on attached host | §3.3 install-as-subject | small |
| 4 | Attach-mode `SvcManager` with declared non-equivalence; capability-keyed skip in `e2ectl test` | §3.4, feeds §3.7 | small-medium |
| 5 | Public component-level install/publish seam (`InstallOnHost`) + workload deployment for EC2 hosts | §3.5 (custom-environments plan Phase 1) | medium |
| 6 | Environment capability vector + visible incompatibility | §3.7 platform adapters, all Class C | medium |
| 7 | Platform adapters: Windows host + MSI, macOS pool lifecycle hooks, ECS, multi-VM, GPU/pinning | §3.7 | large, incremental per platform |

Steps 1-4 are the core mutability layer: they convert the static snapshot into
a lifecycle the suite drives. Step 5 lands the custom-environments plan's
prerequisite. Steps 6-7 are adapter work that proceeds platform-by-platform.

The 48-file fakeintake-flush pattern (§3.6) needs documentation only.

## 6. Coverage verdict

| Test population | Status under current e2ectl |
|---|---|
| Static, healthy-agent suites (apm, otel, discovery, ndm, sbom, usm, most containers) | **Attachable today** — need only entry points; assertions unchanged |
| Config-mutating suites (agent-configuration, agent-subcommands, resilience…) | Blocked on Step 1 — the mutator |
| Destructive/installer suites | Blocked on Steps 2-3 — reset + bare host |
| Topology suites (ha-agent, NSS, multi-VM) | Blocked on Step 5 + the custom-environments plan |
| Windows/macOS/ECS/GPU suites | Blocked on Step 7 adapters — cloud-backed by design |
| Driver Verifier / Fargate identity / GPU presence | Class C — cloud-backed e2ectl only, correct to refuse locally |

The audit found **no pattern that invalidates the e2ectl model**. The one
systemic finding is that attach today is a *static* one-shot: making the
environment lifecycle-driven (mutate, checkpoint, restore, reset) is what turns
"most tests" into "all tests" — with the Class C remainder honestly declaring
their cloud-only nature.
