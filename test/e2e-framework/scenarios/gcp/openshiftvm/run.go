// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package openshiftvm

import (
	kubernetesNewProvider "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	resGcp "github.com/DataDog/datadog-agent/test/e2e-framework/resources/gcp"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/compute"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/openshift"
)

func Run(ctx *pulumi.Context) error {
	gcpEnv, err := resGcp.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	osDesc := os.DescriptorFromString("redhat:9", os.RedHat9)
	vm, err := compute.NewVM(gcpEnv, "openshift",
		compute.WithOS(osDesc),
		compute.WithInstancetype("n2-standard-8"),
		compute.WithNestedVirt(true),
	)
	if err != nil {
		return err
	}
	if err := vm.Export(ctx, nil); err != nil {
		return err
	}

	openshiftCluster, err := kubernetes.NewOpenShiftCluster(&gcpEnv, vm, "openshift", gcpEnv.OpenShiftPullSecretPath(), gcpEnv.OpenShiftCPUs(), gcpEnv.OpenShiftMemory(), gcpEnv.OpenShiftDisk())
	if err != nil {
		return err
	}
	if err := openshiftCluster.Export(ctx, nil); err != nil {
		return err
	}

	if gcpEnv.InitOnly() {
		return nil
	}

	kubeProvider, err := kubernetesNewProvider.NewProvider(ctx, gcpEnv.Namer.ResourceName("openshift-k8s-provider"), &kubernetesNewProvider.ProviderArgs{
		Kubeconfig:            openshiftCluster.KubeConfig,
		EnableServerSideApply: pulumi.BoolPtr(true),
		DeleteUnreachable:     pulumi.BoolPtr(true),
	})
	if err != nil {
		return err
	}

	var fakeIntake *fakeintakeComp.Fakeintake
	if gcpEnv.AgentUseFakeintake() {
		fakeIntakeOptions := []fakeintake.Option{
			fakeintake.WithMemory(2048),
		}
		if gcpEnv.InfraShouldDeployFakeintakeWithLB() {
			fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithLoadBalancer())
		}
		if gcpEnv.AgentUseDualShipping() {
			fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithoutDDDevForwarding())
		}
		if retentionPeriod := gcpEnv.AgentFakeintakeRetentionPeriod(); retentionPeriod != "" {
			fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithRetentionPeriod(retentionPeriod))
		}
		if fakeIntake, err = fakeintake.NewVMInstance(gcpEnv, fakeIntakeOptions...); err != nil {
			return err
		}
		if err := fakeIntake.Export(gcpEnv.Ctx(), nil); err != nil {
			return err
		}
	}

	var extraAgentOptions []kubernetesagentparams.Option
	if gcpEnv.AgentUseDualShipping() {
		extraAgentOptions = append(extraAgentOptions, kubernetesagentparams.WithDualShipping())
	}

	return openshift.DeployComponents(ctx, &gcpEnv, kubeProvider, openshiftCluster, fakeIntake, extraAgentOptions...)
}
