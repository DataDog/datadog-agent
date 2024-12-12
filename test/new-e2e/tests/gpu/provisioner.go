// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package gpu

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/docker"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/components/remote"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"
	goremote "github.com/pulumi/pulumi-command/sdk/go/command/remote"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
)

const gpuEnabledAMI = "ami-0f71e237bb2ba34be" // Ubuntu 22.04 with GPU drivers
const gpuInstanceType = "g4dn.xlarge"
const nvidiaPCIVendorID = "10de"

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
	}
}

func gpuInstanceProvisioner(params *provisionerParams) e2e.Provisioner {
	return e2e.NewTypedPulumiProvisioner[environments.Host]("gpu", func(ctx *pulumi.Context, env *environments.Host) error {
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
		dockerManager, err := docker.NewManager(&awsEnv, host, utils.PulumiDependsOn(installEcrCredsHelperCmd, validateGPUDevicesCmd))
		if err != nil {
			return err
		}

		// Combine agent options from the parameters with the fakeintake and docker dependencies
		params.agentOptions = append(params.agentOptions,
			agentparams.WithFakeintake(fakeIntake),
			agentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(dockerManager)), // Depend on Docker to avoid apt lock issues
		)

		// Set updater to nil as we're not using it, and
		env.Updater = nil

		// Install the agent
		agent, err := agent.NewHostAgent(&awsEnv, host, params.agentOptions...)
		if err != nil {
			return err
		}

		err = agent.Export(ctx, &env.Agent.HostAgentOutput)
		if err != nil {
			return err
		}

		return nil
	}, nil)
}

// validateGPUDevices checks that there are GPU devices present and accesible
func validateGPUDevices(e aws.Environment, vm *remote.Host) (*goremote.Command, error) {
	pciValidate, err := vm.OS.Runner().Command(
		e.CommonNamer().ResourceName("pci-validate"),
		&command.Args{
			Create: pulumi.Sprintf("lspci -d %s:: | grep NVIDIA", nvidiaPCIVendorID),
			Sudo:   false,
		},
	)
	if err != nil {
		return nil, err
	}

	nvidiaValidate, err := vm.OS.Runner().Command(
		e.CommonNamer().ResourceName("nvidia-validate"),
		&command.Args{
			Create: pulumi.Sprintf("nvidia-smi -L | grep GPU"),
			Sudo:   false,
		},
		utils.PulumiDependsOn(pciValidate),
	)
	if err != nil {
		return nil, err
	}

	return nvidiaValidate, nil
}
