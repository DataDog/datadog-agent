# ADR: SMP regression experiment selection

**Status:** Proposed (declarative-manifest model)

## Context

SMP regression experiments in `test/regression/` historically ran as a fixed suite. We want teams to
(a) keep optional experiment suites in-repo and run them on demand, (b) have the *right* experiments
run automatically when code they own changes, and (c) organize experiment folders however they like
without the folder layout dictating *when* an experiment runs. As much logic as possible belongs in
the reusable `smp` CLI rather than CI glue.

This ADR records the **declarative-manifest** model: a single file, `test/regression/selection.yaml`,
maps **trigger buckets → experiments**. Selection metadata is deliberately kept *out* of the
experiment definitions and folder structure.

Guiding principle — the litmus test **"does `local-run` care?"** partitions every attribute:
- **Intrinsic** (yes → lives with the experiment): `runner` (in `experiment.yaml`), `kind` (inferred
  from the manifest filename). `local-run` needs these to run an experiment at all.
- **CI-selection** (no → lives in the manifest): whether an experiment is always-run, ownership-gated,
  or label-gated. Meaningless outside CI, so it does not belong in the experiment definition.

## Decisions

### D1 — Selection lives in a central manifest, not in folders or experiment definitions.

`test/regression/selection.yaml` is the single source of selection policy, passed to the CLI via
`--manifest`. Rationale: (a) separation of concerns — experiment definitions stay CI-agnostic and
`local-run`-friendly; (b) the manifest doubles as the **label registry** (a label is valid iff it's a
key here), which bounds label sprawl; (c) it enables suites that span teams/folders, which a
per-folder scheme fundamentally cannot express. The file is **global and SMP-owned** (via CODEOWNERS)
— accepted as the governance point that keeps the policy coherent and labels bounded, at the cost of
SMP reviewing selection changes.

Experiment *content* ownership (distinct from the manifest) follows CODEOWNERS by folder:
`quality_gates/` experiments are **co-owned by SMP** (they are the core, always-run performance
claims), while every other experiment folder is owned **solely by its team** (e.g. `logs/` →
`agent-log-pipelines`). This is what makes the `codeowners` bucket's involvement resolve to the
right team (D5).

### D2 — Three trigger buckets, unioned.

```yaml
always:            [ <glob | path | name>, ... ]   # unconditional
codeowners:        [ <glob | path | name>, ... ]   # runs when the experiment's owner is involved
labels:
  smp/<label>:
    description:    <optional one-liner>
    experiments:   [ <glob | path | name>, ... ]
```

An experiment runs if it matches `always`, **OR** it matches `codeowners` **and** its CODEOWNERS
owner is in the PR's involved teams, **OR** any label listing it is applied to the PR. Membership is a
*set of triggers* — the old single per-experiment "mode" is gone. An experiment may appear in several
buckets (union; harmless).

### D3 — Entries are standard globs, exact paths, or names.

Entries use **standard glob semantics** (`*` = one path segment, `**` = any depth — no new dialect),
matched against experiment paths relative to `test/regression`; or an exact experiment path; or a
bare **experiment name** (globally unique → pinned, survives moves). Because experiments live under a
`cases/` dir, "all logs experiments" is `logs/**`. Globs are location-coupled (rebind on move);
names are pinned — authors choose per entry. `resolve` expands entries against discovered experiments
and emits concrete `--experiment-path-filter` values to `submit`, so the manifest's glob syntax is
independent of the submit filter.

### D4 — Labels are a registry; keep the `smp/` prefix.

A label is valid **iff** it's a key under `labels:`. Keys keep the `smp/` prefix so SMP labels are
distinguishable from the repo's other labels; the prefix is configurable via `validate --label-prefix`
(default `smp/`). An applied PR label that isn't a manifest key is ignored. Two checks enforce this
(D9). Note the prefix is a `validate`-only concern — `resolve` matches applied `--label` values
**literally** against manifest keys, so it has no prefix flag.

