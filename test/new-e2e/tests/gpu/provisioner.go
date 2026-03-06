// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package gpu

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/nvidia"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	componentsremote "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awskubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

//go:embed testdata/config/agent_config.yaml
var agentConfigStr string

//go:embed testdata/config/system_probe_config.yaml
var systemProbeConfigStr string

type systemData struct {
	ami string
	os  os.Descriptor

	// cudaSanityCheckImage is a Docker image that contains a CUDA sample to
	// validate the GPU setup with the default CUDA installation. Note that the CUDA
	// version in this image must be equal or less than the one installed in the
	// AMI.
	cudaSanityCheckImage string

	// hasEcrCredentialsHelper is true if the system has the ECR credentials helper installed
	// or if it needs to be installed from the repos
	hasEcrCredentialsHelper bool

	// hasAllNVMLCriticalAPIs is true if the system has all the critical APIs in NVML
	// that we need to run the GPU check.
	hasAllNVMLCriticalAPIs bool

	// supportsSystemProbeComponent is true if the system supports the system-probe component
	// that is used to collect GPU metrics. Some systems have older kernels that we don't support.
	supportsSystemProbeComponent bool

	// cudaVersion is the version of CUDA installed in the system, will be used to validate the installation
	// This avoids weird compatibility issues that can arise without explicit errors.
	cudaVersion string
}

type systemName string

// gpuInstanceType is the instance type to use. By default we use g4dn.xlarge,
// which is the cheapest GPU instance type
const gpuInstanceType = "g4dn.xlarge"

// nvidiaPCIVendorID is the PCI vendor ID for NVIDIA GPUs, used to identify the
// GPU devices with lspci
const nvidiaPCIVendorID = "10de"

// nvidiaSMIValidationCmd is a command that checks if the nvidia-smi command is
// available and can list the GPUs
const nvidiaSMIValidationCmd = "nvidia-smi -L | grep GPU"

// validationCommandMarker is a command that can be appended to all validation commands
// to identify them in the output, which can be useful to later force retries. Retries
// are controlled in test/e2e-framework/testing/utils/infra/retriable_errors.go, and the way to
// identify them are based on the pulumi logs. This command will be echoed to the output
// and can be used to identify the validation commands.
const validationCommandMarker = "echo 'gpu-validation-command'"

const helmValuesTemplate = `
datadog:
  kubelet:
    tlsVerify: false
  clusterName: "%s"
  gpuMonitoring:
    enabled: true
    privilegedMode: true
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
    agent:
      env:
        - name: DD_GPU_ENABLED
          value: "true"
        - name: DD_GPU_USE_SP_PROCESS_METRICS
          value: "true"
`

const ddAgentSetup = `#!/bin/bash
# /var/run/datadog directory is necessary for UDS socket creation
sudo mkdir -p /var/run/datadog
sudo groupadd -r dd-agent
sudo useradd -r -M -g dd-agent dd-agent
sudo chown dd-agent:dd-agent /var/run/datadog

# Agent must be in the docker group to be able to open and read
# container info from the docker socket.
sudo groupadd -f -r docker
sudo usermod -a -G docker dd-agent
`

const dockerPullMaxRetries = 3

type provisionerParams struct {
	agentOptions           []agentparams.Option
	kubernetesAgentOptions []kubernetesagentparams.Option
	systemData             systemData
	instanceType           string
	dockerImages           []string
}

