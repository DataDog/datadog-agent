# Remote Observer ablations

The manual CI jobs run the component-combination search and Bayesian tuning through
Agent CI API in DDBuild. The worker runs the testbench and its built-in scorer;
CI only builds/publishes the binary and orchestrates experiments. Agent CI API must
include dd-source PR [#96928](https://github.com/ddoghq/dd-source/pull/96928).

## CI usage

1. Run `observer-build-upload-ddeval-testbench` in your branch's pipeline. This job
   can run independently; it does not trigger evaluations. It uploads once to
   `s3://observer-log-ad-eval-artifacts-ddbuild/official-releases/<commit>/<sha256>/linux-amd64/anomalydetection-testbench`.
2. Run `observer-ablation-ddeval` in the same pipeline. It consumes the publishing
   job's JSON artifact; each trial uses that exact binary SHA-256.
3. Open the job's artifacts: `summary.md`, `report.json`, `best_config.json`, and
   `study.json`. Every completed trial includes its experiment URL, metrics,
   exact config, workflow ID, and duration. A failed or interrupted job retains
   its state in `study.json` and exits unsuccessfully.

Set these variables when starting the ablation job:

| Variable (prefix `OBSERVER_ABLATION_`) | Default | Purpose |
| --- | --- | --- |
| `DATASET` | `Golden 25` | LLMObs dataset name |
| `DATASET_VERSION` | `0` | Pin explicitly, or resolve once on the first trial |
| `SCENARIO_CONCURRENCY` | `6` | Concurrent scenarios per experiment |
| `COMBOS` | `10` | Maximum component combinations (full stack, anchors, random) |
| `SEARCH_TRIALS` | `5` | Optuna trials per combination |
| `TUNE_TRIALS` | `20` | Additional trials on the winning combination |
| `SEED` | `42` | Reproducible combination selection and TPE sampling |
| `LIMIT` | `0` | Dataset record limit; use `1` for smoke tests |
| `FORCE_ENABLE`, `FORCE_DISABLE` | empty | Comma-separated components |
| `CONFIG_TEMPLATE` | publish job's config | Optional experiment JSON path in the checkout; the published binary overrides its artifact |
| `WORKFLOW_TIMEOUT` | `7200` | Seconds to poll each experiment before stopping |
| `RESUME_JOB_ID` | empty | Previous ablation job whose checkpoints should be restored |

For a smoke test set `COMBOS=1`, `SEARCH_TRIALS=1`, `TUNE_TRIALS=1`, and `LIMIT=1`.
This runs two experiments total. Defaults run up to 70 experiments, each over
the selected dataset. Depending on runtime this can exceed a single CI job's budget.
The final config is the best observed across search and tuning, even if tuning
does not improve the score. Reported elapsed time includes interruptions before resume.

Experiments run sequentially; scenarios within them run concurrently. GitLab's
`observer-ablation-ddeval` resource group serializes these jobs across this
project's pipelines. This does **not** limit unrelated local DDEval runs or other
projects; worker-wide admission control is separate.

## Resume

Set `RESUME_JOB_ID` to the interrupted job's numeric GitLab ID, keeping the same
source revision, binary, seed, dataset selection, and study settings. The job
downloads its artifacts using `CI_JOB_TOKEN`. Completed results are replayed
locally to reconstruct the optimizer; in-flight workflows are polled using their
original IDs. Optuna is pinned in CI, and checkpoint fingerprints reject changed
inputs or search code. No pickle is loaded from artifacts.

Checkpoints are written before submission and immediately after each result.
If a submission response is lost, the driver only looks up the saved workflow ID;
it never blindly submits again. If the request never reached Atlas, polling will
eventually stop and report the ID to investigate. A workflow that definitively
fails stops the study to avoid selecting a winner from incomplete evaluations.

The six-hour job reserves time for artifact upload. Resume requires its artifacts
(retained two weeks); runner loss before artifact upload cannot be recovered by
this mechanism. CI automatic retries are disabled to avoid starting a new study
while an earlier workflow is still running. After a polling timeout, resume the
old study before starting unrelated runs that could overlap it.

## Command line

With `observer-ddeval-testbench.json` downloaded from the publish job:

```sh
dda inv --dep 'optuna==4.5.0' anomalydetection.eval-pipeline \
  --eval-backend ddeval \
  --ddeval-config-template observer-ddeval-testbench.json \
  --ddeval-dataset 'Golden 25' --ddeval-dataset-version 0 \
  --ddeval-jobs 6 --n-combos 1 --n-trials-search 1 --n-trials-tune 1 \
  --ddeval-limit 1 --seed 42 --output-dir /tmp/observer-ablation-smoke
```

Repeat with `--resume` to continue. Use a new output directory for a new study.
For a fixed component set, use `eval-bayesian --eval-backend ddeval` with
`--components`, `--lock`/`--only`, and `--n-trials`. Authentication uses
`authanywhere` in CI and `ddtool auth token` locally. The standalone
`anomalydetection.eval-ddeval` local-build path is unchanged.
