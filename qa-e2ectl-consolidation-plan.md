# e2ectl integration & consolidation review — plan

> Evaluation of the five commits on `rework-qa-experience` against the existing
> e2e-framework: duplication audit, consolidation opportunities, and a prioritized
> action plan. No code — plan only.
> Inputs: `git log 26119e657a4^..HEAD`, the framework source, and
> `qa-e2ectl-implementation-notes.md`.

## 1. Integration status

**What genuinely fits:**
- The seam (`testing/provisioner`) landed as a *subtraction*: existing importers
  unchanged via aliases; module builds green (`go build ./...`); bazel builds green;
  `testing/provisioner` has zero pulumi packages in its dep graph.
- `standalone.ProvisionE` is additive; `ai-sandbox` (the prior standalone consumer)
  compiles untouched and is the existence proof for the worker's design.
- The installers (`helm`, `installscript`) are used as intended — reused, not forked.
- The snapshot format is byte-compatible with `StaticStackProvisioner`'s reader — the
  kind driver and the worker speak the same currency the framework already knows.
- No behavior change anywhere in existing test paths (config-driven tests, create-*
  tasks, kindvm suites) — nothing new is *required* of existing consumers yet.

**Verification gaps to keep in mind:**
- The EC2 worker path (provision/destroy/install-host) is compile-checked only.
- Broader framework test suites (pulumi provisioner tests, new-e2e smoke) were not run.
- The compat aliases make compile-level compatibility certain; behavior is untested.

## 2. Duplication audit

### 2.1 True duplication — consolidate

| # | What | Where | Plan |
|---|---|---|---|
| D1 | Snapshot file parsing, twice | `provisioner.ReadSnapshotFile` vs `StaticStackProvisioner.readResources` (kept in `provisioners`) | **Highest drift risk in the set**: two implementations of the same format. StaticStackProvisioner's reader should delegate to the Pulumi-free `provisioner.ReadSnapshotFile`; one format, one parser. |
| D2 | Worker job struct, twice | `cmd/e2ectl-worker/main.go` `job` vs `cmd/e2ectl/internal/workerclient.Job` | Same JSON shape maintained by hand in two binaries. Move the type to a small shared Pulumi-free package (e.g. `testing/e2ectljob`) that both import. The `internal/` restriction forced the duplication; relocating kills it. |
| D3 | EC2 option assembly, twice | `provisionEC2` + `destroyEC2` in the worker | Extract one `ec2Options(job)`. Mechanical. |
| D4 | Attach helpers, near-duplicates | `attachKubernetesEnv` / `attachHostEnv` | One generic `attach[Env any](j)` helper. Mechanical. |
| D5 | `DefaultRCSigningKeySeed`, verbatim | `components/datadog/fakeintake/component.go` vs `kinddriver.go` | Duplicated constant (core must stay Pulumi-free). Interim: keep with cross-reference comments (done); real fix: the outputs seam (§3, P3) gives the constant a Pulumi-free home. |
| D6 | Outbound-IP trick, twice | `fakeintake/docker.go` vs `kinddriver.go` | Same rationale as D5; same fix (Pulumi-free util). |
| D7 | Fakeintake image pinning policy | framework pins `test/fakeintake/version.Tag` with `E2E_FAKEINTAKE_IMAGE_OVERRIDE`; e2ectl uses `:latest` | e2ectl should adopt the same pinned tag + override semantics (`test/fakeintake` is Pulumi-free, importable). Until then the CLI can silently diverge from the fakeintake version the tests run against — a compatibility trap. |
| D8 | Supported-OS table | `config.SupportedOS` vs `e2eos` descriptors | Hand table that can drift. Documented as deliberate at design time; the outputs seam (P3) lets validation derive from `e2eos` directly. |

### 2.2 Looks duplicated but keep — for the good reason

| # | What | Why it exists | Verdict |
|---|---|---|---|
| K1 | Two kind cluster creators: `kinddriver` (kind CLI, Pulumi-free) vs `localkubernetes` Pulumi provisioner | The core CLI must not link Pulumi; the Pulumi one powers existing suites with richer wiring (workload apps, agent options, pre-hooks) | Keep both **for now** — but this is the *strategic* duplication. Decide at M4 (K8s milestone): either the Pulumi kind provisioner migrates onto the CLI driver, or the two coexist permanently with the CLI driver documented as the "manual/durable" path and Pulumi as the "suite" path. Don't decide earlier than needed. |
| K2 | Two local-fakeintake deployments (docker CLI vs Pulumi docker provider) | Same as K1 | Same decision, same time. |
| K3 | Two agent-install paths: installers (e2ectl, no-pulumi tests) vs Pulumi agent components (in-provision install) | Pre-existing framework split; e2ectl is merely the third consumer of the installers | Keep. The vision's end-state (tests declare config, installers do the work) naturally shrinks the Pulumi agent component over time — no forced consolidation. |
| K4 | Worker `provision-ec2` vs `cmd/ai-sandbox` | ~60% overlap (standalone + awshost + attach patterns), different products | Keep separate now. Watch for convergence at M2: ai-sandbox could become another worker action (`run-ai-tool`) or consume the `provision-ec2` job — either direction removes the overlap. |
| K5 | Compat aliases in `provisioners` | Migration shim so nothing breaks | Keep indefinitely as a facade (zero cost), or schedule import migration + deletion as busywork — recommendation: keep. |
| K6 | envstore `fakeIntakeURL/port` vs snapshot `fakeIntake` | Two views of one fact (client view 127.0.0.1 vs agent view outbound IP) — deliberate | Keep, but it can be *simplified*: store only the port in meta and derive both URLs; removes a consistency risk after `start`. |

