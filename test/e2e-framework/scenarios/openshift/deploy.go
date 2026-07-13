// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package openshift contains shared deployment logic for OpenShift scenarios.
package openshift

import (
	kubernetesProvider "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	agentComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/cpustress"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/dogstatsd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/mutatedbyadmissioncontroller"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/prometheus"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redis"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/tracegen"
	dogstatsdstandalone "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dogstatsd-standalone"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/vpa"
)

// openShiftPrivilegedPSSLabels sets a namespace to the privileged Pod Security Standard,
// which OpenShift requires for the dogstatsd workloads (they run without an explicit
// restricted-compliant securityContext).
var openShiftPrivilegedPSSLabels = pulumi.StringMap{
	"pod-security.kubernetes.io/enforce": pulumi.String("privileged"),
	"pod-security.kubernetes.io/warn":    pulumi.String("privileged"),
	"pod-security.kubernetes.io/audit":   pulumi.String("privileged"),
}

// DeployComponents deploys the OpenShift agent and test workloads onto an existing Kubernetes provider.
// fakeIntake may be nil. agentOptions is the full set of agent options to use; pass nil to skip
// agent deployment entirely (e.g. when WithoutAgent() was called). Use agentOptions to pass
// scenario-specific extras such as kubernetesagentparams.WithDualShipping.
func DeployComponents(
	ctx *pulumi.Context,
	env config.Env,
	kubeProvider *kubernetesProvider.Provider,
	cluster *kubeComp.Cluster,
	fakeIntake *fakeintakeComp.Fakeintake,
	agentOptions []kubernetesagentparams.Option,
) error {
	var dependsOnDDAgent pulumi.ResourceOption

	if agentOptions != nil {
		k8sAgentOptions := []kubernetesagentparams.Option{
			func(p *kubernetesagentparams.Params) error {
				p.HelmValues = append(p.HelmValues, agentComp.BuildOpenShiftHelmValues().ToYAMLPulumiAssetOutput())
				return nil
			},
			kubernetesagentparams.WithClusterName(cluster.ClusterName),
			kubernetesagentparams.WithNamespace("datadog"),
			kubernetesagentparams.WithTimeout(900),
		}
		if fakeIntake != nil {
			k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithFakeintake(fakeIntake))
		}
		k8sAgentOptions = append(k8sAgentOptions, agentOptions...)

		k8sAgent, err := helm.NewKubernetesAgent(env, env.CommonNamer().ResourceName("datadog-agent"), kubeProvider, k8sAgentOptions...)
		if err != nil {
			return err
		}
		if err := k8sAgent.Export(ctx, nil); err != nil {
			return err
		}
		dependsOnDDAgent = utils.PulumiDependsOn(k8sAgent)
	}

	if env.TestingWorkloadDeploy() {
		vpaCrd, err := vpa.DeployCRD(env, kubeProvider)
		if err != nil {
			return err
		}
		dependsOnVPA := utils.PulumiDependsOn(vpaCrd)

		if _, err := redis.K8sAppDefinition(env, kubeProvider, "workload-redis", true, dependsOnDDAgent, dependsOnVPA); err != nil {
			return err
		}
		if _, err := prometheus.K8sAppDefinition(env, kubeProvider, "workload-prometheus"); err != nil {
			return err
		}
		if _, err := cpustress.K8sAppDefinition(env, kubeProvider, "workload-cpustress"); err != nil {
			return err
		}
		if _, err := tracegen.K8sAppDefinition(env, kubeProvider, "workload-tracegen"); err != nil {
			return err
		}
		if _, err := nginx.K8sAppDefinition(env, kubeProvider, "workload-nginx", 8080, "", true, dependsOnDDAgent, dependsOnVPA); err != nil {
			return err
		}
		if _, err := mutatedbyadmissioncontroller.K8sAppDefinition(env, kubeProvider, "workload-mutated", "workload-mutated-lib-injection", dependsOnDDAgent); err != nil {
			return err
		}

		if env.DogstatsdDeploy() {
			// Standalone dogstatsd
			if _, err := dogstatsdstandalone.K8sAppDefinition(env, kubeProvider, "dogstatsd-standalone", "/run/crio/crio.sock", fakeIntake, false, ""); err != nil {
				return err
			}

			// Dogstatsd clients that report to the standalone dogstatsd deployment
			if _, err := dogstatsd.K8sAppDefinitionWithOptions(env, kubeProvider, "workload-dogstatsd-standalone", dogstatsdstandalone.HostPort, "/run/datadog/dsd.socket", []dogstatsd.K8sAppOption{dogstatsd.WithNamespaceLabels(openShiftPrivilegedPSSLabels)}, dependsOnDDAgent /* for admission */); err != nil {
				return err
			}

			// Dogstatsd clients that report to the Agent
			if _, err := dogstatsd.K8sAppDefinitionWithOptions(env, kubeProvider, "workload-dogstatsd", 8125, "/var/run/datadog/dsd.socket", []dogstatsd.K8sAppOption{dogstatsd.WithNamespaceLabels(openShiftPrivilegedPSSLabels)}, dependsOnDDAgent /* for admission */); err != nil {
				return err
			}
		}
	}

	return nil
}
