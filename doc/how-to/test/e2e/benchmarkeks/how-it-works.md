# How the benchmark works

The benchmark runs the baseline and the comparison versions of the Agent at the same time, in the same EKS cluster, each in an installation of its own. Both installations monitor the same workload, so that the only difference between them is the version of the Agent. The measurement runs in four phases, between which the installations swap their versions, which cancels out the differences between their machines.

## The cluster

```text
┌─ EKS cluster ────────────────────────────────────────────────────────────┐
│                                                                          │
│  ┌─ installation a · datadog-a ───┐  ┌─ installation b · datadog-b ───┐  │
│  │ node-agent pool · 3 nodes      │  │ node-agent pool · 3 nodes      │  │
│  │   ■ node Agent a               │  │   ■ node Agent b               │  │
│  │   ▪ sleeper pods               │  │   ▪ sleeper pods               │  │
│  ├────────────────────────────────┤  ├────────────────────────────────┤  │
│  │ cluster-agent pool · 1 node    │  │ cluster-agent pool · 1 node    │  │
│  │   ■ Cluster Agent a            │  │   ■ Cluster Agent b            │  │
│  │   · node Agent a               │  │   · node Agent b               │  │
│  ├────────────────────────────────┤  ├────────────────────────────────┤  │
│  │ cluster-checks pool · 1 node   │  │ cluster-checks pool · 1 node   │  │
│  │   ■ cluster checks runner a    │  │   ■ cluster checks runner b    │  │
│  │     running the KSM check      │  │     running the KSM check      │  │
│  │   · node Agent a               │  │   · node Agent b               │  │
│  └────────────────────────────────┘  └────────────────────────────────┘  │
│                                                                          │
│  ┌─ Fargate ──────────────────────┐  ┌─ KWOK, simulated ──────────────┐  │
│  │ CoreDNS, kwok-controller       │  │ 300 nodes · about 2,800 pods   │  │
│  │ churn orchestrator             │  │ no kubelet, no container       │  │
│  └────────────────────────────────┘  └────────────────────────────────┘  │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘

 ■ measured    ▪ the same pods on both installations    · not measured
```

Each installation is a Helm release of the Agent in a namespace of its own, `datadog-a` or `datadog-b`, with the three node pools shown above.

The nodes of the six pools are EC2 instances of the same type, `m6i.large`, with 2 vCPUs and 8 GiB of memory. Each node is labelled and tainted with its role and installation, `benchmark.datadoghq.com/role` and `benchmark.datadoghq.com/variant`, so that only the pods tolerating both run there.

An installation's node Agent runs on every node of the installation, but only those of the node-agent pool are measured. On the two other pools, it collects the container metrics of the Cluster Agent and of the cluster checks runner, which the benchmark compares, and forwards their profiles.

