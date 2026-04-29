// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ssigradualrollout: This test suite is to E2E test the gradual rollout feature in SSI.
// It will test that the gradual rollout feature is enabled by default.
package ssigradualrollout

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"testing"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	compkube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	ssi "github.com/DataDog/datadog-agent/test/new-e2e/tests/ssi"
)

//go:embed testdata/base.yaml
var baseHelmValues string

//go:embed testdata/default_opt_in.yaml
var defaultOptInHelmValues string

// ssiGradualRolloutSuite runs all gradual rollout test scenarios on a single cluster,
// calling UpdateEnv at the start of each test to switch Helm values and workloads.
type ssiGradualRolloutSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestSSIGradualRolloutSuite is the single entry point. One cluster is provisioned with only
// the mock registry deployed initially, then UpdateEnv is called at the start of each test.
func TestSSIGradualRolloutSuite(t *testing.T) {
	e2e.Run(t, &ssiGradualRolloutSuite{}, e2e.WithProvisioner(ssi.Provisioner(ssi.ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(baseHelmValues),
		},
		WorkloadAppFunc: func(e config.Env, kubeProvider *kubernetes.Provider) (*compkube.Workload, error) {
			return nil, deployMockRegistry(e, kubeProvider)
		},
	})))
}

// defaultSSILanguages mirrors supportedLanguages in
// pkg/clusteragent/admission/mutate/autoinstrumentation/language_versions.go. With no
// ddTraceVersions in the target config, the cluster-agent injects all of these at their
// latest major versions, and gradual rollout should resolve a digest for each.
var defaultSSILanguages = []string{"java", "js", "python", "dotnet", "ruby", "php"}

// TestDefaultOptIn verifies that when gradual rollout is enabled (default) and mutable
// major-version tags are configured (the SSI default), the cluster-agent resolves every
// default-language lib init container to a digest-based reference.
func (v *ssiGradualRolloutSuite) TestDefaultOptIn() {
	const (
		scenarioNamespace = "gradual-rollout-default"
		appName           = "gradual-rollout-app"
	)

	// Force cluster-agent restart when the CA rotates (its cert pool is cached at
	// startup). Only matters in E2E_DEV_MODE: sync.Once regenerates certs per
	// binary invocation while the cluster persists. No-op in CI (fresh cluster).
	caCertPEM, _, _, err := getCerts()
	require.NoError(v.T(), err)
	certHash := fmt.Sprintf("%x", sha256.Sum256(caCertPEM))

	v.UpdateEnv(ssi.Provisioner(ssi.ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(baseHelmValues),
			kubernetesagentparams.WithHelmValues(defaultOptInHelmValues),
			kubernetesagentparams.WithHelmValues(fmt.Sprintf(
				"clusterAgent:\n  podAnnotations:\n    checksum/mock-registry-ca: %q\n",
				certHash[:16],
			)),
		},
		WorkloadAppFunc: func(e config.Env, kubeProvider *kubernetes.Provider) (*compkube.Workload, error) {
			return nil, deployMockRegistry(e, kubeProvider)
		},
		AgentDependentWorkloadAppFunc: func(e config.Env, kubeProvider *kubernetes.Provider, dependsOnAgent pulumi.ResourceOption) (*compkube.Workload, error) {
			return nil, deployTestWorkload(e, kubeProvider, scenarioNamespace, appName, dependsOnAgent)
		},
	}))

	k8s := v.Env().KubernetesCluster.Client()
	pod := findMutatedPod(v.T(), k8s, scenarioNamespace, appName, "python")
	for _, lang := range defaultSSILanguages {
		requireDigestBasedLibImage(v.T(), pod, lang)
	}
}
