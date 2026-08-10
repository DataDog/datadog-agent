# ADR: On-demand & ownership-driven SMP regression experiment selection

**Status:** Proposed (v1)

## Context

SMP regression experiments in `test/regression/` today run as a fixed suite. We want teams to
(a) keep optional experiment suites in-repo and run them on demand, and (b) have the *right*
experiments run automatically when code they own changes — controlled from the PR itself, without
leaving GitHub. The mechanism must be reusable outside datadog-agent, so as much logic as possible
belongs in the `smp` CLI rather than CI glue.

This ADR records a **label-driven v1**: on-demand suites are selected by applying a **GitHub label**
to the PR, and ownership-driven suites run automatically based on **CODEOWNERS**. Selection lives in
GitHub's native label UI, which persists per-PR across pushes with no additional state to manage.

The existing **quality-gate experiments do not change** — they continue to run on every PR (and in
scheduled/nightly SMP runs) exactly as they do today. Everything here is additive: it introduces the
*additional* on-demand and ownership-driven suites alongside them.

## Requirements

Requirement IDs (R1–R9) are stable and referenced by the Decisions below. The sub-headings group
them by kind; they do not renumber them.

### User requirements

- **R1** — Run in-repo sets of experiments *on demand* (optional experiments).
- **R2** — A set of experiments *automatically runs when relevant code changes*. v1 defines
  "relevant" via CODEOWNERS; may get smarter later.
- **R3** — Teams can *organize experiments freely* (free-form folder structure).
- **R6** — Selections **persist across pushes / job reruns** — no reselecting after a push.
- **R8** — Each folder is documented: a **group description**, and a **per-experiment description**.

### Technical requirements

- **R5** — The experiment set is pulled from **PR HEAD, not main**, so new experiments are testable
  in their own PR.
- **R7** — The **majority of logic lives in the `smp` CLI**; CI jobs are thin wrappers, reusable in
  other CI setups.
- **R9** — Structural / correctness errors (e.g. duplicate experiment names) are caught as a **hard
  CI gate**, not discovered at run time.

### Provisional requirements *(under review — not yet committed)*

- **R4** — *Easy to rerun* the SMP job to include newly-selected optional experiments. *(Unsure this
  should be a requirement; parked here to revisit — see D9, which currently satisfies it.)*

## Decisions

### D1 — Modes are per (leaf) folder, not per experiment. *(R1, R2, R3)*

A leaf folder declares one `mode`; its experiments inherit it. Modes: `always`, `codeowners`,
`optional` (default `always` when unset — see D14).

Rationale: selection happens at folder granularity (a label maps to a folder), so a per-experiment
mode would make a folder's selection a partial/ambiguous signal. Per-folder modes keep selection
semantics unambiguous.

### D2 — Folder structure is Leaf XOR Dir. *(R3)*

A **Leaf** contains a `cases/` dir of experiments + a `mode` + a README; a **Dir** contains only
child folders (no `cases/`). A folder is never both.

Rationale: keeps the **label ↔ folder mapping 1:1** — a selectable label always targets exactly one
leaf, never a subtree. A Dir is purely organizational and carries no mode or label. Arbitrary nesting
of Dirs is still allowed.

### D3 — Mode semantics. *(R1, R2)*

- `always` — always runs; not user-selectable. The core Agent quality-gate suite
  (`quality_gates/`, `quality_gate_*`) uses this mode, and `always` is the **only** mode executed by
  scheduled/nightly SMP runs (`codeowners` and `optional` are PR-selection concepts with no meaning
  outside a PR). `always` is also the **default** mode when a leaf has no `mode` (see D14).
- `codeowners` — **hard-runs** when an owning CODEOWNERS team touched a file on PR HEAD (see D5);
  **also manually runnable** via its label when no owning team is involved.
- `optional` — runs only when its label is applied to the PR.

