// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package winawshost contains the definition of the AWS Windows Host environment.
package winawshost

import (
	"fmt"
	sysos "os"

	"github.com/DataDog/test-infra-definitions/components/activedirectory"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	installer "github.com/DataDog/datadog-agent/test/new-e2e/pkg/components/datadog-installer"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/defender"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/fipsmode"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/testsigning"
)

const (
	provisionerBaseID = "aws-ec2vm-"
	defaultVMName     = "vm"
)

// ProvisionerParams is a set of parameters for the Provisioner.
type ProvisionerParams struct {
	name string

	instanceOptions        []ec2.VMOption
	agentOptions           []agentparams.Option
	agentClientOptions     []agentclientparams.Option
	fakeintakeOptions      []fakeintake.Option
	activeDirectoryOptions []activedirectory.Option
	defenderoptions        []defender.Option
	installerOptions       []installer.Option
	fipsModeOptions        []fipsmode.Option
	testsigningOptions     []testsigning.Option
}

// ProvisionerOption is a provisioner option.
type ProvisionerOption func(*ProvisionerParams) error

// WithName sets the name of the provisioner.
func WithName(name string) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.name = name
		return nil
	}
}

// WithEC2InstanceOptions adds options to the EC2 VM.
func WithEC2InstanceOptions(opts ...ec2.VMOption) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.instanceOptions = append(params.instanceOptions, opts...)
		return nil
	}
}

// WithAgentOptions adds options to the Agent.
func WithAgentOptions(opts ...agentparams.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = append(params.agentOptions, opts...)
		return nil
	}
}

// WithoutAgent disables the creation of the Agent.
func WithoutAgent() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = nil
		return nil
	}
}

// WithAgentClientOptions adds options to the Agent client.
func WithAgentClientOptions(opts ...agentclientparams.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentClientOptions = append(params.agentClientOptions, opts...)
		return nil
	}
}

// WithFakeIntakeOptions adds options to the FakeIntake.
func WithFakeIntakeOptions(opts ...fakeintake.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = append(params.fakeintakeOptions, opts...)
		return nil
	}
}

// WithoutFakeIntake disables the creation of the FakeIntake.
func WithoutFakeIntake() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = nil
		return nil
	}
}

// WithActiveDirectoryOptions adds Active Directory to the EC2 VM.
func WithActiveDirectoryOptions(opts ...activedirectory.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.activeDirectoryOptions = append(params.activeDirectoryOptions, opts...)
		return nil
	}
}

// WithDefenderOptions configures Windows Defender on an EC2 VM.
func WithDefenderOptions(opts ...defender.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.defenderoptions = append(params.defenderoptions, opts...)
		return nil
	}
}

// WithFIPSModeOptions configures FIPS mode on an EC2 VM.
//
// Ordered before the Agent setup.
func WithFIPSModeOptions(opts ...fipsmode.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fipsModeOptions = append(params.fipsModeOptions, opts...)
		return nil
	}
}

// WithTestSigningOptions configures TestSigning on an EC2 VM.
func WithTestSigningOptions(opts ...testsigning.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.testsigningOptions = append(params.testsigningOptions, opts...)
		return nil
	}
}

// WithInstaller configures Datadog Installer on an EC2 VM.
func WithInstaller(opts ...installer.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.installerOptions = []installer.Option{}
		params.installerOptions = append(params.installerOptions, opts...)
		return nil
	}
}

