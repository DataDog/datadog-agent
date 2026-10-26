# GenSim global 1 Hz experiment tooling

## Retrieval analyzer

`analyze_llmobs_metric_retrieval.py` converts a complete Bits LLMObs trace export into deterministic per-call metric-retrieval evidence. It does not invoke a model or judge.

The analyzer records:

- omitted versus explicit request fields;
- query window and requested interval;
- binned, raw, no-data, or unparseable response format;
- returned effective interval, point and value counts, nulls, reconstructed last timestamp, and full-window point-count gaps;
- requested-versus-effective interval classification;
- truncation and no-data markers;
- conservative, policy-driven source eligibility; and
- whether later trace text mentions the metric and its resolution. This last field is instrumentation, not proof that the evidence caused the conclusion.

Eligibility is never guessed from cadence alone. Use a reviewed policy file tied to the scenario manifest. `episode_2483_metric_policy.json` recognizes only the qualified enqueue metric and excludes APM trace statistics; unmatched sources remain unknown.

## Run

```bash
python3 tools/gensim_global_1hz/analyze_llmobs_metric_retrieval.py \
  TRACE.json \
  --policy tools/gensim_global_1hz/episode_2483_metric_policy.json \
  --output RETRIEVAL.json
```

The output includes the input path and SHA-256. Preserve it beside the immutable trace rather than replacing the trace.

## Test

```bash
python3 -m unittest -v \
  tools/gensim_global_1hz/analyze_llmobs_metric_retrieval_test.py
```

The implementation uses only the Python standard library. Raw point values are processed in one pass; output contains counts and metadata, not duplicated point arrays.

## Outcome-blind corpus refresh and freeze

`refresh_outcome_blind_corpus.py` reads the current GS Admin archived catalog, requires latest-bundle CausalGen quality, pins current source identities, and writes:

- `docs/dev/gensim-global-1hz-classification-baseline.json` — the 96-record bundle/version/hash guard for the human source classification;
- `docs/dev/gensim-global-1hz-corpus-freeze.json` — the deterministic 15-scenario 7/3/2/3 score panel and exact source archive hashes;
- `docs/dev/gensim-global-1hz-corpus-rejection-ledger.json` — all 96 admission and selection decisions;
- `docs/dev/gensim-global-1hz-dataset-manifest.json` — outcome-blind membership and scoring policy;
- `docs/dev/gensim-global-1hz-future-candidate-catalog.md`; and
- `docs/dev/gensim-global-1hz-corpus-matrix.md`.

The one-time bootstrap binds the archived human taxonomy to the current bundle identities. It is allowed only after a source review and before treatment outcomes are inspected:

```bash
python3 tools/gensim_global_1hz/refresh_outcome_blind_corpus.py \
  --archived-catalog docs/dev/archive/gensim-global-1hz/2026-08-25/gensim-global-1hz-future-candidate-catalog.md \
  --bootstrap-classifications \
  --output-dir docs/dev
```

Every later refresh must provide the baseline. A changed version, archive key, manifest, canonical episode hash, or passed-ID set fails closed and requires a new static source review:

```bash
python3 tools/gensim_global_1hz/refresh_outcome_blind_corpus.py \
  --archived-catalog docs/dev/archive/gensim-global-1hz/2026-08-25/gensim-global-1hz-future-candidate-catalog.md \
  --classification-baseline docs/dev/gensim-global-1hz-classification-baseline.json \
  --output-dir docs/dev
```

Validate structural admission independently from execution readiness:

```bash
python3 tools/gensim_global_1hz/freeze_corpus.py \
  docs/dev/gensim-global-1hz-corpus-freeze.json

python3 tools/gensim_global_1hz/freeze_corpus.py --require-ready \
  docs/dev/gensim-global-1hz-corpus-freeze.json
```

The strict command must fail until every panel capability smoke, exact content hash, current-panel judge calibration, and owner approval are recorded. Null, negative, and regressive semantic outcomes never authorize membership replacement.

