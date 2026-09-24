# e2ectl demo — QA environments for the Datadog Agent

A short daily-demo walkthrough: one tool provisions environments, installs the
Agent in them, and runs the same existing new-e2e tests against any of them.

All commands run from the repo root:

```sh
cd /home/bits/go/src/github.com/DataDog/worktrees/datadog-agent-ws/clever-comet/datadog-agent-ws
```

Binaries are refreshed everywhere (`e2ectl` on PATH and the worker beside it,
plus the `test/e2e-framework/` copies, rebuilt 2026-09-24).

The story: **one test body, three environments** — a cloud EKS cluster, a local
kind cluster, and a local Docker agent — with the Agent install always a
separate, repeatable step.

---

## 1. EKS cluster — inventory, connect, install (~4 min)

The cluster is already provisioned (`e2ectl start` on a previous day, or the
one provisioned during the session). Start by showing what exists:

```sh
e2ectl list
```

```
NAME          BASE    STATUS   AGE   AGENT
myeks         eks     ready    2h    -
demo-kube     kind    ready    3m    7.83.0
demo-local    local   ready    1m    binary
```

Every environment — local kind, EKS, EC2 hosts — is one entry in one list.
The EKS cluster has no Agent yet (the `-` in the AGENT column).

### Connect to the cluster

```sh
e2ectl connect myeks
```

```
fakeintake: http://10.1.x.x:80
env dir:    ~/.e2ectl/envs/myeks
wrote the kubeconfig (context "myeks") to ~/.e2ectl/envs/myeks/kubeconfig
added the "myeks" context to ~/.kube/config
start using it (the current context is not changed automatically):
  kubectl config use-context myeks
  kubectl --context myeks get ns
```

```sh
kubectl --context myeks get ns
```

The environment's kubeconfig was in the snapshot all along; `connect` surfaces
it as a normal kubectl context. The current context is never changed behind
your back.

### Install the Agent

```sh
e2ectl install -env myeks
```

```
... helm output ...
agent installed in "myeks" (version 7.83.0, receiver fakeintake, fakeintake http://10.1.x.x:80)
verify with: e2ectl list myeks | e2ectl fakeintake health -env myeks
```

What happened: the released 7.83.0 Agent via the Helm chart, the receiver plan
rendered into standard `datadog.*` chart values, everything pointed at the
Pulumi-provisioned fakeintake in the same VPC. No Pulumi needed here — the
Agent install never touches Pulumi.

---

## 2. A Kubernetes test on the EKS cluster (~1 min)

```sh
e2ectl test -env myeks --suite ./test/new-e2e/examples/
```

```
--- PASS: TestE2ectlKubernetesSuiteOnEKS (2s)
    --- PASS: TestE2ectlKubernetesSuiteOnEKS/TestAgentRunning
PASS
```

The test is the example in `test/new-e2e/examples/e2ectl_kubernetes_test.go`
— one suite, one assertion: the `datadog.agent.running` metric reaches the
fakeintake. `e2ectl test` sets `E2ECTL_ENV`, selects the `OnEKS` entry point
by default, and the suite attaches to the live environment: the snapshot's
kubeconfig becomes the framework's k8s client, the fakeintake client comes
from the same snapshot. Nothing is re-provisioned.

---

## 3. The same test, local kind (~4 min)

Now the same suite on a cluster that started 20 seconds ago on this laptop:

```sh
e2ectl start --config test/new-e2e/examples/e2ectl-kubernetes.yml --name demo-kube
```

```
environment "demo-kube" is ready
fakeintake: http://127.0.0.1:37111
```

```sh
e2ectl install -env demo-kube --config test/new-e2e/examples/e2ectl-kubernetes.yml
```

```
... helm output ...
agent installed in "demo-kube" (version 7.83.0, receiver fakeintake, fakeintake http://127.0.0.1:37111)
```

```sh
e2ectl test -env demo-kube --suite ./test/new-e2e/examples/
```

```
--- PASS: TestE2ectlKubernetesSuiteOnLocalKind (0.3s)
    --- PASS: TestE2ectlKubernetesSuiteOnLocalKind/TestAgentRunning
```

