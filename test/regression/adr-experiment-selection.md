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
distinguishable from the repo's other labels. An applied PR label that isn't a manifest key is
ignored. Two checks enforce this (D9).

### D5 — `codeowners` involvement is path-derived (unchanged); no team in the manifest.

The `codeowners` bucket only marks experiments as involvement-gated. *Which* team owns an experiment
is still computed from **changed-files ∩ CODEOWNERS on PR HEAD** for the experiment's path — CODEOWNERS
stays the single ownership source. Involved-teams and the folder→owner map are **inputs** to `resolve`
(the CLI never parses CODEOWNERS or touches GitHub).

### D6 — `runner` and `kind` are intrinsic; the manifest never mentions them.

`runner` (`container` | `metal`) is an authored field in `experiment.yaml`; `kind`
(`regression` | `config-only`) is inferred from the manifest filename. The manifest *selects*
experiments; `runner`/`kind` determine *how/where* each selected experiment is submitted: `resolve`
filters the run set by the job's `--runner`, and each submit command self-filters its `kind`. One
manifest serves all runner-jobs (each takes its `--runner` slice) — the seam for cross-runner suites.

### D7 — Folders are pure organization (Leaf-XOR-Dir dissolved).

With selection out of the folders, there is no "leaf" selection unit and no homogeneity requirement.
Discovery just finds experiments under any `cases/` dir at any depth. Teams organize folders however
they want; folders still feed CODEOWNERS ownership (for the `codeowners` bucket) and are the natural
home for optional docs (READMEs).

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
- **Label-sync CI job** (online, GitHub API — a GHA, not the CLI): the manifest's `labels:` keys ↔ the
  repo's `smp/*` labels must match (no orphan repo labels; every manifest label exists to be applied).

### D10 — SMP↔CI boundary.

- **CI wrapper (repo-specific):** resolve the PR, mint a read token, diff changed files → involved
  teams, build the folder→owner map (CODEOWNERS), read applied labels, and submit the resolved set.
- **`smp` CLI (reusable, offline):** read the manifest, discover experiments, resolve the union run
  set (filtered by `--runner`), validate. Involved-teams, ownership, labels, and the manifest are
  inputs; CODEOWNERS/GitHub never enter the CLI.

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
- Labels are now free-form registry keys (the tidy `label == smp/<leaf-path>` rule is gone), so the
  registry check is load-bearing.

## Alternatives considered

- **Per-folder mode/label in leaf READMEs (the prior model).** Simple and local, but forces
  folder layout to encode selection (homogeneous leaves), caps an experiment at one label, and can't
  express cross-folder suites. Kept as a working reference; superseded here.
- **Per-experiment mode/labels in `experiment.yaml`.** Gets mixed-mode/multi-label/cross-folder too,
  without a central file — but violates the litmus test (puts CI-selection constructs into the
  experiment definition) and gives no registry/audit surface. Rejected on separation-of-concerns.
- **Per-area manifests instead of one global file.** Scopes contention + self-serve ownership, but
  cross-team suites have no home and there's no single registry/audit. Rejected in favor of one global
  file for full flexibility.
- **Encoding the folder tree in the manifest.** Brittle on moves (drifts from disk). Rejected — the
  manifest is trigger→(globs|names); structure is discovered from the filesystem.

## Open questions / future work

- **ebpf / config-dir unification.** ebpf (metal) runs on separate jobs with their own `CONFIG_DIR`
  and is excluded from this flow (`--exclude-path ebpf`) until we unify the config dir and bring
  metal onto the manifest.
- **Single job spanning multiple runners.** The `runner` axis is the forward-compatible input; today
  each runner-job takes its `--runner` slice.
- **Labels-as-code.** The label-sync check (D9) could graduate from *checking* to *syncing* (create /
  prune repo `smp/*` labels from the manifest).
- **Scheduled label-sync.** The GHA runs on manifest changes; out-of-band repo-label drift would need
  a scheduled run.