// Run deploys a Windows environment given a pulumi.Context
func Run(ctx *pulumi.Context, env *environments.WindowsHost, params *ProvisionerParams) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env.Environment = &awsEnv

	// Make sure to override any OS other than Windows
	// TODO: Make the Windows version configurable
	params.instanceOptions = append(params.instanceOptions, ec2.WithOS(os.WindowsDefault))

	host, err := ec2.NewVM(awsEnv, params.name, params.instanceOptions...)
	if err != nil {
		return err
	}
	err = host.Export(ctx, &env.RemoteHost.HostOutput)
	if err != nil {
		return err
	}

	if params.defenderoptions != nil {
		defender, err := defender.NewDefender(awsEnv.CommonEnvironment, host, params.defenderoptions...)
		if err != nil {
			return err
		}
		// Active Directory setup needs to happen after Windows Defender setup
		params.activeDirectoryOptions = append(params.activeDirectoryOptions,
			activedirectory.WithPulumiResourceOptions(
				pulumi.DependsOn(defender.Resources)))
	}

	if params.testsigningOptions != nil {
		fmt.Println("Enabling test signing")
		fmt.Println("testsigningOptions", params.testsigningOptions)
		testsigning, err := testsigning.NewTestSigning(awsEnv.CommonEnvironment, host, params.testsigningOptions...)
		if err != nil {
			return err
		}
		// Active Directory setup needs to happen after TestSigning setup
		params.activeDirectoryOptions = append(params.activeDirectoryOptions,
			activedirectory.WithPulumiResourceOptions(
				pulumi.DependsOn(testsigning.Resources)))
	}

	if params.activeDirectoryOptions != nil {
		activeDirectoryComp, activeDirectoryResources, err := activedirectory.NewActiveDirectory(ctx, &awsEnv, host, params.activeDirectoryOptions...)
		if err != nil {
			return err
		}
		err = activeDirectoryComp.Export(ctx, &env.ActiveDirectory.Output)
		if err != nil {
			return err
		}

		if params.agentOptions != nil {
			// Agent install needs to happen after ActiveDirectory setup
			params.agentOptions = append(params.agentOptions,
				agentparams.WithPulumiResourceOptions(
					pulumi.DependsOn(activeDirectoryResources)))
		}
	} else {
		// Suite inits all fields by default, so we need to explicitly set it to nil
		env.ActiveDirectory = nil
	}

	// Create FakeIntake if required
	if params.fakeintakeOptions != nil {
		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, params.name, params.fakeintakeOptions...)
		if err != nil {
			return err
		}
		err = fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)
		if err != nil {
			return err
		}
		// Normally if FakeIntake is enabled, Agent is enabled, but just in case
		if params.agentOptions != nil {
			// Prepend in case it's overridden by the user
			newOpts := []agentparams.Option{agentparams.WithFakeintake(fakeIntake)}
			params.agentOptions = append(newOpts, params.agentOptions...)
		}
	} else {
		env.FakeIntake = nil
	}

	if params.agentOptions != nil {
		agentOptions := append(params.agentOptions, agentparams.WithTags([]string{fmt.Sprintf("stackid:%s", ctx.Stack())}))
		agent, err := agent.NewHostAgent(&awsEnv, host, agentOptions...)
		if err != nil {
			return err
		}
		err = agent.Export(ctx, &env.Agent.HostAgentOutput)
		if err != nil {
			return err
		}
		env.Agent.ClientOptions = params.agentClientOptions
	} else {
		env.Agent = nil
	}

	if params.installerOptions != nil {
		installer, err := installer.NewInstaller(&awsEnv, host, params.installerOptions...)
		if err != nil {
			return err
		}
		err = installer.Export(ctx, &env.Installer.Output)
		if err != nil {
			return err
		}
	} else {
		env.Installer = nil
	}

	if params.fipsModeOptions != nil {
		fipsMode, err := fipsmode.New(awsEnv.CommonEnvironment, host, params.fipsModeOptions...)
		if err != nil {
			return err
		}
		// We want Agent setup to happen after FIPS mode setup, but only
		// because that's the use case we are interested in.
		// Ideally the provisioner would allow the user to specify the order of
		// the resources, but that's not supported right now.
		params.agentOptions = append(params.agentOptions,
			agentparams.WithPulumiResourceOptions(
				pulumi.DependsOn(fipsMode.Resources)))
	}

	return nil
}

func getProvisionerParams(opts ...ProvisionerOption) *ProvisionerParams {
	params := &ProvisionerParams{
		name:               defaultVMName,
		instanceOptions:    []ec2.VMOption{},
		agentOptions:       []agentparams.Option{},
		agentClientOptions: []agentclientparams.Option{},
		fakeintakeOptions:  []fakeintake.Option{},
		// Disable Windows Defender on VMs by default
		defenderoptions:    []defender.Option{defender.WithDefenderDisabled()},
		fipsModeOptions:    []fipsmode.Option{},
		testsigningOptions: []testsigning.Option{},
	}

	// check env and enable test signing if we have a test signed driver
	if sysos.Getenv("WINDOWS_DDNPM_DRIVER") == "testsigned" || sysos.Getenv("WINDOWS_DDPROCMON_DRIVER") == "testsigned" {
		params.testsigningOptions = append(params.testsigningOptions, testsigning.WithTestSigningEnabled())
	}

	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Errorf("unable to apply ProvisionerOption, err: %w", err))
	}
	fmt.Println("testsigningOptions", params.testsigningOptions)
	return params
}

// Provisioner creates a VM environment with a Windows EC2 VM, an ECS Fargate FakeIntake and a Host Agent configured to talk to each other.
// FakeIntake and Agent creation can be deactivated by using [WithoutFakeIntake] and [WithoutAgent] options.
func Provisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.WindowsHost] {
	// We need to build params here to be able to use params.name in the provisioner name
	params := getProvisionerParams(opts...)
	provisioner := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.WindowsHost) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := getProvisionerParams(opts...)
		return Run(ctx, env, params)
	}, nil)

	return provisioner
}

// ProvisionerNoAgent wraps Provisioner with hardcoded WithoutAgent options.
func ProvisionerNoAgent(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.WindowsHost] {
	mergedOpts := make([]ProvisionerOption, 0, len(opts)+1)
	mergedOpts = append(mergedOpts, opts...)
	mergedOpts = append(mergedOpts, WithoutAgent())

	return Provisioner(mergedOpts...)
}

// ProvisionerNoAgentNoFakeIntake wraps Provisioner with hardcoded WithoutAgent and WithoutFakeIntake options.
func ProvisionerNoAgentNoFakeIntake(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.WindowsHost] {
	mergedOpts := make([]ProvisionerOption, 0, len(opts)+2)
	mergedOpts = append(mergedOpts, opts...)
	mergedOpts = append(mergedOpts, WithoutAgent(), WithoutFakeIntake())

	return Provisioner(mergedOpts...)
}

// ProvisionerNoFakeIntake wraps Provisioner with hardcoded WithoutFakeIntake option.
func ProvisionerNoFakeIntake(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.WindowsHost] {
	mergedOpts := make([]ProvisionerOption, 0, len(opts)+1)
	mergedOpts = append(mergedOpts, opts...)
	mergedOpts = append(mergedOpts, WithoutFakeIntake())

	return Provisioner(mergedOpts...)
}
