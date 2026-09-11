# QA / e2ectl plans — categories and implementation status

**Source baseline:** `9151707198c` on `rework-qa-experience`, plus local planning documents.
**Assessment:** current code inspection and previously recorded validation; no new builds,
tests, cloud operations or Confluence refresh were run for this inventory.

This is the entry point for the 15 QA/e2ectl planning/reference documents, organized into
status folders under `qa-plans/`. Each document has a tracking banner; this index contains
the consolidated status and remaining work.

```text
qa-plans/
├── qa-e2ectl-plans-index.md          this index (source of truth for status)
├── implemented/   category A — implemented foundations
├── partial/       category B — partially implemented / active hardening
├── pending/       category C — pending feature designs, not implemented
├── deferred/      category D — vision and deferred roadmap
├── historical/    category E — superseded documents and approaches
└── notes/         category F — implementation journal / evidence
```

Folder membership follows the status vocabulary below. When a document's status changes,
move it with `git mv`, update its banner, and fix cross-links (see §10).

**Shareable summary:** [qa-e2ectl-summary.md](qa-e2ectl-summary.md) — a concise,
developer-facing overview of the whole effort (what changed, how to use e2ectl today,
the design rules, what's in flight). Start there; use this index for depth and status.

> **A written plan or code snippet is not an implemented feature.** “Implemented” here
> means present in this branch, not necessarily merged, pushed, fully hardened or covered
> by every original acceptance gate. Framework capability is not automatically CLI support.

## 1. Status vocabulary

| Status | Meaning |
|---|---|
| **Implemented — bounded scope** | The named feature exists in code. Broader ambitions and remaining validation are listed separately. |
| **Partial — active follow-up** | Some of the plan landed; meaningful implementation or acceptance work remains. |
| **Pending — not implemented** | A concrete design exists, but its new feature/code path has not been implemented. |
| **Deferred — roadmap** | A future goal, not part of the next committed implementation slice. |
| **Historical / superseded** | Kept for rationale; its detailed API/ordering/claims are no longer the current plan. Do not implement it blindly. |
| **Reference / journal** | Context or evidence, not a delivery checklist. Historical entries may describe older states. |

“Pending” and “not implemented” are intentionally one status for concrete proposals.
“Deferred” separates ideas not being scheduled yet; “superseded” separates approaches that
should not be implemented at all in their old form.

## 2. At a glance

### What exists now

- Core CLI with `environments`, `init`, `start`, `list`, `install`, `update`,
  `fakeintake names|metrics|health`, and `stop`.
- Registered `kind` and `ec2-host` environment types only.
- One Agent configuration per environment; one standard fakeintake fixture option.
- Typed environment schemas, generated annotated starter YAML, optional semantic hooks,
  strict parsing/defaults and normalized executor transport.
- Explicit driver/scenario registration; cloud infrastructure/fakeintake through the
  executor, Agent installation and local kind operations in the Pulumi-free core.
- Private snapshot writing, automatic component bindings and static typed attachment.

### What does not exist in the CLI yet

- EKS registration or mixed Linux/Windows standalone installation.
- Explicit receiver declarations/selection or safe receiver switching (stock environments).
- Custom multi-component scenarios: scenario-owned topology parameters, scenario-exposed
  installers, and the Go action tools that pair with them.
- Public `testing/configschema` / `testing/fixtures/config` moves and the small
  moves.
- The proposed test runner, workload deployment, classics catalog or CI generation.

By decision (see the revised custom-environment plan), the CLI will **not** gain plural
agent envelopes, per-agent selectors, scoped multi-agent state transactions or generic
topology/footprint machinery: custom topologies stay scenario-owned. The existing framework
already supports EKS, custom Go environments and multi-Agent/multi-fakeintake tests; the
pending work is exposing the appropriate behavior through scenario registrations, not
through new CLI concepts.

## 3. Category A — implemented foundations

### Primary document

| Document | Implementation status | Read it for | Still outside the completed scope |
|---|---|---|---|
| [Typed-config proposal](implemented/qa-e2ectl-typed-config-proposal.md) | **Implemented — core typed-config/template simplification** | Rationale for one data-only type per environment and generated examples | Installer-owned schemas, public schema-package move, complete reproduction manifests, plural topology |

The proposal retains illustrative older syntax. For the actual API, use
[`cmd/internal/configschema/README.md`](../test/e2e-framework/cmd/internal/configschema/README.md)
and the [CLI README](../test/e2e-framework/cmd/e2ectl/README.md).

### Completed feature ledger

| Feature | Code / evidence | Boundary of the completion claim |
|---|---|---|
| Shared typed environment params | `cmd/internal/envconfig/{kind,ec2host,fixtures}` | Separate types, not a cross-provider union |
| Automatic defaults/validation/examples | `cmd/internal/configschema`; commit `9151707198c` | Existing bounded annotation vocabulary; not arbitrary Go serialization |
| Optional `Validate(params)` | Schema validators and typed driver adapter | Shared rules execute in both relevant processes; runtime prerequisites remain separate |
| Offline discovery and starter config generation | `cmd/e2ectl/discovery.go`, tests | `init` writes private files and refuses overwrite; no runtime topology generation yet |
| Explicit driver and executor registration | `internal/driver/registry.go`, worker `scenarios.go` | Current registered bases remain kind and EC2 |
| Pulumi-free runtime seam | `components/outputs`, `components/os/types`, `testing/provisioner`, standalone/installers | CLI dependency boundary established; full consumer compatibility still needs repair/audit |
| Snapshot component bindings/private writes | `snapshot_bindings.go`, `snapshot.go`; commit `1d9212d8741` | Binding correctness is not complete lifecycle recovery or versioned topology state |
| Cloud fakeintake ownership restored to Pulumi | EC2 scenario adapter; commit `4f576f9b45f` | Existing ECS Fargate deployment, no duplicate Docker-over-SSH setup |
| Versioned normalized executor requests | `workerclient`, `DecodeResolved`, fixture JSON decoder | Protocol 1; not the proposed candidate-result protocol |
| Safer candidate-config persistence | `loadOrStoredConfig`, `saveAppliedConfig`, captured `File.Source` | Invalid candidates do not replace stored config; full transaction/recovery model is pending |
| Local kind Agent iteration | Existing install/update path and recorded live demonstration | Not proof that arbitrary artifacts, version-only update or all cloud workflows work |
| Installer-typed agent sections | `config.parseAgent`, `cmd/internal/envconfig/{script,helm}`, `installer.AgentExample/Artifact`; [typed agent config plan](partial/qa-e2ectl-typed-agent-config-plan.md) | Stock installers only: `agent.install` selector + `agent.script`/`agent.helm` typed sections; `api-key` removed; image rules scoped to `helm`; unknown/legacy fields fail with `set it under agent.<install>` guidance; section contents validated at install/update (start stays infrastructure-only); the §5.3 contract revision, scenario sections and stored-config auto-migration remain pending |

Recorded validation includes nine focused Bazel test targets (config/driver/template
+ typed section schema suites), both binary builds, an empty
Pulumi dependency query for the core, and approximately 90 ms offline discovery/generation
smokes. Those results do not establish a green full cross-module/tagged test matrix or a
fresh end-to-end EC2 acceptance run after every later change.

## 4. Category B — partially implemented / active hardening

| Document | Status | How to use it now |
|---|---|---|
| [Ordered follow-up plan](partial/qa-e2ectl-follow-up-plan.md) | **Partial — active follow-up** | Main correctness/reliability backlog for the existing kind/EC2 workflows |
| [Follow-up code plan](partial/qa-e2ectl-follow-up-code-plan.md) | **Partial — active follow-up** | File-level details for the same eight workstreams; not eight completed patches |
| [Extensibility plan](partial/qa-e2ectl-extensibility-plan.md) | **Implemented foundation; later details partially superseded** | Rationale for explicit registration and process boundaries; use newer plans for current/future contracts |

### The eight follow-up workstreams

| Step | Current status | Already done | Still pending / not verified |
|---|---|---|---|
| 1. Framework compatibility | **Pending** | Dependency extraction exists | Remaining `runner.ConfigMap` / `environments.WindowsHost` consumers; tagged runner tests; restored YAML helper coverage; broader build checks |
| 2. Snapshot handoff | **Partial** | Bound resource mapping, shared parsing, private atomic snapshot writes | Persist successful provisioning outputs before client/custom initialization can fail; selective/data-only import path; broader state-version contract |
| 3. State and lifecycle safety | **Partial** | Read-only candidate loading, post-success private config replacement, private worker jobs; failed starts mark entries `error` (command-layer owned) and are recoverable: plain `stop` cleans half-created kind clusters by deterministic name, and `stop --force` removes an entry whose teardown fails with a named-resources warning | Path/ownership hardening, locks, incarnation/revision tokens, operation checkpoints, failure recovery beyond entry level, cancellation |
| 4. Effective config / repair | **Partial** | Order-independent strict parsing, defaults and driver preparation | Helm config/integration forwarding, credential authority, repair without initializing a broken Agent first, version-only update build bug |
| 5. Installer/artifact extension boundary | **Partial** | Shared params, protocol checks, optional update, explicit registration | Installer-owned options, artifact-specific validation/preparation, narrowed contracts and target selection |
| 6. EC2 and endpoint setup | **Partial; main ownership fix implemented** | Pulumi VM + optional Fargate fakeintake, bound endpoint reads, local kind stays local | Full gated EC2 acceptance, private-network access handling, kind allocation/cleanup checkpoints |
| 7. Fakeintake parity / reproducibility | **Pending, with earlier query optimization retained** | CLI avoids one query per metric name | Shared v2/v3 summary API, correct empty JSON, consistent pinned image/RC defaults, stable chart/image selections |
| 8. Build/performance/adoption | **Partial** | Bazel targets, README/guidance, recorded startup/dependency checks | Supported `dda inv e2ectl.*` tasks, CI dependency/performance enforcement, repeatable cold-build/query budgets and broader verification |

### Concrete evidence for unfinished work

This inventory confirmed, rather than inferring from old plan text:

- References to `runner.ConfigMap` and `environments.WindowsHost` remain in new-e2e sources.
- `common/utils/yamlutil` currently contains its implementation/BUILD file but no restored tests.
- The worker still calls `standalone.ProvisionE` before writing the snapshot; that path
  imports and initializes components/custom environments.
- `cmdUpdate` still enters the local image build branch whenever `--skip-build` is absent.
- The standalone Helm installer still has `const releaseName = "dda-linux"`.
- The local CLI fakeintake helper still selects `public.ecr.aws/datadog/fakeintake:latest`.
- The CLI summary query still embeds `/api/v2/series` rather than a shared v2/v3 bulk API.
- No e2ectl-specific task/CI references were found under `tasks`, `.gitlab` or `.github`.
- The kind missing-snapshot teardown branch still deletes the entry without reliably
  establishing that no cluster remains.

This list is not a newly executed full review/test run. It is enough source evidence not
to mark these workstreams complete.

## 5. Category C — pending feature designs, not implemented

These documents are complete as design artifacts. Their new CLI features are not complete
or present as implementation code.

| Workstream | Documents | Current implementation status | First useful implementation slice |
|---|---|---|---|
| EKS environment | [EKS scenario plan](pending/qa-e2ectl-eks-scenario-plan.md) | **Pending — no EKS CLI driver/schema/registration** | Shared EKS params + Linux/Windows rule + existing Pulumi scenario adapter; then standalone mixed-OS installation |
| Local host agent | [Local agent plan](pending/qa-e2ectl-local-agent-plan.md) | **Pending — no local driver/binary installer** | `local` driver (per-env Docker network + fakeintake-only snapshot) + `binary` installer (`dda inv agent.build` → Linux binary mounted into the pinned runtime image → container run → heartbeat readiness); no new CLI concepts |
| Explicit Agent receiver wiring | [Receiver wiring plan](pending/qa-e2ectl-receiver-wiring-plan.md) | **Pending — routing remains inferred from fakeintake presence** | Named destination selection and an explicit resolved plan consumed by current single-Agent installers |
| Custom environments (multi-Agent / multi-fakeintake scenarios) | [Custom-environment plan](pending/qa-e2ectl-custom-environments-plan.md) | **Pending — scenario-owned model designed; no custom scenario implemented** | Public schema package, then the three-VM recipe exposing its own installer (attach via the existing public path, direct field selection) |
| Developer implementation blueprint | [Custom-environment code plan](pending/qa-e2ectl-custom-environments-code-plan.md) | **Pending — code excerpts, not a patch** | Public schema/fixture move; component-level installer entry points, contract revision, scenario installers reusing the shared installers |


### Pending features inside those plans

| Proposed capability | Status | Main owner / notes |
|---|---|---|
| EKS Linux-only / Linux + Windows, reject Windows-only | Not implemented in e2ectl | EKS plan; existing framework node-group helpers are reusable |
| Two OS Helm releases sharing one Linux Cluster Agent | Not implemented in standalone installer | EKS + receiver/installer work; Pulumi path is existing reference behavior |
| Choose Datadog while retaining a provisioned fakeintake | Not implemented | Receiver plan; do not delete the fixture to simulate selection |
| Receiver-aware credentials, TLS, per-feature/RC policy | Not implemented as a common plan | Receiver plan; existing settings are incomplete/divergent between adapters |
| Explicit fakeintake forwarding policy in CLI configuration | Not implemented | Separate fixture/protocol change; AWS default forwarding is not capture-only |
| Plural `schema: 2` envelope, `--agent` selectors, per-agent state journals | **Superseded by decision** | Replaced by scenario-owned params and scenario-exposed installers; the envelope stays schema 1 |
| Concrete `Registration` replacing the Driver extension API | **Dropped by decision** | Current Driver/Installer contracts stay; scenarios plug in via thin cmd adapters |
| Public `testing/topology` / `envstate` / operations / `scenario` helper packages | **Dropped by decision** | Installers attach their own env type via the existing `StaticStackProvisioner` + `standalone.ProvisionE` path, select fields directly, and publish with the existing `UpdateSnapshotResource` |
| Open map-keyed agent/fakeintake collections in scenario config | **Dropped by decision** | Fixed named slots (`agent-a`, `intake-a`) with wiring fixed in scenario code; string map keys are unpredictable across schema, provisioner and installer |
| Component-level installer entry points (`InstallOnHost(host, fakeIntake, params)`, Helm analogue) | Not implemented | The one shared change the revised custom model needs: installers take the component types environments are made of; stock wrappers delegate |
| Minimal driver/installer contracts (no `*config.File`/`envstore.Entry` bags; wrapper-owned snapshot round-trip) | Not implemented | `*config.File` appears only at `Prepare`; typed installers receive prepared params + the attached environment and mutate its component fields — the `installer.Define` wrapper attaches and publishes changed fields (`outputs.Export`, the inverse of `Import`), removing per-installer attach/publish boilerplate and the double-decode |
| Installer-typed agent sections (`agent: {install: X, X: {...}}`) | **Implemented for the stock installers** — `script`/`helm` section schemas in `cmd/internal/envconfig`; scenario agent sections and the §5.3 contract revision remain pending |
| Scenario-exposed installers (multi-agent installs without CLI changes) | Not implemented | The core of the revised custom plan; reuses `Implementation.Installers()` (typed); no environment-view types — installers pass their env's component fields directly |
| Public schema and legacy fixture packages | Not implemented | Prerequisite: scenario packages live outside `cmd/` and need the schema engine |
| Three-VM/two-fakeintake registered recipe | Not implemented | Worked code/YAML examples only |
| HA and NSS as scenario installers + Go action tools | Not implemented | Existing Go tests are evidence; setup becomes installer work, experiments stay Go |
| Multi-install Kubernetes co-location | Not implemented, scenario-owned | A benchmark-style scenario owns placement/CRD/port rules in its own code; no generic CLI support |

### Relationship between these proposals

- Receiver work can begin with the existing single-Agent CLI; its policies are also what
  scenario installers apply for their multiple Agents.
- EKS infrastructure registration can use the existing typed driver path; replacing the
  whole driver API is not a prerequisite just to provision EKS.
- Fully supported mixed-OS EKS requires the standalone Helm/receiver work, not merely an
  EKS entry in the registry.
- Custom scenarios are created and installed through the existing commands; their runtime
  complexity lives in scenario installers and Go action tools, not in the CLI.
- The typed agent config plan's core is implemented for the stock installers (the
  envelope split, section schemas, generation, `api-key` removal); its remaining §5.3
  contract revision is owned by the custom code plan, and scenario agent sections arrive
  with the first scenario. It supersedes the raw `agent.options` sketch in the follow-up
  plan (§5.2).
