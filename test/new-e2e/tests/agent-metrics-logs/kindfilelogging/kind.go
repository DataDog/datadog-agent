// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package kindfilelogging spins up the same pulumi environment as the awskubernetes package but
// includes a logger container as well
package kindfilelogging

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"

	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "aws-kind-"
	defaultVMName     = "kind"
)

// ProvisionerParams contains all the parameters needed to create the environment
type ProvisionerParams struct {
	name              string
	vmOptions         []ec2.VMOption
	agentOptions      []kubernetesagentparams.Option
	fakeintakeOptions []fakeintake.Option
	extraConfigParams runner.ConfigMap
}

func newProvisionerParams() *ProvisionerParams {
	return &ProvisionerParams{
		name:              defaultVMName,
		vmOptions:         []ec2.VMOption{},
		agentOptions:      []kubernetesagentparams.Option{},
		fakeintakeOptions: []fakeintake.Option{},
		extraConfigParams: runner.ConfigMap{},
	}
}

// ProvisionerOption is a function that modifies the ProvisionerParams
type ProvisionerOption func(*ProvisionerParams) error

// WithName sets the name of the provisioner
func WithName(name string) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.name = name
		return nil
	}
}

// WithEC2VMOptions adds options to the EC2 VM
func WithEC2VMOptions(opts ...ec2.VMOption) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.vmOptions = opts
		return nil
	}
}

// WithAgentOptions adds options to the agent
func WithAgentOptions(opts ...kubernetesagentparams.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = opts
		return nil
	}
}

// WithFakeIntakeOptions adds options to the fake intake
func WithFakeIntakeOptions(opts ...fakeintake.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = opts
		return nil
	}
}

// WithoutFakeIntake removes the fake intake
func WithoutFakeIntake() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = nil
		return nil
	}
}

// WithoutAgent removes the agent
func WithoutAgent() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = nil
		return nil
	}
}

// WithExtraConfigParams adds extra config parameters to the environment
func WithExtraConfigParams(configMap runner.ConfigMap) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.extraConfigParams = configMap
		return nil
	}
}

// Provisioner creates a new provisioner
func Provisioner(opts ...ProvisionerOption) e2e.TypedProvisioner[environments.Kubernetes] {
	// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
	// and it's easy to forget about it, leading to issues that are hard to debug.
	params := newProvisionerParams()
	_ = optional.ApplyOptions(params, opts)

	provisioner := e2e.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to issues that are hard to debug.
		params := newProvisionerParams()
		_ = optional.ApplyOptions(params, opts)

		return KindRunFunc(ctx, env, params)
	}, params.extraConfigParams)

	return provisioner
}

// KindRunFunc is the Pulumi run function that runs the provisioner
func KindRunFunc(ctx *pulumi.Context, env *environments.Kubernetes, params *ProvisionerParams) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return fmt.Errorf("aws.NewEnvironment: %w", err)
	}

	host, err := ec2.NewVM(awsEnv, params.name, params.vmOptions...)
	if err != nil {
		return fmt.Errorf("ec2.NewVM: %w", err)
	}

	installEcrCredsHelperCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, host)
	if err != nil {
		return fmt.Errorf("ec2.InstallECRCredentialsHelper %w", err)
	}

	kindCluster, err := kubeComp.NewKindCluster(&awsEnv, host, awsEnv.CommonNamer().ResourceName("kind"), params.name, awsEnv.KubernetesVersion(), utils.PulumiDependsOn(installEcrCredsHelperCmd))
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

	if params.fakeintakeOptions != nil {
		fakeintakeOpts := []fakeintake.Option{fakeintake.WithLoadBalancer()}
		params.fakeintakeOptions = append(fakeintakeOpts, params.fakeintakeOptions...)
		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, params.name, params.fakeintakeOptions...)
		if err != nil {
			return fmt.Errorf("fakeintake.NewECSFargateInstance: %w", err)
		}
		err = fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)
		if err != nil {
			return fmt.Errorf("fakeIntake.Export: %w", err)
		}

		if params.agentOptions != nil {
			newOpts := []kubernetesagentparams.Option{kubernetesagentparams.WithFakeintake(fakeIntake)}
			params.agentOptions = append(newOpts, params.agentOptions...)
		}
	} else {
		env.FakeIntake = nil
	}

	if params.agentOptions != nil {
		kindClusterName := ctx.Stack()
		helmValues := fmt.Sprintf(`
datadog:
  kubelet:
    tlsVerify: false
  clusterName: "%s"
  envDict:
    DD_CONTAINER_EXCLUDE: "kube_namespace:^exclude-namespace$"
agents:
  useHostNetwork: true
`, kindClusterName)

		newOpts := []kubernetesagentparams.Option{kubernetesagentparams.WithHelmValues(helmValues)}
		params.agentOptions = append(newOpts, params.agentOptions...)
		agent, err := agent.NewKubernetesAgent(&awsEnv, kindClusterName, kubeProvider, params.agentOptions...)
		if err != nil {
			return fmt.Errorf("agent.NewKubernetesAgent: %w", err)
		}
		err = agent.Export(ctx, &env.Agent.KubernetesAgentOutput)
		if err != nil {
			return fmt.Errorf("agent.Export: %w", err)
		}

	} else {
		env.Agent = nil
	}

	return nil
}
