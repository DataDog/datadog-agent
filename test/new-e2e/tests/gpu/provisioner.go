// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package gpu

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent/helm"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	"github.com/DataDog/test-infra-definitions/components/docker"
	"github.com/DataDog/test-infra-definitions/components/kubernetes/nvidia"
	"github.com/DataDog/test-infra-definitions/components/os"
	componentsremote "github.com/DataDog/test-infra-definitions/components/remote"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
)

// gpuEnabledAMI is an AMI that has GPU drivers pre-installed. In this case it's
// an Ubuntu 22.04 with NVIDIA drivers
const gpuEnabledAMI = "ami-03ee78da2beb5b622"

// gpuInstanceType is the instance type to use. By default we use g4dn.xlarge,
// which is the cheapest GPU instance type
const gpuInstanceType = "g4dn.xlarge"

// nvidiaPCIVendorID is the PCI vendor ID for NVIDIA GPUs, used to identify the
// GPU devices with lspci
const nvidiaPCIVendorID = "10de"

// cudaSanityCheckImage is a Docker image that contains a CUDA sample to
// validate the GPU setup with the default CUDA installation. Note that the CUDA
// version in this image must be equal or less than the one installed in the
// AMI.
const cudaSanityCheckImage = "669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/nvidia/cuda:12.6.3-base-ubuntu22.04"

// nvidiaSMIValidationCmd is a command that checks if the nvidia-smi command is
// available and can list the GPUs
const nvidiaSMIValidationCmd = "nvidia-smi -L | grep GPU"

// validationCommandMarker is a command that can be appended to all validation commands
// to identify them in the output, which can be useful to later force retries. Retries
// are controlled in test/new-e2e/pkg/utils/infra/retriable_errors.go, and the way to
// identify them are based on the pulumi logs. This command will be echoed to the output
// and can be used to identify the validation commands.
const validationCommandMarker = "echo 'gpu-validation-command'"

const defaultSysprobeConfig = `
gpu_monitoring:
  enabled: true

system_probe_config:
  log_level: debug
`

const helmValuesTemplate = `
datadog:
  kubelet:
    tlsVerify: false
  clusterName: "%s"
  gpuMonitoring:
    enabled: true
  logLevel: DEBUG
agents:
  useHostNetwork: true
  volumes:
    - name: host-root-proc
      hostPath:
        path: /host/proc
  volumeMounts:
    - name: host-root-proc
      mountPath: /host/root/proc
  containers:
    systemProbe:
      env:
        - name: HOST_PROC
          value: "/host/root/proc"
`

const dockerPullMaxRetries = 3

type provisionerParams struct {
	agentOptions           []agentparams.Option
	kubernetesAgentOptions []kubernetesagentparams.Option
	ami                    string
	amiOS                  os.Descriptor
	instanceType           string
	dockerImages           []string
}

func getDefaultProvisionerParams() *provisionerParams {
	return &provisionerParams{
		agentOptions: []agentparams.Option{
			agentparams.WithSystemProbeConfig(defaultSysprobeConfig),
		},
		kubernetesAgentOptions: nil,
		ami:                    gpuEnabledAMI,
		amiOS:                  os.Ubuntu2204,
		instanceType:           gpuInstanceType,
		dockerImages:           []string{cudaSanityCheckImage},
	}
}