The Cluster Agent and the cluster checks runner have a node each, shared only with the node Agent of their installation and the network DaemonSets of EKS. The CPU limit of the Cluster Agent is the 2 vCPUs of its `m6i.large` node, and that of the cluster checks runner far higher, so that the kernel never throttles them, and their memory limits leave room for a leak to grow for a while. An OOM kill would still restart them, which [fails the test](#the-guards).

CoreDNS, the KWOK controller and the churn orchestrator, which generates the workload, run on Fargate, away from the measured nodes: the Fargate profile of the cluster matches the `kube-system` namespace and the `agent.datadoghq.com/sidecar: fargate` label of the churn orchestrator.

Both installations report the same `kube_cluster_name`, and the benchmark tells their data apart through tags:

- the namespace, `kube_namespace:datadog-a` or `kube_namespace:datadog-b`, for the containers of the Agents;
- the `role` and `variant` host tags, which the node Agents derive from the labels of their node;
- the `agent_variant` tag of the KSM metrics, set to the installation reporting them. The `variant` tag can't be used there: the KSM metrics of a pod carry the host tags of the node of that pod.

The two installations can't both own the cluster-wide resources of the Agent. Installation a alone installs the custom resource definitions of the Helm chart, and the admission controllers of both installations are disabled, because they would compete for the same webhook configurations.

See <<<repo("test/e2e-framework/scenarios/aws/benchmarkeks/run.go")>>>.

### A non-burstable instance type

The CPU usage of the Agents is what the benchmark compares, so the CPU performance of a node must not depend on its past. It does on burstable instances, such as the T family: they earn CPU credits when idle and spend them to run above their baseline performance, so their performance at a given time depends on the credits they have left. The node pools use `m6i.large`, a general-purpose type with a constant CPU performance.

The `ddinfra:aws/defaultInstanceType` parameter can change the instance type. Nothing checks that the new one isn't burstable, nor that the CPU limit of the Cluster Agent still covers the vCPUs of its node.

## The workload

The workload has two parts: objects of the Kubernetes API server, which the Cluster Agents and the KSM checks watch, and pods on the real nodes, which the node Agents monitor. Both parts change all the time: the Agents spend most of their resources handling changes rather than steady objects.

The churn orchestrator creates the workload from the manifests of <<<repo("test/e2e-framework/components/datadog/apps/churn/workload")>>>. Its `apps-churn` image is built outside of this repository. Each object of these manifests is a template: its `churn.datadoghq.com/instances` annotation sets how many copies of it churn creates, and its `churn.datadoghq.com/lifetime` annotation how long each copy lives before churn deletes and recreates it. An object without a lifetime is steady.

### Simulated objects, for the Cluster Agents and the KSM checks

```text
  ┌─ Kubernetes API server ────────────────────────────────────────┐
  │ 300 KWOK nodes and their pods, Deployments, Services,          │
  │ CronJobs, Jobs, ConfigMaps, a custom resource definition and   │
  │ its custom resources: created, deleted, recreated              │
  └───────┬─────────────────┬─────────────────┬─────────────────┬──┘
          │ watch           │ watch           │ watch           │ watch
          ▼                 ▼                 ▼                 ▼
   Cluster Agent a   KSM on runner a   Cluster Agent b   KSM on runner b
```

[KWOK](https://kwok.sigs.k8s.io) simulates the nodes of a large cluster for the price of a single controller. The 300 nodes of the benchmark are `Node` objects, created by churn, that the KWOK controller makes look alive: it marks them ready, and marks the pods scheduled on them as running, without any kubelet or container. The API server holds a cluster of 300 nodes and thousands of pods, and the Cluster Agents and the KSM checks list and watch all of it, as they would in a real cluster.

| Objects | Copies | Lifetime | Why |
|---|---|---|---|
| `kwok-node-*` Nodes, managed by KWOK | 300 | Steady | The nodes of the simulated cluster, where all the pods below are scheduled |
| `fake-*` Deployments, 10 pods of 3 containers each | 50, 500 pods | 1 minute | A constant flow of pod and ReplicaSet creations and deletions |
| `fake-*` Services | 50 | 2 minutes | Services come and go too |
| `steady-*` Deployments, 10 pods of 3 containers each | 200, 2,000 pods | Steady | The bulk of the cluster. The KSM check costs more as pods and containers grow, and the Cluster Agent decodes every pod, so these pods are large: environment variables, probes, resources, volumes, and the standard labels KSM turns into tags |
| `steady-*` Services | 200 | Steady | |
| `cron-*` CronJobs, a Job of 3 pods every minute | 30 | Steady | Pods owned by Jobs owned by CronJobs, and the job metrics of KSM. KWOK completes their pods at once, and the CronJobs keep their last 3 successful Jobs |
| `big-*` ConfigMaps, 128 KiB each | 300 | 30 minutes | Large objects KSM lists and watches, though it only needs their metadata |
| `widgets.benchmark.datadoghq.com` custom resource definition | 1 | 5 minutes | Makes the KSM checks rebuild their stores, see below. The Cluster Agents collect custom resource definitions too |
| `widget-*` Widgets, custom resources of that definition | 5 | 1 minute | Custom resources the KSM checks collect, whose store comes and goes with their definition. They are all back about a minute after its recreation |

The KSM checks rebuild all their stores at their first run after any event of a custom resource definition. Without the churned definition, they would only rebuild them while the Agents start, a number of times that varies from one run to the next. Recreated every 5 minutes, the length of a [block](statistics.md#blocks) of a full-length phase, the definition makes them rebuild their stores once or twice per recreation, depending on whether a check run falls between its deletion and its creation. A cost paid at each rebuild, such as old stores that are never released, then grows at a steady pace in every phase and every run.

### Real pods, for the node Agents

```text
┌─ node-agent node of a ───────────┐  ┌─ node-agent node of b ───────────┐
│ 15 sleeper pods · 190 containers │  │ 15 sleeper pods · 190 containers │
│        ▲                         │  │        ▲                         │
│        │ containerd, kubelet     │  │        │ containerd, kubelet     │
│ node Agent a                     │  │ node Agent b                     │
└──────────────────────────────────┘  └──────────────────────────────────┘
```

The sleeper pods run `sleep infinity` in `busybox` containers, so they cost nothing themselves: what they make work is the node Agent monitoring them, through containerd events, workloadmeta, the tagger, and the `kubelet`, `container`, `containerd` and `cri` checks.

| DaemonSets | Copies | Lifetime | Pods and containers per node |
|---|---|---|---|
| `sleeper-*`, 3 containers per pod | 10 | 1 minute | 10 pods, 30 containers |
| `steady-sleeper-*`, 32 containers per pod | 5 | Steady | 5 pods, 160 containers |

The steady pods have many containers so that these four checks take a larger share of the CPU usage of the node Agent: in the CPU profiles of the Agent 7.85.2, they take 21% with these 190 containers per node, against 16% with 70, and a run of the `kubelet` check takes about a second, three times as long as with 70. With 270 containers, the node Agent uses 45 MiB more memory than with 190, but the share of the checks only reaches 23%: process collection, log tailing and the cgroup scans of the tagger grow with the containers too. More pods would add sandboxes to the nodes, and more containers in the churning pods would load containerd and the kubelet without raising the share of the checks.

The pods of `sleeper-*` carry an Autodiscovery annotation scheduling the `simple_check` Python check, so that Autodiscovery schedules and unschedules check instances all the time too.

The sleeper DaemonSets tolerate the taints of the node-agent pools of both installations, so every measured node Agent, on either installation, monitors the same pods.

## A run, step by step

```text
 ●  Both installations and the churn orchestrator are ready
 │    functional tests, during the warm-up
 │    warm-up · 10 min
 │
 ├─ phase 1 · 15 min ────────────  installation a: baseline
 │    ├ block 1 · 5 min             installation b: comparison
 │    ├ block 2 · 5 min
 │    └ block 3 · 5 min
 │
 ●  Swap: installation a gets the comparison images,
 │        installation b the baseline ones
 │    wait for all the Agents to be ready
 │    verify their checks again, during the warm-up
 │    warm-up · 10 min
 │
 ├─ phase 2 · 15 min ────────────  installation a: comparison
 │    └ 3 blocks of 5 min           installation b: baseline
 │
 ●  Swap back, same steps · warm-up · 10 min
 │
 ├─ phase 3 · 15 min ────────────  installation a: baseline
 │    └ 3 blocks of 5 min           installation b: comparison
 │
 ●  Swap, same steps · warm-up · 10 min
 │
 ├─ phase 4 · 15 min ────────────  installation a: comparison
 │    └ 3 blocks of 5 min           installation b: baseline
 │
 ●  Wait for dddev to ingest the last minute of each phase,
    compare the versions, write the report
```

A phase starts on a whole minute, once its warm-up is over and the checks are verified: see [the guards](#the-guards). The suite posts an event to dddev at the start and at the end of each phase, tagged with `benchmark_phase` and `benchmark_phase_boundary`, which the dashboard of the benchmark overlays on its graphs: see [Read the result](index.md#read-the-result).

Each swap is a Helm upgrade of both releases, which replaces all the node Agents of an installation at once, and its Cluster Agent and cluster checks runner too, as long as both versions have different images: each phase then runs new Agent processes. The suite reads the images to swap from the specs of the pods, so the swaps work with any images, including those of a CI pipeline. It then verifies the checks again: an image that only works on the node pools of one installation must not slip through.

With the default durations, the test lasts about 110 minutes once the Agents are ready. `E2E_BENCHMARKEKS_PHASES` sets another even number of phases: see [Run it locally](index.md#run-it-locally). The phases and the swaps are in <<<repo("test/new-e2e/tests/containers/benchmarkeks_test.go", "`Test10Performance`", match="^func \\(s \\*benchmarkEKSSuite\\) Test10Performance\\(")>>>.

### Why the installations swap their versions

All the nodes have the same instance type, but not the same machine: for the same work, a node can use up to 20% more CPU time than another one. If the baseline always ran on the node pools of installation a, and the comparison on those of installation b, the difference between these machines would show up as a difference between the versions.

So each version runs half of the phases on the node pools of each installation, and the machine difference cancels out between the phases: [How the test decides](statistics.md#cancelling-out-the-machines) shows how.

Running both versions at the same time, rather than one after the other on the same nodes, also gives them the same workload, the same API server and the same conditions on AWS. Whatever happens in the cluster at a given time happens to both.

### Why four phases

Two phases, one on each installation, would cancel out the machines too, and the benchmark ran that way at first. But part of the noise is drawn anew at each start of an Agent process, and lasts as long as the process: two processes of the same binary, on the same machine, can settle at different costs. For instance, the working set of the Cluster Agent differed by up to ±5% from one process of the same binary to another, in a validation of 2026-11-07 made of A/A runs and of reverts of past optimizations. With 2 phases, a run compares a single process of each version on each installation: 2 pairs of processes for the Cluster Agent and the cluster checks runner, which made one false regression of +5.1% in that validation.

With 4 phases, each version runs twice on each installation, in new processes at every phase: 4 Cluster Agents and 4 cluster checks runners per version rather than 2, and twice as many node Agents. The estimate averages 4 pairs of processes rather than 2, which divides the part of its noise drawn per process by √2, and the machines still cancel out, with two phases on each side. Each phase is shorter, 15 minutes rather than 20, so that the phases and their warm-ups take 100 minutes rather than 60.

## What is measured

### CPU and memory

The benchmark queries dddev for three metrics of the `agent` container of the node Agents and of the cluster checks runners, and of the `cluster-agent` container of the Cluster Agents:

- `container.cpu.usage`, the CPU usage of all the processes of the container, in nanocores, that is CPU time per second: the cost of running the Agent.
- `container.memory.working_set`, the memory of the container minus its inactive page cache, which the kubelet relies on to evict pods. Unlike `container.memory.usage`, it doesn't drift with what each container read in the past, since the kernel only reclaims that cache under memory pressure.
- `container.memory.rss`, the anonymous memory of the processes, such as the Go heap and the stacks, without any file cache: a leak shows there first.

For example, the CPU usage of the node Agents of installation a comes from:

```text
avg:container.cpu.usage{kube_cluster_name:<cluster>,
    kube_namespace:datadog-a,kube_container_name:agent,
    kube_app_component:agent,role:node-agent}.rollup(avg, 60)
```

Each query returns a point per minute, the average of the containers it selects: here the three node Agents of the node-agent pool of installation a.

The node Agents of each installation collect these metrics, so a version that changed how they are computed would bias the comparison of all the components alike.

The other containers of the node Agent pods, such as `trace-agent` and `system-probe`, aren't compared, because the workload barely exercises them: no application sends traces, for instance, so `trace-agent` only forwards the profiles of the Agents. On Linux, the process checks run in the `agent` container, which is compared.

### Check run times

A check is a small part of the CPU usage of its container: a check twice as slow can barely show in the CPU usage. So the benchmark also compares the run times of two checks:

- `kubelet`, on the node Agents;
- `kubernetes_state_core`, on the cluster checks runners.

The suite runs `agent status collector --json` in the `agent` container of each measured node Agent and cluster checks runner, at the start of each block and at the end of each phase. The Agent keeps the total number of runs of each check instance, and the durations of its last 32 runs, so the difference between two snapshots gives the exact runs of a block. A block of 5 minutes holds 20 runs of a check running every 15 seconds, below the 32 the Agent keeps. Each block then contributes the mean duration of the runs of the check in all the measured Agents of the installation.

The `container` and `containerd` checks aren't timed. They share a 2-second cache of the container stats that whichever runs first fills, and that order changes at each start of the Agent, so their run times swung by up to 97% between the two installations in runs whose profiles showed no change. Their cost still shows in the CPU usage of the node Agents, and their metric samples are still compared.

## The guards

A broken check costs nothing: it must never pass for a performance improvement. So the suite verifies that both versions work before and during the measurement.

| Test | What it verifies |
|---|---|
| `Test00ChecksRun` | On both installations, the `kubelet`, `container` and `containerd` checks of the node Agents and `kubernetes_state_core` on the cluster checks runners each ran at least twice, without errors and emitting metrics. At least one `simple_check` instance ran once that way |
| `Test01ChecksMatchBaseline` | The checks of the comparison, except `simple_check`, emit as many metric samples per run as those of the baseline, within 25% |
| `Test02DataInDDDev` | The data of both installations reaches dddev, including that of every query `Test10Performance` compares |
| `Test10Performance` | From the time the Agents are ready, before the warm-up, to the end of each phase, no measured container restarts and no measured pod is replaced: a restart skews the measurement, and hints at a crash or an OOM kill. The churn orchestrator doesn't restart either: a restart could leave the workload frozen. The checks still run after each swap |

When a functional test fails, `Test10Performance` doesn't run: the performance of a broken Agent is meaningless.