### D5 — `codeowners` involvement is path-derived (unchanged); no team in the manifest.

The `codeowners` bucket only marks experiments as involvement-gated. *Which* team owns an experiment
is still computed from **changed-files ∩ CODEOWNERS on PR HEAD** for the experiment's path — CODEOWNERS
stays the single ownership source. Involved-teams and the experiment→owner map are **inputs** to `resolve`
(the CLI never parses CODEOWNERS or touches GitHub).

### D6 — `runner` and `kind` are intrinsic; the manifest never mentions them.

`runner` (`container` | `metal`) is an authored field in `experiment.yaml`; `kind`
(`regression` | `config-only`) is inferred from the manifest filename. The manifest *selects*
experiments; `runner`/`kind` determine *how/where* each selected experiment is submitted: `resolve`
filters the run set by the job's `--runner`, and each submit command self-filters its `kind`. One
manifest serves all runner-jobs (each takes its `--runner` slice) — the seam for cross-runner suites.

### D7 — Folders are pure organization; any depth is allowed.

With selection out of the folders, there is no per-folder selection unit and no homogeneity
requirement. Discovery finds experiments under any `cases/` directory at any depth, so
`logs/cases/…`, `logs/general/cases/…`, and `logs/syslog/cases/…` can coexist and each be addressed
independently by a path-prefix glob (`logs/**` = all of them; `logs/general/**` = just that group).
Teams organize experiment folders however they want; folders still feed CODEOWNERS ownership (for the
`codeowners` bucket) and are the natural home for optional docs (READMEs).

### D8 — Defaults: runs by default, but must be bucketed to merge.