// gpuHostProvisioner provisions a single EC2 instance with GPU support
func gpuHostProvisioner(params *provisionerParams) provisioners.Provisioner {
	return provisioners.NewTypedPulumiProvisioner[environments.Host]("gpu", func(ctx *pulumi.Context, env *environments.Host) error {
		name := "gpuvm"
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return fmt.Errorf("aws.NewEnvironment: %w", err)
		}

		// Create the EC2 instance
		host, err := ec2.NewVM(awsEnv, name,
			ec2.WithInstanceType(params.instanceType),
			ec2.WithAMI(params.ami, params.amiOS, os.AMD64Arch),
		)
		if err != nil {
			return fmt.Errorf("ec2.NewVM: %w", err)
		}
		err = host.Export(ctx, &env.RemoteHost.HostOutput)
		if err != nil {
			return fmt.Errorf("host.Export: %w", err)
		}

		// Create the fakeintake instance
		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, name)
		if err != nil {
			return fmt.Errorf("fakeintake.NewECSFargateInstance: %w", err)
		}
		err = fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)
		if err != nil {
			return fmt.Errorf("fakeIntake.Export: %w", err)
		}

		// install the ECR credentials helper
		// required to get pipeline agent images or other internally hosted images
		installEcrCredsHelperCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, host)
		if err != nil {
			return fmt.Errorf("ec2.InstallECRCredentialsHelper: %w", err)
		}

		// Validate GPU devices
		validateGPUDevicesCmd, err := validateGPUDevices(&awsEnv, host)
		if err != nil {
			return fmt.Errorf("validateGPUDevices: %w", err)
		}

		// Install Docker (only after GPU devices are validated and the ECR credentials helper is installed)
		dockerManager, err := docker.NewManager(&awsEnv, host, utils.PulumiDependsOn(installEcrCredsHelperCmd))
		if err != nil {
			return fmt.Errorf("docker.NewManager: %w", err)
		}

		// Pull all the docker images required for the tests
		dockerPullCmds, err := downloadDockerImages(&awsEnv, host, params.dockerImages, dockerManager)
		if err != nil {
			return fmt.Errorf("downloadDockerImages: %w", err)
		}

		// Validate that Docker can run CUDA samples
		dockerCudaDeps := append(dockerPullCmds, validateGPUDevicesCmd...)
		dockerCudaValidateCmd, err := validateDockerCuda(&awsEnv, host, dockerCudaDeps...)
		if err != nil {
			return fmt.Errorf("validateDockerCuda failed: %w", err)
		}

		// Combine agent options from the parameters with the fakeintake and docker dependencies
		params.agentOptions = append(params.agentOptions,
			agentparams.WithFakeintake(fakeIntake),
			agentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(dockerCudaValidateCmd)), // Depend on Docker to avoid apt lock issues
		)

		// Set updater to nil as we're not using it
		env.Updater = nil

		// Install the agent
		agent, err := agent.NewHostAgent(&awsEnv, host, params.agentOptions...)
		if err != nil {
			return fmt.Errorf("NewHostAgent failed: %w", err)
		}

		err = agent.Export(ctx, &env.Agent.HostAgentOutput)
		if err != nil {
			return fmt.Errorf("agent export failed: %w", err)
		}

		return nil
	}, nil)
}

// gpuK8sProvisioner provisions a Kubernetes cluster with GPU support
func gpuK8sProvisioner(params *provisionerParams) provisioners.Provisioner {
	provisioner := provisioners.NewTypedPulumiProvisioner[environments.Kubernetes]("gpu-k8s", func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		name := "kind" // Match the name of the kind cluster in the aws-kind provisioner so that we can reuse the DiagnoseFunc, which assumes the VM name is kind
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return fmt.Errorf("aws.NewEnvironment: %w", err)
		}

		host, err := ec2.NewVM(awsEnv, name,
			ec2.WithInstanceType(params.instanceType),
			ec2.WithAMI(params.ami, params.amiOS, os.AMD64Arch),
		)
		if err != nil {
			return fmt.Errorf("ec2.NewVM: %w", err)
		}

		installEcrCredsHelperCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, host)
		if err != nil {
			return fmt.Errorf("ec2.InstallECRCredentialsHelper %w", err)
		}

		validateDevices, err := validateGPUDevices(&awsEnv, host)
		if err != nil {
			return fmt.Errorf("validateGPUDevices: %w", err)
		}

		deps := append(validateDevices, installEcrCredsHelperCmd)

		clusterOpts := nvidia.NewKindClusterOptions(
			nvidia.WithKubeVersion(awsEnv.KubernetesVersion()),
			nvidia.WithCudaSanityCheckImage(cudaSanityCheckImage),
		)

		kindCluster, err := nvidia.NewKindCluster(&awsEnv, host, name, clusterOpts, utils.PulumiDependsOn(deps...))
		if err != nil {
			return fmt.Errorf("kubeComp.NewKindCluster: %w", err)
		}

		err = kindCluster.Export(ctx, &env.KubernetesCluster.ClusterOutput)
		if err != nil {
			return fmt.Errorf("kindCluster.Export: %w", err)
		}

		kubeProvider, err := kubernetes.NewProvider(ctx, awsEnv.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
			EnableServerSideApply: pulumi.Bool(true),
			Kubeconfig:            kindCluster.KubeConfig,
		})
		if err != nil {
			return fmt.Errorf("kubernetes.NewProvider: %w", err)
		}

		// Create the fakeintake instance
		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, name)
		if err != nil {
			return fmt.Errorf("fakeintake.NewECSFargateInstance: %w", err)
		}
		err = fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)
		if err != nil {
			return fmt.Errorf("fakeIntake.Export: %w", err)
		}

		// Pull all the docker images required for the tests
		// Note: we don't pre-pull images from the internal registry as it's not
		// available in the kind nodes
		imagesForKindNodes := []string{}
		for _, image := range params.dockerImages {
			if !strings.Contains(image, "dkr.ecr") {
				imagesForKindNodes = append(imagesForKindNodes, image)
			}
		}
		dockerPullCmds, err := downloadContainerdImagesInKindNodes(&awsEnv, host, kindCluster, imagesForKindNodes, kindCluster.GPUOperator)
		if err != nil {
			return fmt.Errorf("downloadContainerdImagesInKindNodes: %w", err)
		}

		kindClusterName := ctx.Stack()
		helmValues := fmt.Sprintf(helmValuesTemplate, kindClusterName)

		// Combine agent options from the parameters with the fakeintake and docker dependencies, and helm values
		params.kubernetesAgentOptions = append(params.kubernetesAgentOptions,
			kubernetesagentparams.WithFakeintake(fakeIntake),
			kubernetesagentparams.WithHelmValues(helmValues),
			kubernetesagentparams.WithClusterName(kindCluster.ClusterName),
			kubernetesagentparams.WithTags([]string{"stackid:" + ctx.Stack()}),
			kubernetesagentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(dockerPullCmds...)),
		)

		agent, err := helm.NewKubernetesAgent(&awsEnv, "kind", kubeProvider, params.kubernetesAgentOptions...)
		if err != nil {
			return fmt.Errorf("agent.NewKubernetesAgent: %w", err)
		}
		err = agent.Export(ctx, &env.Agent.KubernetesAgentOutput)
		if err != nil {
			return fmt.Errorf("agent.Export: %w", err)
		}

		return nil
	}, nil)

	provisioner.SetDiagnoseFunc(awskubernetes.KindDiagnoseFunc)

	return provisioner
}