### No-run source readiness audit

`audit_source_readiness.py` audits exact local archives without contacting GS Admin, Kubernetes, Datadog, a registry, or a model. It verifies archive and canonical episode hashes, safe tar structure, detailed source-authored ground truth, required bundle files, current Agent adapter compatibility, canonical archive RCA-chain shape, and whether judge fixtures are bound to the same immutable source identity. Missing archives remain `unknown`; the tool never substitutes another version.

```bash
python3 tools/gensim_global_1hz/audit_source_readiness.py \
  --freeze docs/dev/gensim-global-1hz-corpus-freeze.json \
  --classification-baseline docs/dev/gensim-global-1hz-classification-baseline.json \
  --judge-cases docs/dev/gensim-global-1hz-judge-calibration-cases.json \
  --archive-dir "$HOME/episode-bundles/global-1hz-corpus-freeze-sources" \
  --archive-dir "$HOME/episode-bundles" \
  --output /new/local/source-readiness-audit.json
```

The output path must not already exist. `static_execution_ready` covers only source/archive/Agent preparation; it does not imply cluster capacity, deployment, telemetry, Bits/RCA, publication, or treatment-effect readiness. A valid judge fixture must pin the current bundle version, archive key, manifest SHA-256, and canonical episode SHA-256.

Source-grounded drafts for current panel episodes 2574, 2497, 2500, and 2601 are retained in `docs/dev/gensim-global-1hz-judge-calibration-drafts.json`. They are derived from exact current canonical episode YAML and deliberately remain separate from the active calibration library until both the scenario curator and judge owner approve them. Draft presence does not satisfy strict readiness.

Run all tests:

```bash
python3 -m unittest discover -s tools/gensim_global_1hz -p '*_test.py'
```

## Historical four-scenario pilot tooling (retired)

The following tools consume the archived pre-outcome-blind pilot manifest. They are retained for provenance and reproduction only; do not use them to construct the current score panel or a current managed launch.

`build_pilot_bundles.py` derives matched local-GKE A/B bundle trees from
immutable local sources. It preserves the source chart's Agent topology and is
therefore intended for owner-executed local-GKE `episode-ctl` pilot runs. The
tool never contacts Kubernetes, gs-admin, a registry, or a Datadog API.

The checked-in pilot manifest selects four complementary source-pinned cases:
2483 (DogStatsD count), 2481 (recurring Agent check), 2643 (metric-neutral),
and 2674 (log/trace-led). It carries each source tar and canonical episode-YAML
hash, and pins the custom Agent image.

```bash
python3 tools/gensim_global_1hz/build_pilot_bundles.py \
  --manifest docs/dev/archive/gensim-global-1hz/2026-08-25/gensim-global-1hz-pilot-manifest.json \
  --output "$HOME/episode-bundles/global-1hz-pilot-bundles-v1"
```

The output directory must not exist. The compiler refuses to overwrite it,
verifies the canonical source episode hash, requires exactly one `datadog-agent`
container image and one `KUBERNETES=yes` environment entry, copies each source
into A and B, replaces the Agent image in both, and inserts only
`DD_METRIC_RESOLUTION_EXPERIMENT_ENABLED=false|true` as the arm difference. It
writes `pilot-bundle-manifest.json` with source and derived tree hashes and
fails if another file differs between A and B.

These generated bundles are **not gs-flow/Terrapin-ready**: the current source
charts use the GKE container-runtime socket, while nested k3s needs a separately
reviewed CRI socket, host-mount, and Agent-topology adapter. Exact deployed-worker
version pins remain a separate confirmatory-scale concern. For this local-GKE
design pilot, run the output bundles directly with owner-executed `episode-ctl`
commands.

## Generated-episode managed fork plan

