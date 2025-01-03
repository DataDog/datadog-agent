// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package gpu

import (
	"fmt"
	"strconv"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/docker"
	"github.com/DataDog/test-infra-definitions/components/os"
	componentsremote "github.com/DataDog/test-infra-definitions/components/remote"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
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

const defaultGpuCheckConfig = `
init_config:
  min_collection_interval: 5

instances:
  - {}
`

const defaultSysprobeConfig = `
gpu_monitoring:
  enabled: true
`

type provisionerParams struct {
	agentOptions []agentparams.Option
	ami          string
	amiOS        os.Descriptor
	instanceType string
	dockerImages []string
}

func getDefaultProvisionerParams() *provisionerParams {
	return &provisionerParams{
		agentOptions: []agentparams.Option{
			agentparams.WithIntegration("gpu.d", defaultGpuCheckConfig),
			agentparams.WithSystemProbeConfig(defaultSysprobeConfig),
		},
		ami:          gpuEnabledAMI,
		amiOS:        os.Ubuntu2204,
		instanceType: gpuInstanceType,
		dockerImages: []string{cudaSanityCheckImage},
	}
}

func gpuInstanceProvisioner(params *provisionerParams) provisioners.Provisioner {
	return provisioners.NewTypedPulumiProvisioner[environments.Host]("gpu", func(ctx *pulumi.Context, env *environments.Host) error {
		name := "gpuvm"
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		// Create the EC2 instance
		host, err := ec2.NewVM(awsEnv, name,
			ec2.WithInstanceType(params.instanceType),
			ec2.WithAMI(params.ami, params.amiOS, os.AMD64Arch),
		)
		if err != nil {
			return err
		}
		err = host.Export(ctx, &env.RemoteHost.HostOutput)
		if err != nil {
			return err
		}

		// Create the fakeintake instance
		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, name)
		if err != nil {
			return err
		}
		err = fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)
		if err != nil {
			return err
		}

		// install the ECR credentials helper
		// required to get pipeline agent images or other internally hosted images
		installEcrCredsHelperCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, host)
		if err != nil {
			return err
		}

		// Validate GPU devices
		validateGPUDevicesCmd, err := validateGPUDevices(awsEnv, host)
		if err != nil {
			return err
		}

		// Install Docker (only after GPU devices are validated and the ECR credentials helper is installed)
		dockerManager, err := docker.NewManager(&awsEnv, host, utils.PulumiDependsOn(installEcrCredsHelperCmd))
		if err != nil {
			return err
		}

		// Pull all the docker images required for the tests
		dockerPullCmds, err := downloadDockerImages(awsEnv, host, params.dockerImages, dockerManager)
		if err != nil {
			return err
		}

		// Validate that Docker can run CUDA samples
		dockerCudaDeps := append(dockerPullCmds, validateGPUDevicesCmd...)
		dockerCudaValidateCmd, err := validateDockerCuda(awsEnv, host, dockerCudaDeps...)
		if err != nil {
			return fmt.Errorf("validateDockerCuda failed: %w", err)
		}
		// incident-33572: log the output of the CUDA validation command
		pulumi.All(dockerCudaValidateCmd.StdoutOutput(), dockerCudaValidateCmd.StderrOutput()).ApplyT(func(args []interface{}) error {
			stdout := args[0].(string)
			stderr := args[1].(string)
			err := ctx.Log.Info(fmt.Sprintf("Docker CUDA validation stdout: %s", stdout), nil)
			if err != nil {
				return err
			}
			err = ctx.Log.Info(fmt.Sprintf("Docker CUDA validation stderr: %s", stderr), nil)
			if err != nil {
				return err
			}
			return nil
		})

		// Combine agent options from the parameters with the fakeintake and docker dependencies
		params.agentOptions = append(params.agentOptions,
			agentparams.WithFakeintake(fakeIntake),
			agentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(dockerManager, dockerCudaValidateCmd)), // Depend on Docker to avoid apt lock issues
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

// validateGPUDevices checks that there are GPU devices present and accesible
func validateGPUDevices(e aws.Environment, vm *componentsremote.Host) ([]pulumi.Resource, error) {
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

func downloadDockerImages(e aws.Environment, vm *componentsremote.Host, images []string, dependsOn ...pulumi.Resource) ([]pulumi.Resource, error) {
	var cmds []pulumi.Resource

	for i, image := range images {
		cmd, err := vm.OS.Runner().Command(
			e.CommonNamer().ResourceName("docker-pull", strconv.Itoa(i)),
			&command.Args{
				Create: pulumi.Sprintf("docker pull %s", image),
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

func validateDockerCuda(e aws.Environment, vm *componentsremote.Host, dependsOn ...pulumi.Resource) (command.Command, error) {
	return vm.OS.Runner().Command(
		e.CommonNamer().ResourceName("docker-cuda-validate"),
		&command.Args{
			Create: pulumi.Sprintf("%s && docker run --gpus all --rm %s bash -c \"%s\"", validationCommandMarker, cudaSanityCheckImage, nvidiaSMIValidationCmd),
		},
		utils.PulumiDependsOn(dependsOn...),
	)
}
