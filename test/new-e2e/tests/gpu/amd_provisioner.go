// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gpu

import (
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	componentsremote "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

// amdGPUInstanceType is the cheapest AWS instance with an AMD GPU: a Radeon Pro
// V520 (Navi 12) virtual function, driven by the in-tree amdgpu driver.
const amdGPUInstanceType = "g4ad.xlarge"

// amdGPUOS is a stock image: the GPU check reads amdgpu from sysfs and needs
// neither ROCm nor amd-smi, so only the kernel driver has to be installed.
var amdGPUOS = os.Ubuntu2404

// amdAgentConfig enables the GPU check without system-probe, which only
// instruments NVIDIA (CUDA) workloads.
const amdAgentConfig = `log_level: DEBUG
gpu:
  enabled: true
`

// amdDriverSetup loads the in-tree amdgpu driver. Cloud kernels ship it in
// linux-modules-extra, and its firmware is in linux-firmware. The lock timeout
// waits for unattended-upgrades on first boot.
const amdDriverSetup = `set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
sudo apt-get -o DPkg::Lock::Timeout=600 update -q
sudo apt-get -o DPkg::Lock::Timeout=600 install -y -q "linux-modules-extra-$(uname -r)" linux-firmware
sudo modprobe amdgpu
`

// amdGPUHostProvisioner provisions an EC2 instance with an AMD GPU and an Agent
// reporting to a fakeintake.
func amdGPUHostProvisioner() provisioners.Provisioner {
	return provisioners.NewTypedPulumiProvisioner[environments.Host]("amdgpu", func(ctx *pulumi.Context, env *environments.Host) error {
		name := "amdgpuvm"
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return fmt.Errorf("aws.NewEnvironment: %w", err)
		}

		host, err := ec2.NewVM(awsEnv, name,
			ec2.WithInstanceType(amdGPUInstanceType),
			ec2.WithOS(amdGPUOS),
		)
		if err != nil {
			return fmt.Errorf("ec2.NewVM: %w", err)
		}
		if err := host.Export(ctx, &env.RemoteHost.HostOutput); err != nil {
			return fmt.Errorf("host.Export: %w", err)
		}

		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, name)
		if err != nil {
			return fmt.Errorf("fakeintake.NewECSFargateInstance: %w", err)
		}
		if err := fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput); err != nil {
			return fmt.Errorf("fakeIntake.Export: %w", err)
		}

		validated, err := setupAMDGPUDriver(&awsEnv, host)
		if err != nil {
			return fmt.Errorf("setupAMDGPUDriver: %w", err)
		}

		env.Updater = nil
		hostAgent, err := agent.NewHostAgent(&awsEnv, host,
			agentparams.WithAgentConfig(amdAgentConfig),
			agentparams.WithFakeintake(fakeIntake),
			// The agent is installed after the driver to avoid apt lock contention,
			// and so that the GPU is already present when the check first runs.
			agentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(validated)),
		)
		if err != nil {
			return fmt.Errorf("NewHostAgent: %w", err)
		}
		if err := hostAgent.Export(ctx, &env.Agent.HostAgentOutput); err != nil {
			return fmt.Errorf("agent export: %w", err)
		}
		return nil
	}, nil)
}

// setupAMDGPUDriver installs and loads amdgpu, then checks that a DRM card is
// an AMD PCI device (vendor 0x1002) bound to amdgpu: the condition under which
// the GPU check discovers it.
func setupAMDGPUDriver(e *aws.Environment, vm *componentsremote.Host) (command.Command, error) {
	install, err := vm.OS.Runner().Command(
		e.CommonNamer().ResourceName("amdgpu-driver-install"),
		&command.Args{
			Create: pulumi.Sprintf("bash -c '%s'", amdDriverSetup),
		},
	)
	if err != nil {
		return nil, err
	}

	return vm.OS.Runner().Command(
		e.CommonNamer().ResourceName("amdgpu-validate"),
		&command.Args{
			Create: pulumi.Sprintf("%s && for d in /sys/class/drm/card*/device; do "+
				"grep -qx 0x1002 \"$d/vendor\" && [ \"$(basename \"$(readlink -f \"$d/driver\")\")\" = amdgpu ] && exit 0; "+
				"done; echo 'no AMD GPU bound to amdgpu'; lspci -nnk -d 1002::; exit 1", validationCommandMarker),
		},
		utils.PulumiDependsOn(install),
	)
}
