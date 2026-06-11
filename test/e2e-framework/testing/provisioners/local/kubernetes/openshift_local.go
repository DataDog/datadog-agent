// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package localkubernetes

import (
	agentComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/local"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	openShiftLocalProvisionerBaseID = "local-openshift-"
	defaultOpenShiftClusterName     = "openshift"
)

// OpenShiftLocalProvisioner creates an OpenShift (CRC) local provisioner.
func OpenShiftLocalProvisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	params := newProvisionerParams()
	_ = optional.ApplyOptions(params, opts)
	if params.name == defaultVMName {
		params.name = defaultOpenShiftClusterName
	}

	return provisioners.NewTypedPulumiProvisioner(openShiftLocalProvisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		runParams := newProvisionerParams()
		_ = optional.ApplyOptions(runParams, opts)
		if runParams.name == defaultVMName {
			runParams.name = defaultOpenShiftClusterName
		}

		return openShiftLocalRunFunc(ctx, env, runParams)
	}, params.extraConfigParams)
}

func openShiftLocalRunFunc(ctx *pulumi.Context, env *environments.Kubernetes, params *ProvisionerParams) error {
	localEnv, err := local.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	cluster, err := kubeComp.NewLocalOpenShiftCluster(&localEnv, params.name, localEnv.OpenShiftPullSecretPath())
	if err != nil {
		return err
	}
	if err := cluster.Export(ctx, &env.KubernetesCluster.ClusterOutput); err != nil {
		return err
	}

	kubeProvider, err := kubernetes.NewProvider(ctx, localEnv.CommonNamer().ResourceName("openshift-k8s-provider"), &kubernetes.ProviderArgs{
		EnableServerSideApply: pulumi.Bool(true),
		Kubeconfig:            cluster.KubeConfig,
	})
	if err != nil {
		return err
	}

	var fakeIntake *fakeintakeComp.Fakeintake
	if params.fakeintakeOptions != nil {
		fakeIntake, err = fakeintakeComp.NewLocalDockerFakeintake(&localEnv, "fakeintake")
		if err != nil {
			return err
		}
		if err := fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput); err != nil {
			return err
		}
	} else {
		env.FakeIntake = nil
	}

	if params.agentOptions != nil {
		for _, hook := range params.preAgentHooks {
			if err := hook(&localEnv, kubeProvider); err != nil {
				return err
			}
		}

		params.agentOptions = append(
			[]kubernetesagentparams.Option{
				func(p *kubernetesagentparams.Params) error {
					p.HelmValues = append(p.HelmValues, agentComp.BuildOpenShiftHelmValues().ToYAMLPulumiAssetOutput())
					return nil
				},
				kubernetesagentparams.WithClusterName(cluster.ClusterName),
				kubernetesagentparams.WithNamespace("datadog"),
				kubernetesagentparams.WithTimeout(900),
			},
			params.agentOptions...,
		)
		if fakeIntake != nil {
			params.agentOptions = append([]kubernetesagentparams.Option{kubernetesagentparams.WithFakeintake(fakeIntake)}, params.agentOptions...)
		}

		agent, err := helm.NewKubernetesAgent(&localEnv, params.name, kubeProvider, params.agentOptions...)
		if err != nil {
			return err
		}
		if err := agent.Export(ctx, &env.Agent.KubernetesAgentOutput); err != nil {
			return err
		}

		dependsOnDDAgent := pulumi.DependsOn([]pulumi.Resource{agent})
		for _, appFunc := range params.depWorkloadAppFuncs {
			if _, err := appFunc(&localEnv, kubeProvider, dependsOnDDAgent); err != nil {
				return err
			}
		}
	} else {
		env.Agent = nil
	}

	for _, appFunc := range params.workloadAppFuncs {
		if _, err := appFunc(&localEnv, kubeProvider); err != nil {
			return err
		}
	}

	return nil
}