- Baseline locking/recovery remains owned by the hardening backlog (category B); custom
  scenarios inherit it through the shared commands and need no machinery of their own.

Do not implement conflicting variants from several plans simultaneously. The revised
scenario-owned model is the current custom-environment direction; the plural-envelope and
generic-operations model it replaces is superseded and must not be revived from older
text. The existing hardening plan still owns unresolved baseline correctness items.

## 6. Category D — vision and deferred roadmap

| Document | Category/status | Use |
|---|---|---|
| [Vision one-pager](deferred/qa-vision-onepager.md) | **Vision / reference; broad goals deferred** | Short explanation of the desired user experience |
| [Confluence update proposal](deferred/qa-vision-confluence-update.md) | **Vision / roadmap; not a shipped CLI spec** | Full product direction, test/CI model and open questions |

Both documents mix foundations that now exist with future commands that do not. Their
example YAML and timing/reproduction promises are not the current CLI API or guarantees.
The Confluence proposal was based on page v4; this inventory did not refresh or publish it.

### Deferred/not implemented roadmap items

- `e2ectl test`, needs/freshness validation, create/reuse modes and suite context injection.
- Workload `deploy`, setup lifecycle and automatic workload-to-Agent wiring.
- A shared “classics” catalog and test-config-driven CI job generation.
- `plan validate/list`, resolved CI reproduction artifacts and runtime drift reporting.
- General host-binary/MSI/DMG/operator artifact workflows and remote/local Windows image builds.
- DevMode-to-envstore adoption/import and automatic discovery of kept E2E environments.
- Arbitrary additional cloud/local backends, custom AMIs/node pools, pooling, TTL/GC and reset.