An experiment matched by **no** bucket defaults to `always` at **runtime** (it runs, so a
newly-added experiment isn't silently skipped during development). But `validate --in-ci` treats an
unmatched experiment as a **policy error** (blocking), so it must be placed in a bucket **to merge**.
Authors get "add it and it runs" locally/in-PR, plus enforced bucketing at the merge gate.

### D9 — Two checks: offline `validate` + online label-sync.

- **`smp experiments validate --manifest … [--in-ci]`** (offline): *correctness always errors*
  (duplicate experiment names; entries resolving to nothing; malformed/non-`smp/` labels; invalid
  `runner`); *policy* (unmatched experiment) warns by default, errors under `--in-ci`. It does **not**
  warn on missing descriptions (docs are optional now).
- **Label-sync CI job** (online, GitHub API — a GHA, not the CLI): reports drift between the manifest's
  `labels:` keys and the repo's `smp/*` labels, emitting a copy-paste `gh label create`/`delete`
  command per mismatch. It never mutates labels (create/delete is manual — see the labels-as-code
  note). Enforcement is **asymmetric**: a manifest label with **no repo label only warns** (benign,
  self-correcting — the trigger is dormant until created; after creating it, re-run the check, no
  manifest edit needed), while a repo `smp/*` label **absent from the manifest blocks** (misleading
  cruft that would otherwise accumulate — the block forces cleanup). Fix an orphan by deleting the
  repo label or declaring it in the manifest.

### D10 — SMP↔CI boundary.

- **CI wrapper (repo-specific):** resolve the PR, mint a read token, diff changed files → involved
  teams, build the experiment→owner map (CODEOWNERS), read applied labels, and submit the resolved set.
- **`smp` CLI (reusable, offline):** read the manifest, discover experiments, resolve the union run
  set (filtered by `--runner`), validate. Involved-teams, ownership, labels, and the manifest are
  inputs; CODEOWNERS/GitHub never enter the CLI.

### D11 — CI contract: the `--ownership` map.

`resolve` consumes ownership as an input (D5, D10); this pins its shape so the CLI and the CI wrapper
**provably** agree — a mismatch here silently under-selects `codeowners` experiments.

- **Shape.** `--ownership` is a JSON object mapping an **experiment path** (relative to
  `--target-config-dir`, exactly as emitted by `smp experiments list`) to its owning team slugs:

  ```json
  { "logs/general/cases/logs_general": ["agent-log-pipelines"],
    "quality_gates/cases/quality_gate_security_idle": ["agent-security", "single-machine-performance"] }
  ```

  Keying by **experiment path** (not by the group folder) means **per-experiment CODEOWNERS overrides
  are honored** — a co-owned experiment inside an otherwise SMP-owned folder gets its real owners —
  while folder-level delegation still works via normal CODEOWNERS precedence (a folder rule matches
  every experiment path under it). It also needs **no path-derivation on either side** (both hold the
  verbatim path), which removes an entire class of key-mismatch drift.

- **Values.** Team slugs normalized from CODEOWNERS (`@DataDog/x` → `x`), **lowercased**, teams only
  (individual owners dropped, so a value may be `[]`). The map is an **already-resolved lookup table** —
  CODEOWNERS globs were evaluated by the wrapper — so the CLI does **exact-key lookup** by experiment
  path, never glob/prefix matching, and compares team slugs case-sensitively on the lowercased forms.

- **Selection semantics.** A `codeowners`-bucketed experiment is selected iff
  `ownership[experiment_path] ∩ involved_teams ≠ ∅` (a co-owned experiment fires on *any* co-owner's
  involvement). If the path is absent from the map or maps to `[]`, the experiment has **no known
  owner** → never selected by involvement. Because the repo's catch-all `/test/regression/ → SMP` rule
  gives every path at least one team, an *absent* path signals **contract drift** and SHOULD produce a
  resolve-time warning (non-fatal — resolve stays robust; see D10's fallback).

## Consequences

**Positive:**
- Experiment definitions stay CI-agnostic; folders are free to organize any way (D1, D7).
- Cross-team / cross-folder suites are expressible; mixed-mode and multi-label are natural (D2).
- The manifest is a single audit surface and the label registry (bounds sprawl) (D1, D4).
- Reusable, offline CLI core; runner/kind seam keeps multi-runner open (D6, D10).

**Costs / risks:**
- **Central contention / ownership** — one SMP-owned file every team edits; merge-conflict hotspot and
  an SMP review bottleneck. Accepted for flexibility + governance.
- **Drift** — manifest vs disk (stale entries / unmatched experiments) and manifest vs repo labels;
  handled by `validate` + the label-sync job (D9), not free.
- **Locality** — an experiment's selection isn't visible at the experiment; you consult the manifest
  (`smp experiments list` can render resolved triggers per experiment).
- Labels are now free-form registry keys (the tidy `label == smp/<folder-path>` rule is gone), so the
  registry check is load-bearing.

## Design axes: granularity × storage (why central manifest)

Two *independent* questions drove this design. Separating them shows why a central manifest isn't the
only obvious choice — and why it won.

**Axis 1 — granularity: at what level are mode/labels defined?**
- *Per folder* (the prior model, still live as the v2 reference): one mode/label per experiment folder,
  experiments inherit. Simple, and **labels are derivable from the folder path** — but folders must be homogeneous
  (mode becomes a folder-organizing dimension), an experiment can carry only **one** label, and you
  **cannot** express a suite that spans folders.
- *Per experiment*: each experiment carries its own mode/labels. Unlocks **mixed-mode folders**,
  **multi-label** (an experiment in several suites), and near-total folder freedom (an implicit
  ownership-by-path relationship still remains). Cost: labels are **no longer derivable** from a path,
  so they need a registry + validation to avoid sprawl.

**Axis 2 — storage: where is that expressed?**
- *Distributed* (with each experiment): local, no central contention, ownership follows the code — but
  no single place to see/audit the whole policy, and no natural label registry.
- *Central* (one manifest): one place to read/edit the whole policy; it **doubles as the label
  registry**; and it's the **only** way to express a cross-team/cross-folder suite. Costs: a
  merge-conflict hotspot on one file; and because CODEOWNERS involvement is path-derived, **moving an
  experiment can change its `codeowners` behavior without touching the manifest** (ownership follows
  the folder — intended, but a subtlety to know).

**What we chose, and the insight that decouples the axes.** A **central manifest grouped by trigger**
(`always` / `codeowners` / `labels:` buckets), with experiments referenced by glob or name. Because an
experiment can appear in **multiple buckets**, this delivers the *per-experiment granularity benefits*
(mixed-mode + multi-label + cross-folder suites) **without** the *per-experiment storage cost* — no
CI-selection fields ever land in `experiment.yaml`. That separation matters (the Context litmus test):
`mode`/`labels` are CI concepts with no meaning to `local-run` or to an experiment reused outside CI.
Grouping **by trigger** (not by experiment key) is exactly what lets any experiment be assigned to any
set of triggers.

**Why not the alternatives:**
- **Per-experiment mode/labels in `experiment.yaml`** — same granularity, but it puts CI-only fields
  into the experiment definition (what does `mode: optional` mean to `local-run`?) and offers no
  registry/audit surface. Rejected on separation of concerns.
- **Per-area manifests (one per team) instead of one global file** — scopes merge contention and gives
  self-serve ownership, but cross-team suites have no home and there's no single registry/audit.
  Rejected in favor of one global file for full flexibility (accepting SMP as the gatekeeper).
- **Encoding the folder tree in the manifest** — brittle: it drifts from disk on every move and would
  need auto-regeneration. Rejected. Entries are `trigger → (globs | names)`; **structure is discovered
  from the filesystem** (`smp experiments list`).

**Costs we accept, and how they're handled:**
- *Labels not derivable* → the manifest's `labels:` keys **are** the registry; offline `validate`
  checks they're well-formed (+ `smp/` prefix), and an online CI job keeps them in sync with the
  repo's GitHub labels.
- *Merge conflicts on one file* → accepted; the file is SMP-owned, which is also the governance point
  that keeps the label set coherent.
- *CODEOWNERS path-coupling* → a bare glob/name in the manifest doesn't pin ownership, so moving an
  experiment changes its `codeowners` involvement even with the manifest untouched. Intended
  (ownership follows location), documented as a subtlety.
- *Readability / "does the manifest mirror the folder tree?"* → **No.** It's organized **by trigger**
  ("what runs when"), and adding an experiment usually needs **no manifest edit** if it falls under an
  existing glob (e.g. `logs/**`) — only a *new* suite/label needs an entry.

## Open questions / future work

- **ebpf / config-dir unification.** ebpf (metal) runs on separate jobs with their own `CONFIG_DIR`
  and is excluded from this flow (`--exclude-path ebpf`) until we unify the config dir and bring
  metal onto the manifest.
- **Single job spanning multiple runners.** The `runner` axis is the forward-compatible input; today
  each runner-job takes its `--runner` slice.
- **Labels-as-code.** Label creation is deliberately **manual** for now and the foreseeable future
  (the D9 label-sync job only *checks* that manifest keys and repo labels agree). A possible future
  path — not planned — is graduating that job from *checking* to *additive syncing*: auto-create
  repo `smp/*` labels from new manifest keys, on merge to `main`, but **never** auto-prune (deleting a
  label strips it from every PR/issue that had it — surface removals as a warning instead). This would
  live in the GHA (an online, write-scoped op), keeping the offline CLI unchanged.
- **Scheduled label-sync.** The GHA runs on manifest changes; out-of-band repo-label drift would need
  a scheduled run.
- **Selective review automation.** `selection.yaml` is SMP-owned (CODEOWNERS), so every manifest change
  currently needs SMP review. SMP's real concern is the `always` block (the unconditional suite). If
  the review burden grows, we could automate approval of manifest diffs that **don't touch `always`**
  (i.e. changes confined to `codeowners`/`labels` entries), reserving human SMP review for `always`
  changes.
