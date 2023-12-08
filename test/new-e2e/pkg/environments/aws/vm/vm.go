package awsvm

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "aws-ec2vm-"
	defaultVMName     = "vm"
)

type ProvisionerParams struct {
	name string

	vmOptions         []ec2.VMOption
	agentOptions      []agentparams.Option
	fakeintakeOptions []fakeintake.Option
	extraConfigParams runner.ConfigMap
}

func newProvisionerParams() *ProvisionerParams {
	// We use nil arrays to decide if we should create or not
	return &ProvisionerParams{
		name:              defaultVMName,
		vmOptions:         []ec2.VMOption{},
		agentOptions:      []agentparams.Option{},
		fakeintakeOptions: []fakeintake.Option{},
		extraConfigParams: runner.ConfigMap{},
	}
}

type ProvisionerOption func(*ProvisionerParams) error

func WithName(name string) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.name = name
		return nil
	}
}

func WithEC2VMOptions(opts ...ec2.VMOption) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.vmOptions = append(params.vmOptions, opts...)
		return nil
	}
}

func WithAgentOptions(opts ...agentparams.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = append(params.agentOptions, opts...)
		return nil
	}
}

func WithFakeIntakeOptions(opts ...fakeintake.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = append(params.fakeintakeOptions, opts...)
		return nil
	}
}

func WithExtraConfigParams(configMap runner.ConfigMap) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.extraConfigParams = configMap
		return nil
	}
}

func WithoutFakeIntake() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = nil
		return nil
	}
}

func WithoutAgent() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = nil
		params.fakeintakeOptions = nil
		return nil
	}
}

// Provisioner creates a VM environment with an EC2 VM, an ECS Fargate FakeIntake and a Host Agent configured to talk to each other.
// FakeIntake and Agent creation can be deactivated by using [WithoutFakeIntake] and [WithoutAgent] options.
func Provisioner(opts ...ProvisionerOption) e2e.TypedProvisioner[environments.VM] {
	params := newProvisionerParams()
	err := optional.ApplyOptions(params, opts)

	provisioner := e2e.NewPulumiTypedProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.VM) error {
		// We are abusing Pulumi RunFunc error to return our parameter parsing error, in the sake of the slightly simpler API.
		if err != nil {
			return err
		}

		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		host, err := ec2.NewVM(awsEnv, params.name, params.vmOptions...)
		if err != nil {
			return err
		}
		host.Export(ctx, &env.Host.HostOutput)

		// Create FakeIntake if required
		if params.fakeintakeOptions != nil {
			fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, params.name, params.fakeintakeOptions...)
			if err != nil {
				return err
			}
			fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)

			// Normally if FakeIntake is enabled, Agent is enabled, but just in case
			if params.agentOptions != nil {
				// Prepend in case it's overriden by the user
				newOpts := []agentparams.Option{agentparams.WithFakeintake(fakeIntake)}
				params.agentOptions = append(newOpts, params.agentOptions...)
			}
		} else {
			// Suite inits all fields by default, so we need to explicitly set it to nil
			env.FakeIntake = nil
		}

		// Create Agent if required
		if params.agentOptions != nil {
			agent, err := agent.NewHostAgent(awsEnv.CommonEnvironment, host, params.agentOptions...)
			if err != nil {
				return err
			}
			agent.Export(ctx, &env.Agent.HostAgentOutput)
		} else {
			// Suite inits all fields by default, so we need to explicitly set it to nil
			env.Agent = nil
		}

		return nil
	}, params.extraConfigParams)

	return provisioner
}