Existing test-side static attachment is a foundation, not an implementation of the proposed
CLI test runner. A private connection snapshot is not a complete/shareable reproduction
manifest. These distinctions supersede broader wording in older vision examples.

## 7. Category E — historical / superseded documents and approaches

| Document | Why it is historical | Current replacement |
|---|---|---|
| [Original M1 plan](historical/qa-e2ectl-plan.md) | Core commands/seams mostly exist, but original layout/schema/milestone order is obsolete and acceptance is not wholly closed | CLI README, typed-config docs, active hardening ledger above |
| [Original M1 engineering design](historical/qa-e2ectl-m1-design.md) | Old helper/API/config sketches, trust-only executor validation and compatibility assumptions are no longer authoritative | Current source and typed schema/protocol implementation |
| [Consolidation review](historical/qa-e2ectl-consolidation-plan.md) | Later review explicitly superseded its ordering and broad compatibility claims | Follow-up plan + code plan |
| [Original vision draft](historical/qa-experience-vision-draft.md) | Explicitly superseded draft | Confluence update proposal, with newer feature plans qualifying custom/state behavior |

### Superseded choices — not tasks to implement

- Handwritten embedded driver `template.yaml` files and mandatory per-driver validation
  plumbing: replaced by typed schemas and generated starter YAML.