The mode (*whether* an experiment is selected) is orthogonal to its `kind` and `runner` (*what /
where* it is); see D13.

### D4 — Selection is via GitHub labels (native UI), not a custom comment. *(R1, R4, R6)*

Each `optional` and `codeowners` leaf declares a `label:` in its README frontmatter. **Applying that
label to the PR selects the folder.** The run job reads the PR's applied labels live from the PR
object.

Rationale:
- **Persistence is free** (R6): labels live on the PR and survive pushes and job reruns, so a
  selection never has to be re-applied and there is no separate selection state to store.
- **Native UI**: no custom surface to render or keep in sync — GitHub owns the widget.
- **PR-scoped**: labels are read from the PR object, so the result depends only on the PR, not main.

Labels are a **bounded, manually-created set** — one per selectable folder, created deliberately
(`gh label create smp/<folder>`), *not* spawned by CI. This is a conscious guard against label
sprawl (see Alternatives: auto-apply).

### D5 — `codeowners` is involvement-driven (hard), not label-driven. *(R2, R5)*

The `codeowners` hard gate's source of truth is **changed-files ∩ CODEOWNERS from PR HEAD**, computed
by the CI wrapper — **not** a label. The label on a `codeowners` folder only enables a *manual* run
when the owning team is *not* involved.

Rationale: a gate you can forget to (or choose not to) label is not a gate. Involvement is computed,
so an owning team's suite cannot be silently turned off. (Cosmetically surfacing involvement by
auto-applying the folder's label is deferred — see Open questions — to keep the label set bounded.)

### D6 — "Relevant code changed" = changed-files ∩ CODEOWNERS, computed from PR HEAD. *(R2, R5)*

The CI wrapper diffs the PR's changed files, resolves their owning teams from the **checked-out**
CODEOWNERS, and that set of "involved teams" drives `codeowners` mode.

Rationale: using PR-HEAD CODEOWNERS on both sides (changed files → teams, folder → team) makes the
result depend only on the PR — no dependency on what's merged to main. (Requested-reviewer would
depend on the *base* branch's CODEOWNERS; see Alternatives.)

### D7 — Discovery + resolution read the PR-HEAD checkout, in the `smp` CLI. *(R5, R7)*

`smp` enumerates folders/experiments from the config dir on the branch, reads each leaf's
`mode` / `label` / `description`, and **resolves the run set** from its inputs. CODEOWNERS never
enters the CLI (it is a CI input — see D8).

### D8 — SMP↔CI boundary. *(R7)*

- **CI wrapper (per-CI, repo-specific):** resolve the PR, mint tokens, diff changed files → involved
  teams, build the folder→owning-team map (CODEOWNERS), read the PR's applied labels and the target
  runner, and submit the resolved set to SMP (routing each `kind` to its submit command).
- **`smp` CLI (reusable, offline):** discover folders/experiments, read frontmatter, and **resolve
  the run set** as
  `{always} ∪ {codeowners where owner ∈ involved-teams OR its label applied} ∪ {optional where its label applied}`,
  filtered to the requested `--runner` (D13), then list/validate. Involved-teams, applied-labels,
  the ownership map, and the runner are **inputs** to `smp`, so CODEOWNERS and GitHub specifics
  never enter the cross-repo CLI.

### D9 — Rerun via label edit + job retry (v1). *(R4)*

To change what runs, edit the PR's labels and **retry the SMP job**. Because the run job reads the
PR's labels (and recomputes involvement) **live at job execution time**, a plain job *retry* is
sufficient — **no new push is required** — and it re-reads the current label set. Labels persist
across pushes, so nothing is re-selected on push either.

Rationale: a true label-*event*-driven trigger (a label add automatically starting a GitLab job) is
**not** possible with existing repo machinery — GitLab never sees GitHub label events, and there is
**no in-repo prior art** for GHA (or anything) calling into GitLab to start a pipeline. That path is
net-new, codeowner/credential-gated infra. Deferred (see Open questions).

