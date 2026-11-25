# KJob Kubernetes tests

This folder holds the Kubernetes performance tests of the Datadog Agent. Each test, or case, deploys Agent components in a Kubernetes cluster and checks their resource use. The tests find memory regressions that only show in a Kubernetes deployment, for example with the Operator, the Cluster Agent, or many Pods.

KJob is the Kubernetes test service of the Single Machine Performance (SMP) team. Agent CI sends the cases to it, and it runs them.

## How it works

1. Add a label to the PR, then push a commit. The CI jobs read the labels when they start.

   | Label | Images | KJob CI job | Comment job |
   |---|---|---|---|
   | `smp-run-k8s-experiments` | The default images of the cases. The job does not wait for the image builds. | `single_machine_performance-kjob` | `single_machine_performance-kjob-pr-comment` |
   | `smp-run-k8s-experiments-with-build` | The Agent and Cluster Agent images of the commit. The job waits for the image builds. This label wins when the PR has both labels. | `single_machine_performance-kjob-with-build` | `single_machine_performance-kjob-with-build-pr-comment` |

   To use a label in a pipeline that already started, retry the KJob CI job of the label. When it finishes, retry its comment job, because a retry does not run again a comment job that passed.

   The pipeline builds the Node Agent image only when the PR changes Agent source or build files. Without this image, the with-build label makes `single_machine_performance-kjob-with-build` fail, and the comment tells why.
2. The CI job sends all cases in this folder to KJob as one job.
3. KJob runs each case 10 times. For each run, it makes a clean cluster, deploys the case, and waits until the workloads are ready.
4. KJob then measures the containers for 60 seconds, and checks the memory bounds that the case declares.
5. When all runs finish, the comment job of the label posts or updates the `K8s Regression Detector Results` comment on the PR. The comment shows which checks passed or failed, and it links to the logs and metrics of the runs.

Without a label, the CI jobs do not run KJob and change nothing on the PR. The CI jobs never block a pipeline. A failed check or run shows in the comment, and the CI job still passes. The CI job fails when it cannot read the PR labels. It also fails when it cannot get a result, for example when the runs do not finish in time.

Each case runs on KinD, a local cluster on a shared host.

## Layout

```text
test/regression/kjob/
├── README.md
├── report_template.md.j2    # Template of the PR comment
└── <case>/                  # One directory per case
    ├── README.md            # What the case tests
    ├── experiment.yaml      # Runner, KWOK, image parameters, Helm charts, HTTP receivers, and bounds
    ├── target/              # Manifests of the components under test
    ├── activity/            # Optional manifests that add load
    └── helm/                # Optional Helm values files
```

## Add a case

1. Copy a case directory in this folder, and give it a new name. The name is a lowercase DNS label of no more than 40 characters.
2. Change the manifests and `experiment.yaml`. A case declares its images as the parameters `node_agent_image` and `cluster_agent_image`, with default values. Use these parameters in the manifests. CI replaces only these two parameters, with the `-with-build` label. Write a `README.md` that says what the case tests.
3. The clusters have no internet access. Send the Agent payloads to the HTTP receivers that `experiment.yaml` declares, as the current cases do.
4. To check the case locally, run `smp kjob submit test/regression/kjob --dry-run`. This needs Helm `v4.3.1` and no AWS access.

CI runs the new case with all other cases. See the [KJob user guide](https://github.com/ddoghq/single-machine-performance/blob/paul.reinlein/krjo-poc/docs/kjob/user-guide.md) for the full case format.
  - *Note: This will be updated later to an actual public confluence d
