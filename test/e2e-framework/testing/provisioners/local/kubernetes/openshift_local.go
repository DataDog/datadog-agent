// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package localkubernetes

import (
	"testing"

	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/local"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/helmagent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	openShiftLocalProvisionerBaseID = "local-openshift-"
	defaultOpenShiftClusterName     = "openshift"
)

// OpenShiftLocalProvisioner creates an OpenShift (CRC) local provisioner.
//
// Agent installation is performed via Helm after Pulumi provisions the
// OpenShift cluster and FakeIntake (PostProvision). OpenShift-specific Helm
// values are prepended automatically. The preAgentHooks still run inside
// Pulumi since they use the Pulumi k8s provider.
func OpenShiftLocalProvisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	params := newProvisionerParams()
	_ = optional.ApplyOptions(params, opts)
	if params.name == defaultVMName {
		params.name = defaultOpenShiftClusterName
	}

	agentOpts := params.agentOptions
	usePostProvision := agentOpts != nil

	pulumiProv := provisioners.NewTypedPulumiProvisioner(openShiftLocalProvisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		runParams := newProvisionerParams()
		_ = optional.ApplyOptions(runParams, opts)
		if runParams.name == defaultVMName {
			runParams.name = defaultOpenShiftClusterName
		}
		if usePostProvision {
			runParams.agentOptions = nil
		}
		return openShiftLocalRunFunc(ctx, env, runParams)
	}, params.extraConfigParams)

	if !usePostProvision {
		return pulumiProv
	}

	postProvisionOpts := append(
		[]kubernetesagentparams.Option{kubernetesagentparams.WithHelmValues(helmagent.OpenShiftHelmValues)},
		agentOpts...,
	)

	return provisioners.WithPostProvision(pulumiProv, func(t *testing.T, env *environments.Kubernetes) {
		helmagent.Install(installers.FromT(t), env, runner.CloudGCP, postProvisionOpts...)
	})
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
	_ = fakeIntake // used by Pulumi path; installer reads env.FakeIntake directly

	// Run preAgentHooks (SCC setup, RBAC) via the Pulumi k8s provider before PostProvision.
	if params.agentOptions != nil || len(params.preAgentHooks) > 0 {
		for _, hook := range params.preAgentHooks {
			if err := hook(&localEnv, kubeProvider); err != nil {
				return err
			}
		}
	}

	// Agent installation is handled by PostProvision via helmagent.Install.
	env.Agent = nil

	for _, appFunc := range params.workloadAppFuncs {
		if _, err := appFunc(&localEnv, kubeProvider); err != nil {
			return err
		}
	}

	return nil
}
