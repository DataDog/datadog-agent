// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ssigradualrollout provides end-to-end tests for the SSI gradual rollout image
// resolver. It deploys an in-cluster mock container registry and asserts that the
// cluster-agent's admission webhook injects digest-based lib init containers for every
// default-language target when gradual rollout is enabled (the default).
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

type ssiGradualRolloutSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

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

// Mirrors supportedLanguages in
// pkg/clusteragent/admission/mutate/autoinstrumentation/language_versions.go. With no
// ddTraceVersions in the target config, the cluster-agent injects all of these.
var defaultSSILanguages = []string{"java", "js", "python", "dotnet", "ruby", "php"}

func (v *ssiGradualRolloutSuite) TestDefaultOptIn() {
	const (
		scenarioNamespace = "gradual-rollout-default"
		appName           = "gradual-rollout-app"
	)

	// Force a cluster-agent restart when the CA rotates: its cert pool is snapshotted
	// at startup, and in E2E_DEV_MODE the cluster persists across binary invocations
	// while sync.Once regenerates certs each run. No-op in CI.
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