### 2.3 Integration gaps (should exist, doesn't yet)

- **G1 — no `dda inv` task.** Binaries were built ad hoc to `/tmp` during the run. M1 plan
  task 7 promised invoke wiring (`ai_sandbox.py` pattern): `dda inv e2ectl.build` +
  `e2ectl.run`, binaries in `bin/`, worker sitting next to the core binary (workerclient
  already discovers it there). Without this, adoption and CI wiring stall.
- **G2 — no README.** `cmd/e2ectl` has usage text and `examples/`, but no README section
  in e2e-framework docs; the AGENTS.md hierarchy expects one.
- **G3 — `install` asymmetry.** `install` demands `--config`; `update` reads the stored
  copy. `install` should default to the stored config when `--config` is omitted.
- **G4 — DevMode interop.** `e2e.WithDevMode()` keeps infra alive after a test but
  registers nothing in envstore; an e2ectl user cannot discover or attach to it. M3
  (test attach) should bridge: DevMode suites write an envstore entry + snapshot.
- **G5 — create-* bridge.** The old manual-env tasks remain the sanctioned path for
  "give me a VM to poke". Plan their deprecation only after e2ectl covers the VM case
  end-to-end (M2), and meanwhile document e2ectl as the experimental alternative.
- **G6 — envstore home.** `~/.e2ectl` vs runner conventions (`~/.test_infra_config.yaml`,
  home-dir output). Separation is arguably *cleaner*; decide once and document. No urgency.

## 3. Simplification candidates (small, do opportunistically)

- `cmd/e2ectl/util.go` is a grab-bag (`readSnapshot`, `jsonUnmarshal`, `osCommand`) —
  three wrappers that add nothing; `fillFakeintakeURL` belongs in `envstore`, `osCommand`
  is a pointless indirection.
- `fakeintakecmd.Metrics` without `--name` did one `FilterMetrics` call per metric
  name — quadratic; found live (minutes against a warm fakeintake) and fixed: one
  `GetRawPayloads` call, parsed locally with `aggregator.ParseMetricSeries` (~5s for
  ~900 metrics). Lesson: perf notes written before running the thing are worthless.
- The **binary name decision** (`e2ectl` is a placeholder): every file, doc and
  env-var name references it. Renaming is cheap *now* (one binary, two commands' help
  text, `E2ECTL_HOME`, `E2ECTL_WORKER`, docs) and expensive after adoption — decide
  before G1 lands.

## 4. Prioritized action plan

**P1 — drift-risk removal (do first, small):**
1. D1: StaticStackProvisioner delegates to `provisioner.ReadSnapshotFile`.
2. D2: shared job package; delete the duplicated struct.
3. D3, D4: extract `ec2Options` and the generic attach helper.
4. G3: `install` defaults to the stored config.

**P2 — adoption enablers:**
5. G1: `dda inv e2ectl.build` / `e2ectl.run` (ai-sandbox pattern), binaries in `bin/`.
6. D7: fakeintake image pinning parity with `version.Tag` + override.
7. G2: README (+ AGENTS.md pointer).
8. Name decision, then rename if needed (before G1/G2 docs solidify).

**P3 — the structural seam (bigger, unlocks several D-items at once):**
9. Move the Output structs (`FakeintakeOutput`, `HostOutput`, `ClusterOutput`,
   `KubernetesAgentOutput`, `HostAgentOutput`) into Pulumi-free `outputs` packages;
   Pulumi packages alias them. Unlocks: D5 + D6 dedup, D8 (validate from `e2eos`),
   `install`/`attach` moving from the worker into the core CLI (worker shrinks to
   EC2-only), and G4's bridge becomes trivial. This is the highest-leverage refactor
   on the list and the one the design doc already scheduled as the follow-up seam.

**P4 — strategic consolidations (milestone-scoped, decide at the right time):**
10. K1/K2: kind driver vs Pulumi kind provisioner — decision at M4.
11. K4: ai-sandbox convergence — evaluate at M2.
12. G4: DevMode ↔ envstore bridge — part of M3 (test attach).
13. G5: create-* deprecation — after M2.

**Explicitly not planned:** touching existing suites, forcing any consumer to adopt
anything, or the bigger vision items (needs/setup/ci sections, wipe, locks) — those
follow the milestone plan (`qa-e2ectl-plan.md`), not this review.