func getDefaultProvisionerParams() *provisionerParams {
	return &provisionerParams{
		agentOptions: []agentparams.Option{
			agentparams.WithSystemProbeConfig(systemProbeConfigStr),
			agentparams.WithAgentConfig(agentConfigStr),
		},
		kubernetesAgentOptions: nil,
		instanceType:           gpuInstanceType,
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
			ec2.WithAMI(params.systemData.ami, params.systemData.os, os.AMD64Arch),
			ec2.WithUserData(ddAgentSetup),
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

		// Validate GPU devices
		validateGPUDevicesCmd, err := validateGPUDevices(&awsEnv, host, params.systemData.cudaVersion)
		if err != nil {
			return fmt.Errorf("validateGPUDevices: %w", err)
		}

		// Install Docker (only after GPU devices are validated and the ECR credentials helper is installed)
		dockerManager, err := docker.NewManager(&awsEnv, host)
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
		dockerCudaValidateCmd, err := validateDockerCuda(&awsEnv, host, params.systemData.cudaSanityCheckImage, dockerCudaDeps...)
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
			ec2.WithAMI(params.systemData.ami, params.systemData.os, os.AMD64Arch),
		)
		if err != nil {
			return fmt.Errorf("ec2.NewVM: %w", err)
		}

		installEcrCredsHelperCmd, err := docker.InstallECRCredentialsHelper(awsEnv.Namer, host)
		if err != nil {
			return fmt.Errorf("docker.InstallECRCredentialsHelper %w", err)
		}

		validateDevices, err := validateGPUDevices(&awsEnv, host, params.systemData.cudaVersion)
		if err != nil {
			return fmt.Errorf("validateGPUDevices: %w", err)
		}

		deps := append(validateDevices, installEcrCredsHelperCmd)

		clusterOpts := nvidia.NewKindClusterOptions(
			nvidia.WithKubeVersion(awsEnv.KubernetesVersion()),
			nvidia.WithCudaSanityCheckImage(params.systemData.cudaSanityCheckImage),
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

	provisioner.SetDiagnoseFunc(awskubernetes.DiagnoseFunc)

	return provisioner
}

// validateGPUDevices checks that there are GPU devices present and accesible
func validateGPUDevices(e *aws.Environment, vm *componentsremote.Host, cudaVersion string) ([]pulumi.Resource, error) {
	commands := map[string]string{
		"pci":            fmt.Sprintf("lspci -d %s:: | grep NVIDIA", nvidiaPCIVendorID),
		"driver":         "lsmod | grep nvidia",
		"nvidia":         "nvidia-smi -L | grep GPU",
		"driver-version": "cat /proc/driver/nvidia/version | grep NVRM",
		"cuda-version":   fmt.Sprintf("nvidia-smi | grep 'CUDA Version:' | grep %s ; nvidia-smi | grep 'CUDA Version:'", cudaVersion), // print the cuda version for debugging even if there's not a match
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
		pullCmd := makeRetryCommand("docker pull "+image, dockerPullMaxRetries)
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

func validateDockerCuda(e *aws.Environment, vm *componentsremote.Host, cudaSanityCheckImage string, dependsOn ...pulumi.Resource) (command.Command, error) {
	return vm.OS.Runner().Command(
		e.CommonNamer().ResourceName("docker-cuda-validate"),
		&command.Args{
			Create: pulumi.Sprintf("%s && docker run --gpus all --rm %s bash -c \"%s\"", validationCommandMarker, cudaSanityCheckImage, nvidiaSMIValidationCmd),
		},
		utils.PulumiDependsOn(dependsOn...),
	)
}

func makeRetryCommand(cmd string, maxRetries int) string {
	return fmt.Sprintf("counter=0; while [ \\$counter -lt %d ] && ! %s ; do echo failed to pull, retrying ; sleep 1; counter=\\$((counter+1)); done ; if [ \\$counter -eq %d ]; then echo 'cannot pull image, maximum number of retries reached'; exit 1; fi", maxRetries, cmd, maxRetries)
}

func downloadContainerdImagesInKindNodes(e *aws.Environment, vm *componentsremote.Host, kindCluster *nvidia.KindCluster, images []string, dependsOn ...pulumi.Resource) ([]pulumi.Resource, error) {
	var cmds []pulumi.Resource

	for i, image := range images {
		pullCmd := makeRetryCommand("crictl pull "+image, dockerPullMaxRetries)
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