- Separate CLI/worker EC2 parameter structs: replaced by one shared data-only type.
- A worker that trusts the CLI without revalidation: replaced by normalized decoding and
  protocol checks at the executor boundary.
- Deploying EC2 fakeintake with Docker-over-SSH on the Agent VM: replaced by the existing
  Pulumi ECS Fargate scenario.
- Running Agent installation/Helm inside the Pulumi executor: current installation stays
  in the Pulumi-free runtime path.
- Assuming aliases made all framework consumers automatically compatible: remaining
  consumer migration and tagged-test issues are still active.
- Treating every custom test as reducible to generic YAML/CLI actions: the new custom
  plan explicitly supports trusted Go-only workflows.

The extensibility document also contains historical sections, even though its central
explicit-registration/process-boundary foundation is implemented. Its old “no shared
parameter struct” wording must not be read as rejecting today's **per-environment**
shared CLI/executor type; the prohibition is against a giant cross-provider union.

## 8. Category F — implementation journal / evidence

| Document | Status | Reading rule |
|---|---|---|
| [Implementation notes](notes/qa-e2ectl-implementation-notes.md) | **Reference / chronological journal** | Later entries can supersede earlier “final state” claims; use this index/current code for status |

The journal records live local experiments, regressions, snapshot fixes, stale-binary/disk
incidents and the final typed-config work. It is not a current checklist. Historical
statements about worker installation, Docker-on-VM fakeintake or completed consumer
migration must not override later findings.