### D10 — `smp experiments validate` is the CI gate; correctness always fails, metadata policy is `--in-ci`-gated. *(R9)*

`smp experiments validate` runs over the config dir and separates **correctness** from **policy**:

- **Correctness — always an error** (any caller, local or CI): duplicate experiment names,
  Leaf-XOR-Dir violations (a folder with both `cases/` and descendant `cases/`), a *present* `label`
  that ≠ `smp/<leaf-path>`, duplicate labels, an invalid `runner` value. These break a run
  *everywhere*, so they fail even during local iteration.
- **Metadata policy — warning by default, error under `--in-ci`**: a missing `mode` on a leaf, or a
  missing `label` on an `optional` / `codeowners` leaf. "Committed experiments carry metadata" is a
  repo policy, so the datadog-agent CI gate passes `--in-ci` to enforce it while local iteration and
  other repos see only warnings (see D14).
- A missing `description` is a **warning** regardless.
- **No homogeneity requirement** — a leaf may mix `kind`/`runner`; this is not an error (D13).

In v1 the gate runs with **`--exclude-path ebpf`**. This is distinct from the *run* exclusion of ebpf
(handled semantically by `runner: metal` + the container filter, D13): ebpf is simply **not yet
modeled** — it has a duplicate `tcp_rr` name across `ebpf/cases` and `ebpf/config-only/cases`, and
`ebpf` is a leaf that nests another leaf — so the gate skips it until ebpf is reorganized (see Open
questions). Temporary.

Rationale: correctness errors — especially duplicate names — must never merge and also matter locally,
so they always fail. Metadata *presence* is repo policy, gated by `--in-ci` so it never trips someone
iterating locally. (Lenient-default is not fail-safe — CI must pass `--in-ci` — but the gate is
version-controlled and a missing flag makes it visibly catch nothing, an acceptable one-time-setup
risk.)

### D11 — Two-level docs via README frontmatter; run instructions are derived, not hand-written. *(R8)*

- **Leaf folder README:** `description:` (group one-liner) + `mode:` + `label:` (for
  `optional` / `codeowners`) + optional prose body.
- **Experiment folder README:** `description:` (experiment one-liner) + optional prose body.

How-to-run instructions are **not** hand-written per folder. The convention — "to run a suite, apply
its `label` to the PR" — is documented **once** centrally, and `smp experiments list` (and the
add-experiment skill) surface the concrete label from the frontmatter. This avoids the same sentence
drifting across N READMEs.

Rejected: per-experiment *sections in the group README* — drift (sections must be hand-synced to
`cases/`) and fragile section-parsing.

### D12 — Reporting reuses the existing SMP report mechanism (v1). *(R9)*

Results are surfaced by the existing regression-detector report, and the run job logs the resolved run
set. v1 adds no new PR-native reporting surface; a richer one is deferred (see Open questions).

### D13 — Experiment `runner` and `kind` are axes orthogonal to selection. *(R7)*

Two attributes describe *what / where* an experiment is, independent of *whether* it is selected
(mode/label):

- **`kind`** — `regression` (compares two builds; manifest `experiment.yaml`) vs `config-only`
  (compares two configs of one build via baseline/comparison env vars; manifest
  `config-only-experiment.yaml`). **Inferred from the manifest** — never authored, to avoid a second
  source of truth. Determines which submit command runs the experiment (`submit-regression` vs
  `submit-experimental-config-only`).
- **`runner`** — the runner/target type the experiment must execute on (`container`, `metal`;
  extensible to new runner types later). **Explicit optional field in the experiment manifest** (`experiment.yaml` /
  `config-only-experiment.yaml`), manifest key `runner:`, default `container`; cannot be inferred.
  (Named `runner`, **not** `environment`, because `environment` is already the manifest's env-vars
  map.) It is per-experiment and intrinsic, so a **leaf may be heterogeneous** (mix runners / kinds).

