# KJob Kubernetes tests

This folder holds the Kubernetes performance tests of the Datadog Agent. Each test, or case, deploys Agent components in a Kubernetes cluster and checks their resource use. The tests find memory regressions that only show in a Kubernetes deployment, for example with the Operator, the Cluster Agent, or many Pods.

KJob is the Kubernetes test service of the Single Machine Performance (SMP) team. Agent CI sends the cases to it, and it runs them.

## How it works

1. On each dev-branch pipeline, the `single_machine_performance-kjob` CI job sends all cases in `cases/` to KJob as one job.
2. KJob runs each case 10 times. For each run, it makes a clean cluster, deploys the case, and waits until the workloads are ready.
3. KJob then measures the containers for 60 seconds, and checks the memory bounds that the case declares.
4. When all runs finish, the CI job posts a `K8s Regression Detector Results` comment on the PR. The comment shows which checks passed or failed, and it links to the logs and metrics of the runs.

The CI job never blocks a pipeline. A failed check shows in the comment and in the CI job result.

A case runs on one of two cluster types. **KinD** runs a local cluster on a shared host. **EKS** runs a managed AWS cluster with a real worker node. Results from the two types are not comparable, so each case selects one type.

## Layout

```text
test/regression/kjob/
├── README.md
├── report_template.md.j2    # Template of the PR comment
└── cases/
    └── <case>/              # One directory per case
        ├── README.md        # What the case tests
        ├── experiment.yaml  # Cluster type, load, HTTP receivers, and bounds
        ├── target/          # Manifests of the components under test
        ├── activity/        # Optional manifests that add load
        └── helm/            # Optional Helm values files
```

## Add a case

1. Copy a case directory under `cases/`, and give it a new name. The name is a lowercase DNS label of no more than 40 characters.
2. Change the manifests and `experiment.yaml`. Write a `README.md` that says what the case tests.
3. The clusters have no internet access. Send the Agent payloads to the HTTP receivers that `experiment.yaml` declares, as the current cases do.
4. To check the case locally, run `smp kjob submit test/regression/kjob --dry-run`. This needs Helm `v4.3.1` and no AWS access.

CI runs the new case with all other cases. See the [KJob user guide](https://github.com/ddoghq/single-machine-performance/blob/paul.reinlein/krjo-poc/docs/kjob/user-guide.md) for the full case format.
  - *Note: This will be updated later to an actual public confluence d
