# Receiver and artifact implementation status

This records the implemented subset of the
[receiver design](../pending/qa-e2ectl-receiver-wiring-plan.md) and
[build blueprint](../pending/qa-e2ectl-agent-build-code-plan.md). Those documents
remain broader roadmaps; they are not a claim that every planned capability ships.

## What is implemented

- Explicit, typed receiver registrations: **fakeintake**, **Datadog**, and an
  externally managed **blackhole**. The blackhole has a runnable stateless HTTP
  server with bounded streaming discard and protocol-correct process acknowledgments.
- Shared public routing policy/rendering across Binary, script and Helm. Explicit
  capture uses a dummy key; native credentials are resolved only when required.
  Agent-facing and query-facing fakeintake endpoints are separate.
- Typed artifact providers: **invoke-binary**, **existing-binary**, **invoke-image**,
  **existing-image**, **existing-package**, and **omnibus-repackage**. Commands and
  drivers do not dispatch on build task names or artifact formats.
- Public artifact receipts bind actual paths/inventory/checksums, target and image
  identity. Local binary staging uses the real `dev/embedded` closure, not legacy
  `dev/lib`, plus an explicitly recorded runtime-image/site-packages dependency.
- Native binary and image build tasks emit result manifests. Omnibus support wraps
  the existing `omnibus.build-repackaged-agent` task inside a supplied isolated
  build image; it does not implement a second packaging engine.
- Existing artifacts never build during ordinary update. Skip-build verifies
  installed pins without re-copying arbitrary source outputs. Binary-only receiver
  apply also reuses the installed artifact and mutable runtime state.
- New binary runtime state uses an owner-labelled Docker volume, reused during
  update/rewire and removed by normal stop. Private root-owned state no longer
  prevents deletion of the host environment directory.
- Ubuntu DEB installation forces same-version reinstallation, verifies installed
  dpkg identity and every declared executable-role hash, and removes only service
  masks whose ownership it can prove. Failed operations remain visible for repair.

See the maintained usage/API references:

- [CLI quickstart](../../test/e2e-framework/cmd/e2ectl/README.md)
- [Receiver guide](../../test/e2e-framework/testing/receivers/README.md)
- [Artifact/package guide](../../test/e2e-framework/testing/installers/agentbuild/README.md)

## Verification performed

After receiver and integration review/fix passes, the parent reran without cached
**test results** (compiled build artifacts could still use the normal Bazel cache):

- **25 framework/CLI test targets passed**, covering receivers, installers,
  providers, snapshots, clients and shared fakeintake defaults.
- **Trace consumer regressions passed** (`TestReceiverRouting`), exercising real
  configuration loading and handlers with recording transports.
- **27 Python tests passed** (9 artifact-result tests and 18 Omnibus tests).
- **Real apt/dpkg regression passed** inside a disposable network-disabled Ubuntu
  container with no host mounts. Same-version changed core/trace bytes were
  installed; foreign replacement masks were preserved; postinst-modified
  executables were rejected. Systemctl behavior in this fixture is stubbed, not
  a claim of a real SSH/systemd/Agent package deployment.
- **CLI Pulumi dependency query was empty**.
- Parent added a semantic infrastructure-input comparison regression: changed
  comments/default spelling are accepted, changed worker count/fixture intent
  are rejected, and stored config remains untouched. The two affected targets
  passed freshly after this final adjustment.

Local Linux arm64 smokes performed during implementation:

- Real `invoke-binary` build, staged CPU and Python checks with networking disabled,
  existing-binary reuse, negative unprofiled-receipt rejection, skip-build with the
  original source receipt unavailable, and receiver-only apply.
- Real hacky image build and receipt, followed by existing-image install/update
  on an isolated kind cluster without rebuilding. Core/process/trace rollout and
  the separately released Cluster Agent became ready.
- Runtime-volume state with root-owned mode-000 nested contents survived skip-build
  and rewire; plain stop removed only the owned resources.
- Parent delivery smoke: fresh `system.cpu.user` series attributed to the expected
  hostname and this new fakeintake's start time arrived. It then rewired the same
  binary to a running blackhole; artifact record/runtime volume were unchanged.
  The sink acknowledged a POST, returned 404 for payload queries, and did not log
  the probe body. No retained-payload verification of blackhole Agent delivery is
  claimed. The initial smoke's inline-host-tag assumption was corrected in the
  harness; the payloads were arriving and were identified by host/time instead.

All smoke environments/containers/networks/owned volumes were cleaned up. Private
logs and receipts under `/tmp/e2ectl-parent-validation.OAQcxd` and
`/tmp/e2ectl-parent-receivers.S2X9Er` are local evidence, not portable/reusable build
artifacts after teardown. No real Datadog data delivery, cloud provisioning,
host package installation or host Omnibus execution was performed.

## Important boundaries still open

1. **Explicit package receivers are unsupported.** Existing DEB/repack providers
   support visibly legacy routing only until their managed receiver capability
   contract is validated. No fallback from an explicit receiver is performed.
2. **Full Omnibus production and real SSH/systemd package startup have not been
   exercised live.** They need a known isolated native toolchain and an authorized
   disposable Ubuntu host. Never run repack against the host `/opt` tree blindly.
3. **Receiver RC is disabled for capture/sink.** RC trust/cache transitions and
   Helm/script/package route-only apply are rejected, not silently approximated.
4. **Arbitrary artifact versions do not establish routing compatibility.** Managed
   binary/image use requires the supported producer evidence; local images retain
   the tested base constraints. RPM/MSI, cross-builds and automatic caching are
   not implemented.
5. **Routing is not network isolation.** Tracer-flare diagnostic uploads retain
   native-site behavior; arbitrary checks/sidecars and fixture forwarding remain
   separately owned. Applied/readiness state is not automatic delivery proof.
6. Package verification covers status/identity and declared executables, not every
   conffile or postinst-modified runtime byte. Owned masks may remain on failure;
   no automatic package rollback or legacy runtime-state migration is promised.

The existing user edit to the metric-emission test was not changed. Its SHA-256
remained `fb53b6b56979157c02436c56d4adc0fde299d1d63461e00f1da5dc2f78480145`.