`resolve` emits both and filters by `--runner`; the submit commands take an optional
`--runner` filter. **Routing (v1) is deliberately simple:** each submit command self-selects its
own kind — `submit-regression` processes only `regression` experiments, `submit-experimental-config-only`
only `config-only` ones, each skipping the other kind — so the run job resolves for
`runner: container` and just calls the appropriate command; no wrapper-side kind partitioning.
This keeps the design open: a future *unified* submit that discerns kind, and/or one job spanning
multiple runners, slot in with no change to folders, frontmatter, or `resolve` output.

(In v1, ebpf is additionally excluded by `--exclude-path ebpf` rather than by `runner`, because its
jobs stay on an older SMP whose manifests reject an unknown `runner:` key — see D14/D10.)

`runner` lives in the manifest (not the leaf README or `config.yaml`) because it is execution
config that `submit` already parses, and because it must resolve identically no matter which config-dir
root scans it — a folder/`config.yaml` default would be context-dependent (an ebpf experiment is metal
whether scanned from `test/regression` or `test/regression/ebpf`).

Rationale: metal (ebpf) and container both exist now, so the runner axis models present reality, not
speculation; new runner types are added as new values. Conflating runner with kind (treating
"config-only" as if it implied "metal") would bake today's coincidence into the model and break the
day container-config-only, metal-regression, or a new runner type appears.

### D14 — Sensible defaults; runtime is lenient, the CI gate enforces policy via `--in-ci`. *(R7, R9)*

Missing metadata never breaks the *runtime* paths: a leaf with no README / no `mode` defaults to
`mode: always`, and no `runner` defaults to `container`. So `local-run` and direct `submit` (both
mode-agnostic) and `resolve` all "just work" for a developer's one-off experiment — no README required
for local iteration.

`validate` is the separate CI *policy* gate (D10). Correctness errors (duplicate names, malformed
labels, Leaf-XOR-Dir, invalid runner) always fail — even locally. The *metadata-presence* checks
(missing `mode`; missing `label` on a `codeowners`/`optional` leaf) are warnings by default and become
errors only under **`--in-ci`**, which the datadog-agent CI gate passes. So local iteration and other
repos see warnings, never a hard failure for a not-yet-written README.

**Coupling to note:** because the default is `always`, an un-annotated ebpf leaf would default to
`always` + `container` (the default `runner`) and leak into the container PR flow. In v1 the ebpf jobs
stay on the *old* SMP, whose manifests `deny_unknown_fields`, so ebpf experiments **cannot** yet carry
a `runner:` key — the `runner: metal` + `--runner container` exclusion is therefore **not available in
v1**. So v1 excludes ebpf from the main flow by **`--exclude-path ebpf`** on `resolve`/`submit` (and on
`validate`, D10). Thus `{default mode = always}` + `{`--exclude-path ebpf` on the main job}` are a
**single coupled change** — shipping the default without the exclude would run ebpf on every PR. The
`runner: metal` semantic exclusion activates when ebpf migrates to the new SMP (see Open questions).

Rationale: defaults give runtime resilience and local ergonomics; strict `validate` gives repo policy.
They are different tools for different callers and do not conflict — local dev never invokes
`validate`.

## Consequences

**Positive:**

- No custom PR surface to build or maintain — selection uses GitHub's native label UI.
- Selection persists natively across pushes/reruns (D4/D6), with no additional state to manage.
- PR-only determinism, testable in-PR, no main dependency (D5/D6/D7).
- The hard gate can't be silently disabled — involvement is computed, not labeled (D5).
- Correctness (name collisions, structure, metadata) is enforced before merge (D10).
- Reusable, offline core; CODEOWNERS/GitHub stay in the wrapper (D8).
- The label set is small and deliberate (D4) — no sprawl.
- The `runner` axis (D13) keeps the design open to metal, further runner types, and a future
  single-job-across-runners submission with **no** folder or contract change.
