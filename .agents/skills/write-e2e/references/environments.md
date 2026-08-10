# Environments and provisioners

Load when the target is anything other than a Linux host, when you need a non-default OS, architecture, or cloud, or when you want a provisioner variant without the agent or without fakeintake.

Provision only what the test uses. Every `Provisioner` includes a fakeintake, and dropping it is right for the many suites that assert on services, packaging, permissions, CLI output, or config rather than on payloads — on AWS the intake is an ECS Fargate task, so an unused one costs provisioning time and money on every run. Host provisioners drop it with the `ProvisionerNoFakeIntake` constructor; the container and cluster provisioners have no such constructor and take the scenario's `WithoutFakeIntake()` inside `WithRunOptions` instead.

Authoritative directories, in preference order when this file and the code disagree:

- `test/e2e-framework/testing/environments/` — every environment struct
- `test/e2e-framework/testing/provisioners/` — every provisioner
- `test/e2e-framework/scenarios/` — the Pulumi options each provisioner forwards to
- `test/new-e2e/examples/` — one runnable example per environment

## Environment structs

Import `"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"`.

| Struct | Fields you will use |
|---|---|
| `Host` | `RemoteHost`, `FakeIntake`, `Agent`, `Updater` |
| `WindowsHost` | `RemoteHost`, `FakeIntake`, `Agent`, `ActiveDirectory` |
| `DockerHost` | `RemoteHost`, `FakeIntake`, `Agent` (a `DockerAgent`), `Docker` |
| `Kubernetes` | `KubernetesCluster`, `FakeIntake`, `Agent` |
| `ECS` | `ECSCluster`, `FakeIntake` |

## Provisioners

Every path below is prefixed `github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/`.

| Cloud / target | Path | Package | Constructors |
|---|---|---|---|
| AWS host | `aws/host` | `awshost` | `Provisioner`, `ProvisionerNoFakeIntake`, `ProvisionerNoAgentNoFakeIntake` |
| AWS Windows host | `aws/host/windows` | `winawshost` | `Provisioner`, `ProvisionerNoAgent`, `ProvisionerNoFakeIntake`, `ProvisionerNoAgentNoFakeIntake` |
| AWS Docker host | `aws/docker` | `awsdocker` | `Provisioner` |
| AWS kind on EC2 | `aws/kubernetes/kindvm` | `kindvm` | `Provisioner` |
| AWS EKS | `aws/kubernetes/eks` | `eks` | `Provisioner` |
| AWS kubeadm | `aws/kubernetes/kubeadm` | `kubeadm` | `Provisioner` |
| AWS ECS | `aws/ecs` | `ecs` | `Provisioner` |
| Azure host | `azure/host/linux` | `azurehost` | `Provisioner`, `ProvisionerNoFakeIntake`, `ProvisionerNoAgentNoFakeIntake` |
| Azure Windows host | `azure/host/windows` | `winazurehost` | `Provisioner`, `ProvisionerNoAgent`, `ProvisionerNoFakeIntake`, `ProvisionerNoAgentNoFakeIntake` |
| Azure AKS | `azure/kubernetes` | `azurekubernetes` | `AKSProvisioner` |
| GCP host | `gcp/host/linux` | `gcphost` | `Provisioner`, `ProvisionerNoFakeIntake`, `ProvisionerNoAgentNoFakeIntake` |
| GCP GKE | `gcp/kubernetes` | `gcpkubernetes` | `GKEProvisioner` |
| GCP OpenShift | `gcp/kubernetes/openshiftvm` | `gcpopenshiftvm` | `OpenshiftVMProvisioner` |
| Local podman | `local/host` | `localhost` | `PodmanProvisioner` and `…NoFakeIntake` variants |
| Local kind | `local/kubernetes` | `localkubernetes` | `Provisioner`, `OpenShiftLocalProvisioner` |

Tests conventionally alias the kind provisioner as `awskubernetes`. That alias is a naming convention, not a package name — the import path ends in `kindvm`.

The `NoFakeIntake` and `NoAgent` constructors exist only on the host provisioners. Docker, ECS, kind, EKS and kubeadm expose `Provisioner` alone, and drop the intake with the scenario's `WithoutFakeIntake()` inside `WithRunOptions`.

Dropping the *agent* is not uniformly available: `WithoutAgent()` exists on the `ec2`, `ec2docker`, and `eks` scenarios only. ECS, kindvm, and kubeadm have no such option, so confirm it exists before relying on it.

## Two option shapes

AWS provisioners expose `WithExtraConfigParams`, `WithRunOptions`, and an environment option whose name varies — `WithEnv` on `awshost` and `awsdocker`, `WithAwsEnv` on `ecs`, `kindvm`, `eks`, and `kubeadm`, and none at all on `winawshost`. Everything else nests inside `WithRunOptions` as a scenario option:

```go
awshost.Provisioner(
	awshost.WithRunOptions(
		ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.Ubuntu2204)),
		ec2.WithAgentOptions(agentparams.WithLogs()),
	),
)
```

Azure, GCP, and local provisioners are flat — the same options hang directly off the provisioner package:

```go
azurehost.Provisioner(
	azurehost.WithAgentOptions(agentparams.WithLogs()),
	azurehost.WithInstanceOptions(compute.WithInstanceType("Standard_D2s_v3")),
)
```