// validateGPUDevices checks that there are GPU devices present and accesible
func validateGPUDevices(e *aws.Environment, vm *componentsremote.Host) ([]pulumi.Resource, error) {
	commands := map[string]string{
		"pci":    fmt.Sprintf("lspci -d %s:: | grep NVIDIA", nvidiaPCIVendorID),
		"driver": "lsmod | grep nvidia",
		"nvidia": "nvidia-smi -L | grep GPU",
	}

	var cmds []pulumi.Resource

	for name, cmd := range commands {
		c, err := vm.OS.Runner().Command(
			e.CommonNamer().ResourceName("gpu-validate", name),
			&command.Args{
				Create: pulumi.Sprintf("%s && %s", validationCommandMarker, cmd),
				Sudo:   false,
			},
		)
		if err != nil {
			return nil, err
		}
		cmds = append(cmds, c)
	}

	return cmds, nil
}

func downloadDockerImages(e *aws.Environment, vm *componentsremote.Host, images []string, dependsOn ...pulumi.Resource) ([]pulumi.Resource, error) {
	var cmds []pulumi.Resource

	for i, image := range images {
		pullCmd := makeRetryCommand(fmt.Sprintf("docker pull %s", image), dockerPullMaxRetries)
		cmd, err := vm.OS.Runner().Command(
			e.CommonNamer().ResourceName("docker-pull", strconv.Itoa(i)),
			&command.Args{
				Create: pulumi.Sprintf("/bin/bash -c \"%s\"", pullCmd),
			},
			utils.PulumiDependsOn(dependsOn...),
		)
		if err != nil {
			return nil, err
		}

		cmds = append(cmds, cmd)
	}

	return cmds, nil
}

func validateDockerCuda(e *aws.Environment, vm *componentsremote.Host, dependsOn ...pulumi.Resource) (command.Command, error) {
	return vm.OS.Runner().Command(
		e.CommonNamer().ResourceName("docker-cuda-validate"),
		&command.Args{
			Create: pulumi.Sprintf("%s && docker run --gpus all --rm %s bash -c \"%s\"", validationCommandMarker, cudaSanityCheckImage, nvidiaSMIValidationCmd),
		},
		utils.PulumiDependsOn(dependsOn...),
	)
}

func makeRetryCommand(cmd string, maxRetries int) string {
	return fmt.Sprintf("counter=0; while ! %s && [ $counter -lt %d ]; do echo failed to pull, retrying ; sleep 1; counter=$((counter+1)); done", cmd, maxRetries)
}

func downloadContainerdImagesInKindNodes(e *aws.Environment, vm *componentsremote.Host, kindCluster *nvidia.KindCluster, images []string, dependsOn ...pulumi.Resource) ([]pulumi.Resource, error) {
	var cmds []pulumi.Resource

	for i, image := range images {
		pullCmd := makeRetryCommand(fmt.Sprintf("crictl pull %s", image), dockerPullMaxRetries)
		cmd, err := vm.OS.Runner().Command(
			e.CommonNamer().ResourceName("kind-node-pull", fmt.Sprintf("image-%d", i)),
			&command.Args{
				Create: pulumi.Sprintf("kind get nodes --name %s | xargs -I {} docker exec {} /bin/bash -c \"%s\"", kindCluster.ClusterName, pullCmd),
			},
			utils.PulumiDependsOn(dependsOn...),
		)
		if err != nil {
			return nil, err
		}

		cmds = append(cmds, cmd)
	}

	return cmds, nil
}