- Sensible defaults + lenient runtime (D14) keep local dev and one-off experiments frictionless.

**Costs / risks:**

- **No live "what will run" preview** before the job runs. The resolved set is visible from the job
  logs / report, not up-front on the PR. (Accepted for v1.)
- Adding a selectable folder is a **two-step** action — create the label *and* set frontmatter;
  mitigated by `validate` (D10) checking the two stay in sync.
- `codeowners` involvement is **not surfaced on the PR** until the job runs (no auto-label). A
  deferred cosmetic auto-label would address this.
- Rerun is **manual replay** (D9), not one-click.
- Default `mode: always` (D14) means an un-annotated committed leaf **runs by default**; cost-control
  is opt-in via `optional`/`codeowners`. Made safe by the strict `validate` gate (D10) requiring
  explicit metadata in-repo.
- In v1, ebpf is excluded from the main flow by `--exclude-path ebpf` (its jobs stay on the old SMP,
  which rejects an unknown `runner:` key); the cleaner `runner`-based exclusion is deferred to the
  ebpf migration (D14, Open questions).

## Alternatives considered

- **Single multi-writer control-plane comment (checkboxes + status + reports) → dropped.** One
  comment edited by the generator, run job, and preview job must merge sections without ever
  clobbering the user's ticks — brittle. GitHub's native label UI gives persistent per-PR selection
  for free. The custom comment surface is *deferred*, not deleted (Open questions).
- **Auto-applied labels (via `actions/labeler` path rules or a CI step) → deferred.** Path-rule
  auto-labeling would duplicate CODEOWNERS ownership as a second path→label map (drift), and
  auto-creation risks label sprawl. The repo's existing `actions/labeler` also carries a known
  `sync-labels: true # currently doesn't work` caveat. v1 reads involvement directly from CODEOWNERS
  (single source) and keeps labels manual.
- **Labels as the source of truth for the hard gate → rejected (D5).** A gate you can forget to label
  isn't a gate; involvement must be computed.
