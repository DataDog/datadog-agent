// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package localkubernetes contains the provisioner for the local Kubernetes based environments
package localkubernetes

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"

	"github.com/DataDog/test-infra-definitions/common/config"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	fakeintakeComp "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
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
	// and it's easy to forget about it, leading to hard to debug issues.
	params := newProvisionerParams()
	_ = optional.ApplyOptions(params, opts)

	provisioner := e2e.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := newProvisionerParams()
		_ = optional.ApplyOptions(params, opts)

		return KindRunFunc(ctx, env, params)
	}, params.extraConfigParams)

	return provisioner
}

// KindRunFunc is the Pulumi run function that runs the provisioner
func KindRunFunc(ctx *pulumi.Context, env *environments.Kubernetes, params *ProvisionerParams) error {

	// Fake Intake is not supported when running a local kind cluster

	localEnv, err := config.NewCommonEnvironment(ctx, nil)
	if err != nil {
		return err
	}
	kindCluster, err := kubeComp.NewLocalKindCluster(localEnv, localEnv.CommonNamer.ResourceName("kind"), params.name, localEnv.KubernetesVersion())
	if err != nil {
		return err
	}

	err = kindCluster.Export(ctx, &env.KubernetesCluster.ClusterOutput)
	if err != nil {
		return err
	}

	kubeProvider, err := kubernetes.NewProvider(ctx, localEnv.CommonNamer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
		EnableServerSideApply: pulumi.Bool(true),
		Kubeconfig:            kindCluster.KubeConfig,
	})
	if err != nil {
		return err
	}

	if params.fakeintakeOptions != nil {
		fakeintakeOpts := []fakeintake.Option{fakeintake.WithLoadBalancer()}
		params.fakeintakeOptions = append(fakeintakeOpts, params.fakeintakeOptions...)
		fakeIntake, err := fakeintakeComp.NewLocalKubernetesFakeintake(localEnv, "fakeintake", kubeProvider)
		if err != nil {
			return err
		}
		err = fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)
		if err != nil {
			return err
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
agents:
  useHostNetwork: true
`, kindClusterName)

		newOpts := []kubernetesagentparams.Option{kubernetesagentparams.WithHelmValues(helmValues)}
		params.agentOptions = append(newOpts, params.agentOptions...)
		agent, err := agent.NewKubernetesAgent(localEnv, kindClusterName, kubeProvider, params.agentOptions...)
		if err != nil {
			return err
		}
		err = agent.Export(ctx, &env.Agent.KubernetesAgentOutput)
		if err != nil {
			return err
		}
	} else {
		env.Agent = nil
	}

	return nil
}

// func NewLocalKindCluster(ctx *pulumi.Context, env *environments.Kubernetes, resourceName string) error {
// 	return components.NewComponent(env, resourceName, func(clusterComp *Cluster) error {
// 		opts = utils.MergeOptions[pulumi.ResourceOption](opts, pulumi.Parent(clusterComp))

// 		runner := vm.OS.Runner()
// 		commonEnvironment := env
// 		packageManager := vm.OS.PackageManager()
// 		curlCommand, err := packageManager.Ensure("curl", nil, opts...)
// 		if err != nil {
// 			return err
// 		}

// 		_, dockerInstallCmd, err := docker.NewManager(env, vm, true, opts...)
// 		if err != nil {
// 			return err
// 		}
// 		opts = utils.MergeOptions(opts, utils.PulumiDependsOn(dockerInstallCmd, curlCommand))

// 		kindVersionConfig, err := getKindVersionConfig(kubeVersion)
// 		if err != nil {
// 			return err
// 		}

// 		kindArch := vm.OS.Descriptor().Architecture
// 		if kindArch == os.AMD64Arch {
// 			kindArch = "amd64"
// 		}
// 		kindInstall, err := runner.Command(
// 			commonEnvironment.CommonNamer.ResourceName("kind-install"),
// 			&command.Args{
// 				Create: pulumi.Sprintf(`curl -Lo ./kind "https://kind.sigs.k8s.io/dl/%s/kind-linux-%s" && sudo install kind /usr/local/bin/kind`, kindVersionConfig.kindVersion, kindArch),
// 			},
// 			opts...,
// 		)
// 		if err != nil {
// 			return err
// 		}

// 		clusterConfigFilePath := fmt.Sprintf("/tmp/kind-cluster-%s.yaml", kindClusterName)
// 		clusterConfig, err := vm.OS.FileManager().CopyInlineFile(
// 			pulumi.String(kindClusterConfig),
// 			clusterConfigFilePath, false, opts...)
// 		if err != nil {
// 			return err
// 		}

// 		nodeImage := fmt.Sprintf("%s:%s", kindNodeImageRegistry, kindVersionConfig.nodeImageVersion)
// 		createCluster, err := runner.Command(
// 			commonEnvironment.CommonNamer.ResourceName("kind-create-cluster", resourceName),
// 			&command.Args{
// 				Create:   pulumi.Sprintf("kind create cluster --name %s --config %s --image %s --wait %s", kindClusterName, clusterConfigFilePath, nodeImage, kindReadinessWait),
// 				Delete:   pulumi.Sprintf("kind delete cluster --name %s", kindClusterName),
// 				Triggers: pulumi.Array{pulumi.String(kindClusterConfig)},
// 			},
// 			utils.MergeOptions(opts, utils.PulumiDependsOn(clusterConfig, kindInstall), pulumi.DeleteBeforeReplace(true))...,
// 		)
// 		if err != nil {
// 			return err
// 		}

// 		kubeConfigCmd, err := runner.Command(
// 			commonEnvironment.CommonNamer.ResourceName("kind-kubeconfig", resourceName),
// 			&command.Args{
// 				Create: pulumi.Sprintf("kind get kubeconfig --name %s", kindClusterName),
// 			},
// 			utils.MergeOptions(opts, utils.PulumiDependsOn(createCluster))...,
// 		)
// 		if err != nil {
// 			return err
// 		}

// 		// Patch Kubeconfig based on private IP output
// 		// Also add skip tls
// 		clusterComp.KubeConfig = pulumi.All(kubeConfigCmd.Stdout, vm.Address).ApplyT(func(args []interface{}) string {
// 			allowInsecure := regexp.MustCompile("certificate-authority-data:.+").ReplaceAllString(args[0].(string), "insecure-skip-tls-verify: true")
// 			return strings.ReplaceAll(allowInsecure, "0.0.0.0", args[1].(string))
// 		}).(pulumi.StringOutput)
// 		clusterComp.ClusterName = pulumi.String(kindClusterName).ToStringOutput()

// 		return nil
// 	}, opts...)
// }
