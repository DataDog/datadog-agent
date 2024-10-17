// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package awskubernetes contains the provisioner for the Kubernetes based environments
package awskubernetes

import (
	"context"
	"fmt"

	"github.com/DataDog/test-infra-definitions/common/utils"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent/helm"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/cpustress"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/dogstatsd"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/mutatedbyadmissioncontroller"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/nginx"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/prometheus"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/redis"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/tracegen"
	dogstatsdstandalone "github.com/DataDog/test-infra-definitions/components/datadog/dogstatsd-standalone"
	fakeintakeComp "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
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

func kindDiagnoseFunc(ctx context.Context, stackName string) (string, error) {
	dumpResult, err := dumpKindClusterState(ctx, stackName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Dumping Kind cluster state:\n%s", dumpResult), nil
}

// KindProvisioner creates a new provisioner
func KindProvisioner(opts ...ProvisionerOption) e2e.TypedProvisioner[environments.Kubernetes] {
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

	provisioner.SetDiagnoseFunc(kindDiagnoseFunc)

	return provisioner
}

// KindRunFunc is the Pulumi run function that runs the provisioner
func KindRunFunc(ctx *pulumi.Context, env *environments.Kubernetes, params *ProvisionerParams) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	host, err := ec2.NewVM(awsEnv, params.name, params.vmOptions...)
	if err != nil {
		return err
	}

	installEcrCredsHelperCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, host)
	if err != nil {
		return err
	}

	kindCluster, err := kubeComp.NewKindCluster(&awsEnv, host, awsEnv.CommonNamer().ResourceName("kind"), params.name, awsEnv.KubernetesVersion(), utils.PulumiDependsOn(installEcrCredsHelperCmd))
	if err != nil {
		return err
	}

	err = kindCluster.Export(ctx, &env.KubernetesCluster.ClusterOutput)
	if err != nil {
		return err
	}

	kubeProvider, err := kubernetes.NewProvider(ctx, awsEnv.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
		EnableServerSideApply: pulumi.Bool(true),
		Kubeconfig:            kindCluster.KubeConfig,
	})
	if err != nil {
		return err
	}

	var fakeIntake *fakeintakeComp.Fakeintake
	if params.fakeintakeOptions != nil {
		fakeintakeOpts := []fakeintake.Option{fakeintake.WithLoadBalancer()}
		params.fakeintakeOptions = append(fakeintakeOpts, params.fakeintakeOptions...)
		fakeIntake, err = fakeintake.NewECSFargateInstance(awsEnv, params.name, params.fakeintakeOptions...)
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

	var dependsOnCrd []pulumi.Resource
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
		agent, err := helm.NewKubernetesAgent(&awsEnv, kindClusterName, kubeProvider, params.agentOptions...)
		if err != nil {
			return err
		}
		err = agent.Export(ctx, &env.Agent.KubernetesAgentOutput)
		if err != nil {
			return err
		}
		dependsOnCrd = append(dependsOnCrd, agent)
	} else {
		env.Agent = nil
	}

	if params.deployDogstatsd {
		if _, err := dogstatsdstandalone.K8sAppDefinition(&awsEnv, kubeProvider, "dogstatsd-standalone", fakeIntake, false, ctx.Stack()); err != nil {
			return err
		}
	}

	// Deploy testing workload
	if params.deployTestWorkload {
		// dogstatsd clients that report to the Agent
		if _, err := dogstatsd.K8sAppDefinition(&awsEnv, kubeProvider, "workload-dogstatsd", 8125, "/var/run/datadog/dsd.socket"); err != nil {
			return err
		}

		// dogstatsd clients that report to the dogstatsd standalone deployment
		if _, err := dogstatsd.K8sAppDefinition(&awsEnv, kubeProvider, "workload-dogstatsd-standalone", dogstatsdstandalone.HostPort, dogstatsdstandalone.Socket); err != nil {
			return err
		}

		if _, err := tracegen.K8sAppDefinition(&awsEnv, kubeProvider, "workload-tracegen"); err != nil {
			return err
		}

		if _, err := prometheus.K8sAppDefinition(&awsEnv, kubeProvider, "workload-prometheus"); err != nil {
			return err
		}

		if _, err := mutatedbyadmissioncontroller.K8sAppDefinition(&awsEnv, kubeProvider, "workload-mutated", "workload-mutated-lib-injection"); err != nil {
			return err
		}

		// These workloads can be deployed only if the agent is installed, they rely on CRDs installed by Agent helm chart
		if params.agentOptions != nil {
			if _, err := nginx.K8sAppDefinition(&awsEnv, kubeProvider, "workload-nginx", "", true, utils.PulumiDependsOn(dependsOnCrd...)); err != nil {
				return err
			}

			if _, err := redis.K8sAppDefinition(&awsEnv, kubeProvider, "workload-redis", true, utils.PulumiDependsOn(dependsOnCrd...)); err != nil {
				return err
			}

			if _, err := cpustress.K8sAppDefinition(&awsEnv, kubeProvider, "workload-cpustress", utils.PulumiDependsOn(dependsOnCrd...)); err != nil {
				return err
			}
		}
	}
	for _, appFunc := range params.workloadAppFuncs {
		_, err := appFunc(&awsEnv, kubeProvider)
		if err != nil {
			return err
		}
	}

	return nil
}