Copying a snippet across clouds without adjusting the shape is the most common compile failure. Confirm with `grep -n '^func With' test/e2e-framework/testing/provisioners/<path>/*.go`.

## Scenario options

Prefixed `github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/`.

| Scenario | Path | Notable options |
|---|---|---|
| EC2 host | `aws/ec2` | `WithEC2InstanceOptions`, `WithAgentOptions`, `WithAgentClientOptions`, `WithFakeIntakeOptions`, `WithoutAgent`, `WithoutFakeIntake`, `WithUpdater`, `WithDocker` |
| EC2 instance | `aws/ec2` | `WithOS`, `WithOSArch`, `WithAMI`, `WithLatestAMI`, `WithInstanceType`, `WithUserData`, `WithIMDSv1Disable`, `WithVolumeThroughput` |
| Docker on EC2 | `aws/ec2docker` | `WithEC2VMOptions`, `WithAgentOptions`, `WithTestingWorkload`, `WithPreAgentInstallHook` |
| kind on EC2 | `aws/kindvm` | `WithVMOptions`, `WithAgentOptions`, `WithFakeintakeOptions`, `WithDeployDogstatsd`, `WithDeployTestWorkload`, `WithWorkloadApp`, `WithKindWorkerNodes`, `WithDeployOperator` |
| EKS | `aws/eks` | `WithEKSOptions(WithLinuxNodeGroup, WithLinuxARMNodeGroup, WithBottlerocketNodeGroup, WithWindowsNodeGroup, WithGPUNodeGroup, WithoutFargate)` |
| ECS | `aws/ecs` | `WithECSOptions(WithLinuxNodeGroup, WithWindowsNodeGroup, WithFargateCapacityProvider)`, `WithAgentOptions`, `WithTestingWorkload` |
| Windows on EC2 | `aws/ec2/windows` | `WithAgentOptions`, `WithActiveDirectoryOptions`, `WithDefenderOptions`, `WithFIPSModeOptions`, `WithTestSigningOptions` |
| fakeintake | `aws/fakeintake` | `WithLoadBalancer`, `WithCPU`, `WithMemory`, `WithRetentionPeriod` |

## Agent parameters

`components/datadog/agentparams` — the full list is in `params.go`; these cover most tests:

`WithAgentConfig(yaml)` · `WithIntegration(folder, yaml)` (`folder` is like `custom_logs.d`) · `WithLogs()` · `WithTelemetry()` · `WithSystemProbeConfig(yaml)` · `WithSecurityAgentConfig(yaml)` · `WithFile(path, content, useSudo)` · `WithFileWithPermissions(...)` · `WithTags([]string)` · `WithHostname(string)` · `WithAdditionalInstallParameters([]string)` (MSI flags) · `WithVersion` / `WithPipeline` / `WithLocalPackage` / `WithFlavor`.

Container equivalents: `components/datadog/dockeragentparams` and `components/datadog/kubernetesagentparams` (`WithHelmValues` is repeatable and layers; `WithNamespace`, `WithJMX`, `WithFIPS`, `WithOTelAgent`, `WithGKEAutopilot`, `WithDualShipping`). For environment variables the containerised agent must see, `test/e2e-framework/AGENTS.md` § "Agent configuration" covers `WithAgentServiceEnvVariable` and `AgentServiceEnvironment`.

## Operating systems

Import `e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"`; the lists live in `linux_descriptors.go` and `windows_descriptors.go`.

Prefer `Ubuntu2204E2E` (also `UbuntuDefault`) — a prebaked image with the test tooling already installed, which avoids installing packages on the VM. Also available: `Ubuntu2404`, `Ubuntu2204`, `Debian12`, `AmazonLinux2023`, `AmazonLinux2`, `RedHat9`, `Suse15`, `Fedora40`, `CentOS7`, `AlmaLinux9`, `WindowsServer2025` (`WindowsServerDefault`), `WindowsServer2022/2019/2016`, and `WindowsClient*`.

```go
ec2.WithOS(e2eos.Ubuntu2204)
ec2.WithOSArch(e2eos.AmazonLinux2023, e2eos.ARM64Arch)
ec2.WithAMI("ami-0123456789abcdef0", e2eos.Ubuntu2204, e2eos.AMD64Arch)
ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.AlmaLinux9), ec2.WithLatestAMI()) // AlmaLinux resolves its AMI by search
```

Region and cloud account are not chosen in test code — they come from the runner profile and Pulumi config. The only test-side knob is `WithExtraConfigParams(runner.ConfigMap{...})`, or `--configparams k=v` on the command line.

## Suite options and custom environments

`test/e2e-framework/AGENTS.md` §§ "Useful suite options" and "Beyond out of the box environments" cover `WithDevMode`, `WithStackName`, the custom-environment and untyped escape hatches, and the `UpdateEnv` caveat. Four things it does not say:

- `e2e.WithSkipDeleteOnFailure()` keeps a failed run's infrastructure for inspection, unlike `WithDevMode` which keeps it unconditionally.
- A suite may take several provisioners as long as each has a distinct `ID()`.
- `BeforeTest` reverts to the suite's original provisioners, so an `UpdateEnv` in one test does not leak into the next.
- In the untyped escape hatch (`test/new-e2e/examples/suite_serial_kube_test.go`), env fields are matched to Pulumi resources by `import:"dd-KubernetesCluster-kind"` struct tags rather than by type.
