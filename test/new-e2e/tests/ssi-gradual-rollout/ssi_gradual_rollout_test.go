// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ssigradualrollout: This test suite is to E2E test the gradual rollout feature in SSI.
// It will test the following scenarios:
// - The gradual rollout feature is enabled by default.
// - The gradual rollout feature is disabled by setting the `DD_ADMISSION_CONTROLLER_AUTO_INSTRUMENTATION_GRADUAL_ROLLOUT_ENABLED` environment variable to `false`.
// - The gradual rollout feature is disabled by setting the `DD_ADMISSION_CONTROLLER_AUTO_INSTRUMENTATION_CONTAINER_REGISTRY` environment variable to a custom registry.
// - The gradual rollout feature is disabled by setting the version tag for a library to a full canonical tag (e.g. "1.2.3") instead of a mutable tag (e.g. "v1" or "latest").

package ssigradualrollout

import (
	_ "embed"
	"testing"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/singlestep"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	compkube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	ssi "github.com/DataDog/datadog-agent/test/new-e2e/tests/ssi"
)

//go:embed testdata/default_opt_in.yaml
var baseHelmValues string

//go:embed testdata/default_opt_in.yaml
var defaultOptInHelmValues string

//go:embed testdata/explicit_opt_out.yaml
var explicitOptOutHelmValues string

//go:embed testdata/canonical_tag_opt_out.yaml
var canonicalTagOptOutHelmValues string

//go:embed testdata/custom_registry_opt_out.yaml
var customRegistryOptOutHelmValues string

// ssiGradualRolloutSuite runs all gradual rollout test scenarios on a single cluster,
// calling UpdateEnv at the start of each test to switch Helm values and workloads.
type ssiGradualRolloutSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestSSIGradualRolloutSuite is the single entry point. One cluster is provisioned with only
// the mock registry deployed initially, then UpdateEnv is called at the start of each test.
func TestSSIGradualRolloutSuite(t *testing.T) {
	e2e.Run(t, &ssiGradualRolloutSuite{}, e2e.WithProvisioner(ssi.Provisioner(ssi.ProvisionerOptions{
		WorkloadAppFunc: func(e config.Env, kubeProvider *kubernetes.Provider) (*compkube.Workload, error) {
			return nil, deployMockRegistry(e, kubeProvider)
		},
	})))
}

// TestDefaultOptIn verifies that when gradual rollout is enabled (default) and a mutable tag
// (v4) is configured, the cluster-agent resolves the image to a digest-based reference (@sha256:...).
func (v *ssiGradualRolloutSuite) TestDefaultOptIn() {
	const (
		scenarioNamespace = "gradual-rollout-default"
		appName           = "gradual-rollout-python-app"
	)

	v.UpdateEnv(ssi.Provisioner(ssi.ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(baseHelmValues),
			kubernetesagentparams.WithHelmValues(defaultOptInHelmValues),
		},
		WorkloadAppFunc: func(e config.Env, kubeProvider *kubernetes.Provider) (*compkube.Workload, error) {
			return nil, deployMockRegistry(e, kubeProvider)
		},
		AgentDependentWorkloadAppFunc: func(e config.Env, kubeProvider *kubernetes.Provider, dependsOnAgent pulumi.ResourceOption) (*compkube.Workload, error) {
			return singlestep.Scenario(e, kubeProvider, "gradual-rollout-default", []singlestep.Namespace{
				{
					Name: scenarioNamespace,
					Apps: []singlestep.App{
						{
							Name:    appName,
							Image:   "gcr.io/datadoghq/injector-dev/python",
							Version: "d425e7df",
							Port:    8080,
						},
					},
				},
			}, dependsOnAgent)
		},
	}))

	k8s := v.Env().KubernetesCluster.Client()
	pod := findMutatedPod(v.T(), k8s, scenarioNamespace, appName, "python")
	RequireDigestBasedLibImage(v.T(), pod, "python")
}