- **Reusing the existing `team/<name>` labels for selection → rejected.** Those labels are
  auto-applied from changed-files ∩ CODEOWNERS (`assign_team_labels`, gated to teams in
  `.ddqa/config.toml`) and already own other meaning (review routing / triage / release notes).
  Rejected on both jobs: (a) as a `codeowners` *trigger* they are redundant with computed involvement
  (D6) and strictly worse — they add a dependency on a separate async labeler workflow, cover only
  `.ddqa`-listed teams, and are hand-editable, which reintroduces the exact "gate you can turn off"
  failure D5 avoids; (b) as an `optional`/`codeowners` *selector* they are too coarse (a team owns
  many folders, so a per-team label can't pick a folder) and carry ownership semantics, not an
  opt-in. Labels are kept **intentionally scoped to SMP** (`smp/<folder>`). The `team/*` labels are
  useful only as a pre-existing *cosmetic* involvement indicator (see Open questions).
- **team-gate trigger: requested-reviewer → changed-files (D6).** Requested-reviewer depends on the
  *base branch's* CODEOWNERS (GitHub evaluates auto-requests against main), which breaks in-PR
  testing (R5). Changed-files uses PR-HEAD CODEOWNERS on both sides.
- **per-experiment modes → per-folder modes (D1).** See D1 rationale.
- **CODEOWNERS parsing inside `smp` → CI wrapper (D8).** CODEOWNERS is repo-specific; the CLI must
  stay cross-repo.
- **Comment-state persistence (read-modify-write) → native label persistence (D4).** Labels persist
  on the PR with no merge logic, making the old persistence machinery unnecessary.
- **Path-based ebpf exclusion vs runner filtering.** The *target* is `runner: metal` + a container
  filter (expresses intent, generalizes to further runner types). But v1 keeps the ebpf jobs on the old SMP, whose
  manifests reject unknown fields, so ebpf can't carry `runner:` yet — v1 therefore excludes ebpf by
  `--exclude-path ebpf` on `resolve`/`submit` **and** `validate`. The runner-based exclusion replaces
  the path-exclude when ebpf migrates (Open questions).
- **Default mode `optional` vs `always` → chose `always` (D14).** `optional`-default is more
  conservative (nothing runs until declared) but counterintuitive for local dev ("why isn't my
  experiment running?"). `always`-default matches intuition and is made safe by the strict `validate`
  gate plus runner-scoped runs.
- **Authoring `kind` in frontmatter → rejected; inferred from the manifest (D13).** A written `kind`
  would be a second source of truth that can drift from the actual `experiment.yaml` /
  `config-only-experiment.yaml`.
- **Coupling runner and kind (e.g. "config-only" ⇒ "metal") → rejected (D13).** They are
  independent axes; coupling them breaks when container-config-only / metal-regression / a new runner appears.

## Open questions / future work

- **Richer PR-native control/report surface ("real UI").** If labels prove too coarse or invisible,
  revisit a custom comment or app for surfacing selection and results.
- **Cosmetic visibility of `codeowners` involvement on the PR** before the job runs. The existing
  `team/<name>` labels (auto-applied from CODEOWNERS involvement) already serve this for free, so a
  dedicated auto-applied `smp/*` visibility label is likely unnecessary; if a gap remains, a
  best-effort apply via the run job's API access is the fallback (not path rules).
- **Job discoverability for rerun (R4).** Retrying the SMP job (D9) assumes you can *find* it in the
  GitLab pipeline, which is awkward today. Candidate: augment the existing regression report to
  include a **direct link to the SMP job**, so it can be located and retried in one hop. Deferred —
  a report enhancement, not core to selection.
- **Direct-from-PR one-click trigger** (label/slash-command → GHA → GitLab) — needs a GitLab
  credential in GHA; codeowner/policy-gated. There is **no in-repo prior art** for triggering a
  GitLab pipeline from GitHub, so this is net-new infra, not a reuse of an existing pattern.
- **Smarter "relevance" than CODEOWNERS** (R2 future).
- **Multi-owner experiments:** define `codeowners` as "runs if *any* owning team is involved."
- **Config-only as a first-class *selectable* kind.** v1 keeps config-only a separate nightly track
  (ebpf metal). Making it PR-selectable needs `--experiment-path-filter` on
  `submit-experimental-config-only` and `kind` emitted by `resolve` so the wrapper can route it.
  Deferred until a non-ebpf team wants a selectable config-only suite.
- **ebpf reorganization into the model.** Today ebpf is excluded from `validate` (D10) because it has
  a duplicate `tcp_rr` name and `ebpf` is a leaf nesting a leaf. Reorganizing ebpf into a Dir with
  typed leaves (e.g. `ebpf/<regression-leaf>/cases`, `ebpf/config-only/cases`) and de-duping names
  would let it drop the `--exclude-path ebpf`. Migration also moves the ebpf jobs onto the new SMP (so
  their manifests can carry `runner: metal`) and switches the main-flow exclusion from
  `--exclude-path ebpf` to the `runner` filter. Coordinate with ebpf-platform.
- **Single job spanning multiple runners.** An SMP submission-layer capability; the
  `runner` axis (D13) is the forward-compatible input, so no folder/contract change is needed
  when it lands. The wrapper's per-runner fan-out collapses at that point.

**Decided (was open):**
- **Mode naming** — the always-run mode is **`always`** (renamed from `quality_gates`). The suite
  identity stays in the folder name `quality_gates/` and the `quality_gate_*` experiment names.
- **Label naming convention** — `smp/<full-path>`, mirroring the folder tree (e.g. `smp/logs/syslog`);
  `validate` enforces `label == smp/<leaf-path>` and uniqueness.
