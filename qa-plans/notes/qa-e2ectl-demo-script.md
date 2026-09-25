# e2ectl demo — commands only

From the repo root:

```sh
cd /home/bits/go/src/github.com/DataDog/worktrees/datadog-agent-ws/clever-comet/datadog-agent-ws
```

## 1. EKS: list, connect, install

```sh
e2ectl list
e2ectl connect myeks
kubectl --context myeks get ns
e2ectl install -env myeks
e2ectl fakeintake health -env myeks
```

## 2. Kubernetes test on EKS

```sh
e2ectl test -env myeks --suite ./test/new-e2e/examples/ --run TestE2ectlKubernetesSuite
```

## 3. Same test, local kind

```sh
e2ectl start  --config test/new-e2e/examples/e2ectl-kubernetes.yml --name demo-kube
e2ectl install -env demo-kube --config test/new-e2e/examples/e2ectl-kubernetes.yml
e2ectl test -env demo-kube --suite ./test/new-e2e/examples/ --run TestE2ectlKubernetesSuite
```

## 4. Local Docker agent + agent-subcommands

```sh
e2ectl start  --config test/new-e2e/tests/agent-subcommands/e2ectl-local.yml --name demo-local
e2ectl install -env demo-local --config test/new-e2e/tests/agent-subcommands/e2ectl-local.yml
e2ectl test -env demo-local --suite ./test/new-e2e/tests/agent-subcommands/ --run TestLinuxHealthSuiteOnLocal
```

## Cleanup

```sh
e2ectl stop -env demo-kube
e2ectl stop -env demo-local
```
