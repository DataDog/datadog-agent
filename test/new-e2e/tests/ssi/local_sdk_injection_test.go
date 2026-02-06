// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package ssi

import (
	"os"
	"testing"
	"time"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ssi/testutils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/singlestep"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	compkube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

type localSDKInjectionSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestLocalSDKInjectionSuite(t *testing.T) {
	helmValues, err := os.ReadFile("testdata/local_sdk_injection.yaml")
	require.NoError(t, err, "Could not open helm values file for test")
	e2e.Run(t, &localSDKInjectionSuite{}, e2e.WithProvisioner(Provisioner(ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(string(helmValues)),
		},
		AgentDependentWorkloadAppFunc: func(e config.Env, kubeProvider *kubernetes.Provider, dependsOnAgent pulumi.ResourceOption) (*compkube.Workload, error) {
			return singlestep.Scenario(e, kubeProvider, "local-sdk-injection", []singlestep.Namespace{
				{
					Name: "application",
					Apps: []singlestep.App{
						{
							Name:    DefaultAppName,
							Image:   "gcr.io/datadoghq/injector-dev/python",
							Version: "d425e7df",
							Port:    8080,
							PodLabels: map[string]string{
								"admission.datadoghq.com/enabled": "true",
								"tags.datadoghq.com/service":      DefaultAppName,
							},
							PodAnnotations: map[string]string{
								"admission.datadoghq.com/python-lib.version": "v3.18.1",
							},
						},
						{
							Name:    "expect-no-injection",
							Image:   "gcr.io/datadoghq/injector-dev/python",
							Version: "d425e7df",
							Port:    8080,
						},
					},
				},
			}, dependsOnAgent)
		},
	})))
}

func (v *localSDKInjectionSuite) TestClusterAgentInstalled() {
	FindPodInNamespace(v.T(), v.Env().KubernetesCluster.Client(), "datadog", "cluster-agent")
}

func (v *localSDKInjectionSuite) TestExpectInjection() {
	// Get clients.
	intake := v.Env().FakeIntake.Client()
	k8s := v.Env().KubernetesCluster.Client()

	// Ensure the pod was injected.
	pod := FindPodInNamespace(v.T(), k8s, "application", DefaultAppName)
	podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
	podValidator.RequireInjection(v.T(), DefaultExpectedContainers)
	podValidator.RequireLibraryVersions(v.T(), map[string]string{
		"python": "v3.18.1",
	})
	podValidator.RequireInjectorVersion(v.T(), "0.52.0")

	// Ensure the service has traces.
	require.Eventually(v.T(), func() bool {
		traces := FindTracesForService(v.T(), intake, DefaultAppName)
		return len(traces) != 0
	}, 1*time.Minute, 10*time.Second, "did not find any traces at intake for DD_SERVICE %s", DefaultAppName)
}

func (v *localSDKInjectionSuite) TestExpectNoInjection() {
	pod := FindPodInNamespace(v.T(), v.Env().KubernetesCluster.Client(), "application", "expect-no-injection")
	podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
	podValidator.RequireNoInjection(v.T())
}
