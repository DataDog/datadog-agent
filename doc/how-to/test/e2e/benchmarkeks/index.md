# Performance benchmark

`benchmarkeks` is the performance non-regression test of the Agent on Kubernetes. It deploys two versions of the Agent side by side on one EKS cluster, has both monitor the same busy workload, and compares how much CPU and memory they use. It answers one question: does the Agent of a pull request cost more, or less, than the Agent of its base branch?

The test is the `TestBenchmarkEKSSuite` suite of <<<repo("test/new-e2e/tests/containers/benchmarkeks_test.go")>>>.

## What it compares

The two versions of the Agent are:

- the **baseline**, the Agent of the base branch;
- the **comparison**, the Agent under test, built by the pipeline in CI.

For each version, the test measures three components, each loaded by a different part of the workload:

| Component | What loads it |
|---|---|
| Node Agent | The containers of its node, through the `kubelet`, `container`, `containerd` and `cri` checks and Autodiscovery |
| Cluster Agent | The objects of the Kubernetes API server it watches |
| Cluster checks runner | The `kubernetes_state_core` (KSM) check its Cluster Agent dispatches to it, which watches the API server too |

For each component, it compares [the CPU and memory usage](how-it-works.md#cpu-and-memory) of the container: `container.cpu.usage`, `container.memory.working_set` and `container.memory.rss`. It also compares [the run times](how-it-works.md#check-run-times) of the `kubelet` and `kubernetes_state_core` checks. Unlike other E2E tests, it has no fakeintake: the Agents report to dddev, the Datadog org of the E2E tests, and the test reads their metrics back from there.

Each comparison gets a verdict: **unchanged**, **improvement**, **regression**, or **error** when it couldn't be measured. A regression or an error fails the test. Before measuring anything, the test [verifies that both versions work](how-it-works.md#the-guards).

The next pages explain [how the test works](how-it-works.md) and [how it decides that a change is real](statistics.md).

## Run it in CI

The benchmark keeps an EKS cluster for about two and a half hours, so it only runs on demand, except in the pull requests changing the benchmark itself:

| Pull request changing | Benchmark | Pull request comment |
|---|---|---|
| The benchmark itself: its test, scenario, workload, CI jobs or baseline lookup | Runs automatically | Shows that it runs, then its results |
| The code it measures: the container checks, KSM, workloadmeta, the tagger, Autodiscovery, the check runner and the Kubernetes utilities | Manual | A button to run it, then that it runs and its results |
| Anything else | Manual | None, unless the benchmark is run |

The paths of the first two rows are in <<<repo(".gitlab/test/e2e/e2e_containers_benchmarkeks.yml", "the CI jobs", match="^\\.benchmarkeks_test_changes:")>>>. The benchmark only runs automatically, and the button is only posted, in push pipelines, not in the pipelines started from the GitLab UI or API, which would run the benchmark again, or replace its results with the button, for a commit whose push pipeline ran it. The benchmark runs automatically even before the pull request is opened, as the first push of a branch usually comes before its pull request: the comment then appears with the results, if the pull request exists by then.

To run it, play the manual job `new-e2e-containers-benchmarkeks-baseline` of the pipeline, which the button of the pull request comment links to. The other jobs of the benchmark wait for it, then start on their own:

```text
manual ▶ new-e2e-containers-benchmarkeks-baseline
           │  finds the baseline commit and copies its images to agent-qa
           ▼
         new-e2e-containers-benchmarkeks-init
           │  creates the EKS cluster, its node pools and KWOK,
           │  while the pipeline builds the images of the Agent
           ▼
         new-e2e-containers-benchmarkeks  ◄── images of the pipeline
           │  installs both Agents and the workload, runs the test,
           │  keeps the report in its artifacts
           ▼
         new-e2e-containers-benchmarkeks-pr-comment
              adds the report to the pull request comment
```

A single comment of the pull request follows the benchmark, which its jobs update as it progresses, through <<<repo("tasks/new_e2e_tests.py", "`dda inv new-e2e-tests.benchmarkeks-comment`", match="^def benchmarkeks_comment\\(")>>>:

1. `new-e2e-containers-benchmarkeks-instructions` posts a button, ▶ **Run the benchmark**, linking to the manual job, when the pull request changes the code the benchmark measures.
2. `new-e2e-containers-benchmarkeks-baseline` turns it into ⏳ **Benchmark running** when it starts, linking to the job of the benchmark.
3. `new-e2e-containers-benchmarkeks-pr-comment` replaces it with the outcome, followed by the report: ✅ **No regression**, ❌ **Regression**, ⚠️ **Incomplete comparison** when a metric couldn't be compared, or ⚠️ **Benchmark failed** when the benchmark failed before comparing the versions.

Each pipeline starts the comment over, and GitHub keeps its previous versions in the history of its edits. Only the jobs of the pipeline of the head commit of the pull request update it: a push cancels the jobs of the previous pipeline that haven't started yet, and those of an older commit that still run leave the comment as it is. On a manual run, though, when the job of the benchmark doesn't run, e.g. because a build of the images of the pipeline failed, the comment keeps saying that the benchmark runs.

The baseline commit is the merge base of the branch with the `base_branch` of `release.json`, which is `main`, or the release branch for a branch based on one. When no pipeline built images for that commit, it is the closest older commit of the base branch with images, up to <<<repo("tasks/libs/testing/benchmarkeks.py", "about 4 days old", match="^MAX_BASELINE_AGE =")>>>. Otherwise, the job fails and asks to rebase the branch on its base branch. The E2E tests pull their images from `agent-qa`, where images expire after a day, so the job copies the baseline images there again from the registry of the builds, which keeps them for months.

/// note
The benchmark jobs only exist in the full pipelines of development branches. Following <<<repo(".gitlab/test/e2e/e2e_containers_benchmarkeks.yml", "their rules", match="^\\.except_benchmarkeks_unsupported:")>>>, they don't exist in:

- the pipelines of `main`, release branches, the merge queue and tags;
- light pipelines, such as those of pull requests based on another branch than `main` or a release branch;
- coverage pipelines, deploy pipelines, and pipelines run with `RUN_E2E_TESTS` set to `off`.

They are allowed to fail for now: a regression shows as a failed `new-e2e-containers-benchmarkeks` job, but doesn't fail the pipeline.
///

### Read the result

The job `new-e2e-containers-benchmarkeks` prints the report at the end of its log, and keeps it in its artifacts, in `e2e-output/benchmarkeks-report/`:

- `report.json`, whenever the measurement completes, with every number of the comparison, including the means and the log ratio of each phase;
- `report.md`, the report of the pull request comment, in the Markdown of GitHub.

In the comment, the results and the versions are in collapsed sections. The summary of the results tells whether metrics regressed or improved, and the summary of the versions their commits.

The report gives the times of every phase, then a table like this one, from a CI run comparing two builds of the same Agent code, made when the benchmark ran 2 phases of 20 minutes and [the sleeper pods](how-it-works.md#real-pods-for-the-node-agents) had only 70 containers per node:

| Component | Metric | Baseline | Comparison | Change | p-value | Verdict |
|---|---|--:|--:|--:|--:|---|
| Node Agent | `container.cpu.usage` | 82.9 millicores | 81.9 millicores | -1.2% (-1.0 millicores) | 0.173 | ⚪ unchanged |
| Node Agent | `container.memory.working_set` | 192.4 MiB | 185.1 MiB | -3.8% (-7.3 MiB) | 0.002 | ⚪ unchanged |
| Node Agent | `container.memory.rss` | 187.1 MiB | 179.7 MiB | -3.9% (-7.3 MiB) | 0.002 | ⚪ unchanged |
| Cluster Agent | `container.cpu.usage` | 79.8 millicores | 79.7 millicores | -0.2% (-0.0 millicores) | 0.736 | ⚪ unchanged |
| Cluster Agent | `container.memory.working_set` | 257.4 MiB | 266.9 MiB | +3.7% (+9.5 MiB) | 0.011 | ⚪ unchanged |
| Cluster Agent | `container.memory.rss` | 255.1 MiB | 264.7 MiB | +3.8% (+9.5 MiB) | 0.011 | ⚪ unchanged |
| Cluster checks runner | `container.cpu.usage` | 275.1 millicores | 274.2 millicores | -0.3% (-0.9 millicores) | 0.475 | ⚪ unchanged |
| Cluster checks runner | `container.memory.working_set` | 746.5 MiB | 745.0 MiB | -0.2% (-1.5 MiB) | 0.685 | ⚪ unchanged |
| Cluster checks runner | `container.memory.rss` | 740.9 MiB | 739.3 MiB | -0.2% (-1.6 MiB) | 0.675 | ⚪ unchanged |
| Node Agent | `kubelet` check run time | 352.1 ms | 304.8 ms | -13.4% (-47.4 ms) | 0.001 | ⚪ unchanged |
| Cluster checks runner | `kubernetes_state_core` check run time | 1555.5 ms | 1542.1 ms | -0.9% (-13.4 ms) | 0.151 | ⚪ unchanged |

**Baseline** and **Comparison** are the averages of each version over the phases, and the value in parentheses in **Change** is their difference. The verdict is ⚪ unchanged, 🟢 improvement, 🔴 regression, or ⚠️ error when the metric couldn't be compared. The percentage comes from the ratios of both versions block by block, averaged over the phases, which [cancels out the machines](statistics.md#cancelling-out-the-machines): it can differ slightly from the ratio of the two columns. [How the test decides](statistics.md) explains the p-value and the verdict.

In this run, three rows are significant, below 0.05 / 11 ≈ 0.0045, but below their thresholds: both memory metrics of the node Agents (−7.3 MiB, under 10 MiB) and the run time of the `kubelet` check (−13.4%, under 20%). Both builds run the same code: these are differences that [escape the p-value](statistics.md#limits), and that the thresholds absorb.

The report then lists the images of both versions, and links, for each component, to the comparison page of dddev between the CPU profiles of the baseline (A) and of the comparison (B): the Agents upload a profile every 5 minutes throughout the run, which shows where a regression comes from. The profiles are selected by `image_tag`, that is by version as long as the images of the two versions have different tags, from the start of the first phase to the end of the last one, so that a comparison holds those of all the processes of each version, on both installations, rather than the 3 profiles a single process uploads in a phase.

The report and the end of the log also link to the dashboard [E2e tests — containers — benchmarkeks](https://dddev.datadoghq.com/dashboard/bkx-svx-pjc) of dddev, set to the cluster and the time of the run. Its graphs of the Agents have a curve per `image_tag`, that is per version, as long as the images of the two versions have different tags: a version keeps its curve across the swaps, which only move it to the node pools of the other installation. The links of the metrics in the report split them by `image_tag` too. Green and orange markers show the start and the end of each phase: the test posts them to dddev as events, whose text tells which images each installation runs. Its table of profiles links to the same comparisons, for the CPU time, the allocations, the live heap and the goroutines of each component.

## Run it locally

Follow the [one-time E2E setup](../index.md#one-time-setup) first. Because the test reads the metrics of the Agents back from dddev, your E2E configuration needs a dddev API key, and the test needs a dddev application key, set as for an [EKS cluster of manual QA](../../manual-qa/aws-eks.md#prerequisites).

Set the images of both versions, for instance to compare two releases:

```bash
dda inv new-e2e-tests.run --targets=./tests/containers \
  --run='^TestBenchmarkEKSSuite$' \
  --configparams ddagent:baselineVersion=7.84.0 \
  --configparams ddagent:baselineClusterAgentVersion=7.84.0 \
  --configparams ddagent:comparisonVersion=7.85.0 \
  --configparams ddagent:comparisonClusterAgentVersion=7.85.0
```

The parameters start with `ddagent:baseline` or `ddagent:comparison`, followed by:

| Image | Full image path | Tag only, in the default repository |
|---|---|---|
| Node Agent, which the cluster checks runner also runs | `FullImagePath` | `Version` |
| Cluster Agent | `ClusterAgentFullImagePath` | `ClusterAgentVersion` |

/// warning
Set the Cluster Agent image too: without it, both versions run the same default Cluster Agent image. Without an explicit image, the E2E framework picks one in its usual <<<repo("test/e2e-framework/components/datadog/agent/docker_image.go", "resolution order", match="^func DockerClusterAgentFullImagePath")>>>, which uses the images of the pipeline when `E2E_PIPELINE_ID` and `E2E_COMMIT_SHA` are set.
///

The test reads these environment variables, whose defaults are set in <<<repo("test/new-e2e/tests/containers/benchmarkeks_test.go", "the suite", match="defaultBenchmarkPhases +=")>>>:

| Variable | Default | Effect |
|---|---|---|
| `E2E_BENCHMARKEKS_PHASES` | `4` | The number of measurement phases, even and at least 2: the installations swap their versions between consecutive phases |
| `E2E_BENCHMARKEKS_WARMUP` | `10m` | How long the Agents run before each measurement phase starts |
| `E2E_BENCHMARKEKS_DURATION` | `15m` | How long each measurement phase lasts, a whole number of minutes, at least `3m` |
| `E2E_BENCHMARKEKS_REPORT_DIR` | The output directory of the suite | Where the reports are written |
| `E2E_BENCHMARKEKS_BASELINE_AGENT_IMAGE`, `E2E_BENCHMARKEKS_BASELINE_CLUSTER_AGENT_IMAGE` | None | The node Agent and Cluster Agent images of the baseline, set by the CI jobs. They take precedence over the `ddagent:baseline*` parameters, so unset them to use those |
| `E2E_BENCHMARKEKS_BASELINE_SHA` | None | The commit of the baseline images, shown in the report |

Shorter durations, or 2 phases, make a quick try possible, at the cost of a less sensitive comparison: the [blocks](statistics.md#blocks) then shrink, down to 1 minute, so that each phase still holds at least 3 of them, and fewer phases average fewer Agent processes (see [why four phases](how-it-works.md#why-four-phases)).

To look around the cluster without running the test, `dda inv aws.create-benchmarkeks` deploys the same scenario, and `dda inv aws.destroy-benchmarkeks` deletes it: see <<<repo("tasks/e2e_framework/aws/benchmarkeks.py")>>> for their options, and the [stack lifecycle](../../manual-qa/index.md#stack-lifecycle) of manual QA.

## Where the code lives

| Path | Content |
|---|---|
| <<<repo("test/e2e-framework/scenarios/aws/benchmarkeks")>>> | The Pulumi scenario: the cluster, its node pools, the two Helm installations of the Agent |
| <<<repo("test/e2e-framework/components/datadog/apps/churn")>>> | The deployment of the churn orchestrator, and the manifests of the workload |
| <<<repo("test/e2e-framework/components/datadog/apps/kwok")>>> | The KWOK controller, which simulates the nodes |
| <<<repo("test/e2e-framework/testing/provisioners/aws/kubernetes/benchmarkeks")>>> | The provisioner of the `BenchmarkEKS` environment |
| <<<repo("test/new-e2e/tests/containers/benchmarkeks_test.go")>>> | The test suite |
| <<<repo("test/new-e2e/tests/containers/benchmark")>>> | The statistics and the report |
| <<<repo(".gitlab/test/e2e/e2e_containers_benchmarkeks.yml")>>> | The CI jobs |
| <<<repo("tasks/libs/testing/benchmarkeks.py")>>> | The choice of the baseline |