Unrelated local edits, including the `/resume` insertion, remain preserved. User environment
configuration files are not plans and are deliberately excluded from this inventory.

## 9. Suggested execution order from here

This is a proposed priority order, not a claim that all designs have been approved or
scheduled for implementation.

1. **Repair baseline compatibility and the small correctness gaps**: remaining consumers,
   restored tests, version-only update behavior and ignored configuration/credential fields.
2. **Finish safe state/lifecycle foundations**: preserve outputs before initialization
   failures, lock/checkpoint mutations, retain cleanup identity, handle partial kind setup.
3. **Implement explicit receiver policy on current installers**, including visibility of
   real-backend/forwarding behavior and safe replacement of routing settings.
4. **Move the schema engine public and implement the simplest
   multi-host recipe** (three VMs / two fakeintakes) with a scenario-exposed installer —
   attaching via the existing public path, selecting env fields directly, and adding no
   new CLI concepts or helper packages. Follow with HA/NSS scenario installers and
   their Go tools.
5. **Add EKS and complete supported mixed-OS/multi-install Helm behavior**, reusing shared
   state/receiver foundations; EKS infrastructure work can be a separate earlier slice.
6. **Then expand the test/CI/reproduction vision**, based on stable state and ownership
   contracts rather than raw snapshots and implicit singleton assumptions.

Build integration, dependency gates and fakeintake pin/query parity can proceed as focused
parallel workstreams; do not hide them behind a broad architecture rewrite.

## 10. How to keep this index accurate

For each implementation commit:

1. Update the relevant feature/workstream row, not just the document's title.
2. Link the commit or concrete source/test path proving what changed.
3. Separate code present, tests passed and live acceptance; record unverified gates.
4. If only part of a plan landed, keep the overall plan **Partial** and list the remainder.
5. Mark replaced approaches **Superseded**, rather than leaving them as competing TODOs.
6. Do not mark a proposal implemented because its Markdown snippets pass syntax checks.

The status folders are deliberately mutable: when a document's status changes, `git mv` it
to the matching folder (`implemented/`, `partial/`, `pending/`, `deferred/`, `historical/`,
`notes/`), update its category banner and this index's tables, and rewrite cross-document
links — they are relative, so a move breaks them. Prefer `git mv` over plain copies so
history follows the file. A document whose status is disputed stays in its current folder
with a note in this index rather than being moved speculatively. Banners and this index
together remain the source of truth for progress; a folder alone is not evidence of
implementation.