The point: **the same suite body** — `TestAgentRunning` is the same method
behind both entry points. Only the environment differs: managed AL2023 nodes
and a cloud fakeintake, versus kind containers and a Docker fakeintake. Tests
describe behavior, environments describe infrastructure, and the two compose.

---

## 4. Agent in Docker + an agent-subcommand test (~3 min)

Last base: the Agent as a binary built from this checkout, running in a
container on a private Docker network — the tightest local loop.

```sh
e2ectl start --config test/new-e2e/tests/agent-subcommands/e2ectl-local.yml --name demo-local
```

```
environment "demo-local" is ready
fakeintake: http://127.0.0.1:33415
```

```sh
e2ectl install -env demo-local --config test/new-e2e/tests/agent-subcommands/e2ectl-local.yml
```

```
... agent build/runtime verification output ...
agent installed in "demo-local" (binary from checkout, fakeintake http://127.0.0.1:33415)
```

The install built the Agent binary from the working tree, pinned it into a
container on the environment's network, and verified it runs (the status and
health probes in the output above are the receipt — runtime evidence, not
just a file on disk).

Now the ORIGINAL agent-subcommands health suite against it — the same test
body the EC2 suite runs:

```sh
e2ectl test -env demo-local --suite ./test/new-e2e/tests/agent-subcommands/ \
  --run 'TestLinuxHealthSuiteOnLocal/TestDefaultInstallHealthy'
```

```
--- PASS: TestLinuxHealthSuiteOnLocal (0.4s)
    --- PASS: TestLinuxHealthSuiteOnLocal/TestDefaultInstallHealthy
```

`agent health` reports PASS — the AgentClient invokes the pinned binary
directly in the container, no sudo anywhere.

### Cleanup

```sh
e2ectl stop -env demo-kube
e2ectl stop -env demo-local
```

```
Deleted nodes: ["demo-kube-control-plane"]
environment "demo-kube" stopped
environment "demo-local" stopped
```

---

## The three takeaways

1. **Environments are inventory, not YAML in the test.** `e2ectl list` shows
   every environment; tests attach to one by name instead of provisioning
   their own. One environment can serve many suites and many reruns.
2. **The Agent install is its own step.** Same environment, new Agent: rerun
   `e2ectl install` — a released version, a pipeline DEB, or a binary built
   from the checkout. Switching the receiver (fakeintake → blackhole →
   Datadog) is the same mechanism.
3. **The tests are the existing ones.** The suites running here are the
   original new-e2e suites (agent-subcommands health, the examples suite) —
   only the entry point differs: attach instead of provision.

## Notes / gotchas

- The local-binary install builds from the checkout the first time (~10 min);
  the cache is warm on this machine, so the demo runs in ~1 min. On a fresh
  clone, pre-warm with the part-4 install before the demo.
- EKS only: the fakeintake URL in step 1 is the Pulumi-provisioned ECS
  Fargate instance; if anything looks off, `e2ectl fakeintake health -env
  myeks` is the quick pre-flight before running the test.
- `e2ectl test` forces `-count=1` (no go-test caching): the attached
  environment is mutable state a cache hit would turn into a false PASS.
- Raw output has one Helm chart warning line (`Condition path
  'datadog.autoscaling.workload.enabled' ... returned non-bool value`) before
  the success summary — it is tool noise from the crds subchart, not a
  failure.

## Commands cheat sheet

| What | Command |
|---|---|
| List environments | `e2ectl list` |
| Generate a starter config | `e2ectl init --base eks --output my.yml` |
| Start an environment | `e2ectl start --config my.yml --name myenv` |
| Connect shell (ssh/kubectl) | `e2ectl connect myenv` |
| Install/update the Agent | `e2ectl install -env myenv [--config my.yml]` |
| Run a suite attached to it | `e2ectl test -env myenv --suite ./test/new-e2e/tests/<area>/` |
| Inspect fakeintake | `e2ectl fakeintake names\|metrics\|health -env myenv` |
| Stop | `e2ectl stop -env myenv` |
