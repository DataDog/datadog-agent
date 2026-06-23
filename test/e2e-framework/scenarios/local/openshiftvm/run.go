// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package openshiftvm

import (
	kubernetesProvider "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/local"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/openshift"
)

func Run(ctx *pulumi.Context) error {
	localEnv, err := local.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	cluster, err := kubernetes.NewLocalOpenShiftCluster(&localEnv, "openshift", localEnv.OpenShiftPullSecretPath(), localEnv.OpenShiftCPUs(), localEnv.OpenShiftMemory(), localEnv.OpenShiftDisk())
	if err != nil {
		return err
	}
	if err := cluster.Export(ctx, nil); err != nil {
		return err
	}

	if localEnv.InitOnly() {
		return nil
	}

	kubeProvider, err := kubernetesProvider.NewProvider(ctx, localEnv.CommonNamer().ResourceName("openshift-k8s-provider"), &kubernetesProvider.ProviderArgs{
		Kubeconfig:            cluster.KubeConfig,
		EnableServerSideApply: pulumi.BoolPtr(true),
		DeleteUnreachable:     pulumi.BoolPtr(true),
	})
	if err != nil {
		return err
	}

	var fakeIntake *fakeintakeComp.Fakeintake
	if localEnv.AgentUseFakeintake() {
		fakeIntake, err = fakeintakeComp.NewLocalDockerFakeintake(&localEnv, "fakeintake")
		if err != nil {
			return err
		}
		if err := fakeIntake.Export(ctx, nil); err != nil {
			return err
		}
	}

	var extraAgentOptions []kubernetesagentparams.Option
	if localEnv.AgentUseDualShipping() {
		extraAgentOptions = append(extraAgentOptions, kubernetesagentparams.WithDualShipping())
	}

	return openshift.DeployComponents(ctx, &localEnv, kubeProvider, cluster, fakeIntake, extraAgentOptions...)
}