`build_managed_episode_fork_plan.py` prepares a local, **no-write** GS Admin
publication plan for an incident-generated frozen source that has no
hand-authored `gensim-apps` source. It is not an uploader and never constructs a
remote client. It verifies that the pilot/source manifests agree on the frozen
source and canonical episode hashes, hashes supplied immutable blueprint and
managed arm-bundle artifacts, requires distinct control/treatment destination
names, and records the owner-approved append-only operations plus required
readback fields.

The existing episode remains immutable. The user executes any approved GS Admin
writes separately; never use this tool to infer that publication occurred.

```bash
python3 tools/gensim_global_1hz/build_managed_episode_fork_plan.py \
  --pilot-manifest docs/dev/archive/gensim-global-1hz/2026-08-25/gensim-global-1hz-pilot-manifest.json \
  --source-manifest "$HOME/episode-bundles/global-1hz-corpus-freeze-sources/source-manifest.json" \
  --episode-id 2483 \
  --output-group global-1hz-eval \
  --control-name global-1hz-2483-control \
  --treatment-name global-1hz-2483-treatment \
  --blueprint /immutable/episode-2483-blueprint-v1.tar \
  --control-bundle /immutable/episode-2483-control.tar \
  --treatment-bundle /immutable/episode-2483-treatment.tar \
  --output /new/local/episode-2483-fork-plan.json
```

The required blueprint and managed-bundle tar artifacts must be supplied only
after their source/readback and managed-render contracts are proven. The output
path must not already exist.

For the first 2483 smoke only, build the pre-publication managed arm tars with:

```bash
python3 tools/gensim_global_1hz/build_managed_smoke_bundles.py \
  --manifest docs/dev/archive/gensim-global-1hz/2026-08-25/gensim-global-1hz-pilot-manifest.json \
  --episode-id 2483 \
  --output /new/local/episode-2483-managed-smoke-arms
```

This builder is intentionally not a generic nested-k3s adapter and does not
publish. It requires the frozen DaemonSet, CRI socket declaration, and
`/var/log/pods` mount to remain present, changes only the Agent digest and arm
toggle, and writes deterministic `control.tar` and `treatment.tar` artifacts.

Test it with:

```bash
python3 -m unittest -v tools/gensim_global_1hz/build_managed_episode_fork_plan_test.py
```

### Owner-run append-only publication

`publish_managed_episode_fork.py` is the only owner-run helper for this
publication path. It defaults to a local-only dry run, validates the plan and
all local artifact hashes without importing the GS Admin client, and performs
no network activity:

```bash
python3 tools/gensim_global_1hz/publish_managed_episode_fork.py \
  --plan "$HOME/episode-bundles/global-1hz-managed-smoke-2483-v1/episode-2483-fork-plan.json"
```

Only the owner may execute the authenticated write. Run the approved GS Admin
Python environment through `dd-auth`, require `--apply`, and choose a new
readback path. The command refuses pre-existing derived identities, source
artifact hash drift, existing destination artifacts, archive hash drift after
publication, and overwrite of the readback file. It never mutates source
episode 2483 or starts an execution.

```bash
dd-auth --domain dd.datad0g.com -- <approved-gsadmin-python> \
  tools/gensim_global_1hz/publish_managed_episode_fork.py \
  --plan "$HOME/episode-bundles/global-1hz-managed-smoke-2483-v1/episode-2483-fork-plan.json" \
  --readback "$HOME/episode-bundles/global-1hz-managed-smoke-2483-v1/owner-publication-readback.json" \
  --apply
```

The resulting readback is then verifier input; do not infer publication from
the local plan or a dry-run result:


```bash
python3 tools/gensim_global_1hz/verify_managed_episode_fork_readback.py \
  --plan /local/episode-2483-fork-plan.json \
  --readback /owner-retained/episode-2483-fork-readback.json \
  --output /new/local/episode-2483-fork-verification.json
```

The verifier requires distinct control/treatment episode IDs and exact matching
group, names, blueprint hash, and arm-bundle hash.

```bash
python3 -m unittest -v tools/gensim_global_1hz/build_pilot_bundles_test.py
```
