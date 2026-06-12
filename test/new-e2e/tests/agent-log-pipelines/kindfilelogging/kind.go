// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package kindfilelogging spins up the same pulumi environment as the awskubernetes package but
// includes a logger container as well
package kindfilelogging

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/helmagent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"

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

// kindDefaultHelmValues are KinD-specific Helm overrides for kindfilelogging clusters.
const kindDefaultHelmValues = `
datadog:
  kubelet:
    tlsVerify: false
  envDict:
    DD_CONTAINER_EXCLUDE: "kube_namespace:^exclude-namespace$"
agents:
  useHostNetwork: true
`

// Provisioner creates a new provisioner.
// Agent installation is performed via Helm after Pulumi provisions the KinD
// cluster and FakeIntake (PostProvision), rather than inside Pulumi itself.
func Provisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	params := newProvisionerParams()
	_ = optional.ApplyOptions(params, opts)

	agentOpts := params.agentOptions
	usePostProvision := agentOpts != nil

	pulumiProv := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to issues that are hard to debug.
		params := newProvisionerParams()
		_ = optional.ApplyOptions(params, opts)
		if usePostProvision {
			params.agentOptions = nil
		}
		return KindRunFunc(ctx, env, params)
	}, params.extraConfigParams)

	if !usePostProvision {
		return pulumiProv
	}

	clusterNameTag, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.StackNameSuffix, "")
	if err != nil {
		clusterNameTag = ""
	}
	postProvisionOpts := []kubernetesagentparams.Option{
		kubernetesagentparams.WithHelmValues(kindDefaultHelmValues),
	}
	if clusterNameTag != "" {
		postProvisionOpts = append(postProvisionOpts, kubernetesagentparams.WithTags([]string{"stackid:" + clusterNameTag}))
	}
	postProvisionOpts = append(postProvisionOpts, agentOpts...)

	return provisioners.WithPostProvision(pulumiProv, func(t *testing.T, env *environments.Kubernetes) {
		helmagent.Install(installers.FromT(t), env, runner.CloudAWS, postProvisionOpts...)
	})
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

	installEcrCredsHelperCmd, err := docker.InstallECRCredentialsHelper(awsEnv.Namer, host)
	if err != nil {
		return fmt.Errorf("docker.InstallECRCredentialsHelper: %w", err)
	}

	kindCluster, err := kubeComp.NewKindCluster(&awsEnv, host, params.name, awsEnv.KubernetesVersion(), utils.PulumiDependsOn(installEcrCredsHelperCmd))
	if err != nil {
		return fmt.Errorf("kubeComp.NewKindCluster: %w", err)
	}
	err = kindCluster.Export(ctx, &env.KubernetesCluster.ClusterOutput)
	if err != nil {
		return fmt.Errorf("kindCluster.Export: %w", err)
	}

	// kubeProvider is no longer needed since agent install moved to PostProvision.
	// Keep the provider creation for any future Pulumi workload resources that need it.
	_ = kindCluster.KubeConfig // reference keeps the cluster in the dependency graph

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

	// Agent installation is handled by PostProvision via helmagent.Install.
	env.Agent = nil

	return nil
}
