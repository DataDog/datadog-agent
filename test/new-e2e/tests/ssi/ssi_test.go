// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ssi: single suite that provisions one cluster and calls UpdateEnv before
// each test group (injection mode, local SDK, namespace selection, workload selection)
// to update the environment instead of provisioning 4 separate clusters.

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

// ssiSuite runs all SSI test groups on a single cluster, calling UpdateEnv at the start of
// each group to update the env (workloads, helm values).
type ssiSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestSSISuite is the single entry point: one cluster is provisioned once with the base config,
// then UpdateEnv is called at the start of each test group.
func TestSSISuite(t *testing.T) {
	helmValues, err := os.ReadFile("testdata/base.yaml")
	require.NoError(t, err, "Could not open helm values file for test")
	e2e.Run(t, &ssiSuite{}, e2e.WithProvisioner(Provisioner(ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(string(helmValues)),
		},
	})))
}

func (v *ssiSuite) TestInjectionMode() {
	helmValues, err := os.ReadFile("testdata/injection_mode.yaml")
	require.NoError(v.T(), err, "Could not open helm values file for test")
	v.UpdateEnv(Provisioner(ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(string(helmValues)),
		},
		AgentDependentWorkloadAppFunc: func(e config.Env, kubeProvider *kubernetes.Provider, dependsOnAgent pulumi.ResourceOption) (*compkube.Workload, error) {
			return singlestep.Scenario(e, kubeProvider, "injection-mode", []singlestep.Namespace{
				{
					Name: "injection-mode",
					Apps: []singlestep.App{
						{
							Name:    "injection-mode-app-csi",
							Image:   "registry.datadoghq.com/injector-dev/python",
							Version: "16ad9d4b",
							Port:    8080,
							PodAnnotations: map[string]string{
								"admission.datadoghq.com/apm-inject.injection-mode": "csi",
							},
						},
						{
							Name:    "injection-mode-app-init-container",
							Image:   "registry.datadoghq.com/injector-dev/python",
							Version: "16ad9d4b",
							Port:    8080,
							PodAnnotations: map[string]string{
								"admission.datadoghq.com/apm-inject.injection-mode": "init_container",
							},
						},
					},
				},
			}, dependsOnAgent)
		},
	}))

	testCases := []struct {
		name string
		mode testutils.InjectionMode
	}{
		{"injection-mode-app-csi", testutils.InjectionModeCSI},
		{"injection-mode-app-init-container", testutils.InjectionModeInitContainer},
	}

	k8s := v.Env().KubernetesCluster.Client()
	intake := v.Env().FakeIntake.Client()

	for _, tc := range testCases {
		v.Run(tc.name, func() {
			pod := FindPodInNamespace(v.T(), k8s, "injection-mode", tc.name)
			podValidator := testutils.NewPodValidator(pod, tc.mode)
			podValidator.RequireInjection(v.T(), []string{tc.name})
			podValidator.RequireInjectorVersion(v.T(), "0.54.0")
			podValidator.RequireLibraryVersions(v.T(), map[string]string{"python": "v3.18.1"})

			require.Eventually(v.T(), func() bool {
				traces := FindTracesForService(v.T(), intake, tc.name)
				return len(traces) != 0
			}, 1*time.Minute, 10*time.Second, "did not find any traces at intake for DD_SERVICE %s", tc.name)
		})
	}
}

func (v *ssiSuite) TestLocalSDKInjection() {
	helmValues, err := os.ReadFile("testdata/local_sdk_injection.yaml")
	require.NoError(v.T(), err, "Could not open helm values file for test")
	v.UpdateEnv(Provisioner(ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(string(helmValues)),
		},
		AgentDependentWorkloadAppFunc: func(e config.Env, kubeProvider *kubernetes.Provider, dependsOnAgent pulumi.ResourceOption) (*compkube.Workload, error) {
			return singlestep.Scenario(e, kubeProvider, "local-sdk-injection", []singlestep.Namespace{
				{
					Name: "application",
					Apps: []singlestep.App{
						{
							Name:    "local-sdk-injection-app",
							Image:   "gcr.io/datadoghq/injector-dev/python",
							Version: "d425e7df",
							Port:    8080,
							PodLabels: map[string]string{
								"admission.datadoghq.com/enabled": "true",
								"tags.datadoghq.com/service":      "local-sdk-injection-app",
							},
							PodAnnotations: map[string]string{
								"admission.datadoghq.com/python-lib.version": "v3.18.1",
							},
						},
						{
							Name:    "local-sdk-expect-no-injection",
							Image:   "gcr.io/datadoghq/injector-dev/python",
							Version: "d425e7df",
							Port:    8080,
						},
					},
				},
			}, dependsOnAgent)
		},
	}))

	v.Run("ClusterAgentInstalled", func() {
		FindPodInNamespace(v.T(), v.Env().KubernetesCluster.Client(), "datadog", "cluster-agent")
	})

	v.Run("ExpectInjection", func() {
		// Get clients.
		intake := v.Env().FakeIntake.Client()
		k8s := v.Env().KubernetesCluster.Client()

		// Ensure the pod was injected.
		pod := FindPodInNamespace(v.T(), k8s, "application", "local-sdk-injection-app")
		podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
		podValidator.RequireInjection(v.T(), []string{"local-sdk-injection-app"})
		podValidator.RequireLibraryVersions(v.T(), map[string]string{
			"python": "v3.18.1",
		})
		podValidator.RequireInjectorVersion(v.T(), "0.52.0")

		// Ensure the service has traces.
		require.Eventually(v.T(), func() bool {
			traces := FindTracesForService(v.T(), intake, "local-sdk-injection-app")
			return len(traces) != 0
		}, 1*time.Minute, 10*time.Second, "did not find any traces at intake for DD_SERVICE %s", "local-sdk-injection-app")
	})

	v.Run("ExpectNoInjection", func() {
		pod := FindPodInNamespace(v.T(), v.Env().KubernetesCluster.Client(), "application", "local-sdk-expect-no-injection")
		podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
		podValidator.RequireNoInjection(v.T())
	})
}

func (v *ssiSuite) TestNamespaceSelection() {
	helmValues, err := os.ReadFile("testdata/namespace_selection.yaml")
	require.NoError(v.T(), err, "Could not open helm values file for test")
	v.UpdateEnv(Provisioner(ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(string(helmValues)),
		},
		AgentDependentWorkloadAppFunc: func(e config.Env, kubeProvider *kubernetes.Provider, dependsOnAgent pulumi.ResourceOption) (*compkube.Workload, error) {
			return singlestep.Scenario(e, kubeProvider, "namespace-selection", []singlestep.Namespace{
				{
					Name: "expect-injection",
					Apps: []singlestep.App{
						{
							Name:    "namespace-selection-inject",
							Image:   "gcr.io/datadoghq/injector-dev/python",
							Version: "d425e7df",
							Port:    8080,
						},
					},
				},
				{
					Name: "expect-no-injection",
					Apps: []singlestep.App{
						{
							Name:    "namespace-selection-no-inject",
							Image:   "gcr.io/datadoghq/injector-dev/python",
							Version: "d425e7df",
							Port:    8080,
						},
					},
				},
			}, dependsOnAgent)
		},
	}))

	v.Run("ClusterAgentInstalled", func() {
		FindPodInNamespace(v.T(), v.Env().KubernetesCluster.Client(), "datadog", "cluster-agent")
	})

	v.Run("ExpectInjection", func() {
		// Get clients.
		intake := v.Env().FakeIntake.Client()
		k8s := v.Env().KubernetesCluster.Client()

		// Ensure the pod was injected.
		pod := FindPodInNamespace(v.T(), k8s, "expect-injection", "namespace-selection-inject")
		podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
		podValidator.RequireInjection(v.T(), []string{"namespace-selection-inject"})
		podValidator.RequireLibraryVersions(v.T(), map[string]string{
			"python": "v3.18.1",
		})
		podValidator.RequireInjectorVersion(v.T(), "0.52.0")

		// Ensure the service has traces.
		require.Eventually(v.T(), func() bool {
			traces := FindTracesForService(v.T(), intake, "namespace-selection-inject")
			return len(traces) != 0
		}, 1*time.Minute, 10*time.Second, "did not find any traces at intake for DD_SERVICE %s", "namespace-selection-inject")
	})
	v.Run("ExpectNoInjection", func() {
		pods := GetPodsInNamespace(v.T(), v.Env().KubernetesCluster.Client(), "expect-no-injection")
		for _, pod := range pods {
			podValidator := testutils.NewPodValidator(&pod, testutils.InjectionModeAuto)
			podValidator.RequireNoInjection(v.T())
		}
	})
}

func (v *ssiSuite) TestWorkloadSelection() {
	helmValues, err := os.ReadFile("testdata/workload_selection.yaml")
	require.NoError(v.T(), err, "Could not open helm values file for test")
	v.UpdateEnv(Provisioner(ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(string(helmValues)),
		},
		AgentDependentWorkloadAppFunc: func(e config.Env, kubeProvider *kubernetes.Provider, dependsOnAgent pulumi.ResourceOption) (*compkube.Workload, error) {
			return singlestep.Scenario(e, kubeProvider, "workload-selection", []singlestep.Namespace{
				{
					Name: "targeted-namespace",
					Labels: map[string]string{
						"injection": "yes",
					},
					Apps: []singlestep.App{
						{
							Name:    "workload-selection-inject",
							Image:   "gcr.io/datadoghq/injector-dev/python",
							Version: "d425e7df",
							Port:    8080,
							PodLabels: map[string]string{
								"language": "python",
							},
						},
						{
							Name:    "workload-selection-expect-no-injection",
							Image:   "gcr.io/datadoghq/injector-dev/python",
							Version: "d425e7df",
							Port:    8080,
						},
					},
				},
			}, dependsOnAgent)
		},
	}))

	v.Run("ClusterAgentInstalled", func() {
		FindPodInNamespace(v.T(), v.Env().KubernetesCluster.Client(), "datadog", "cluster-agent")
	})

	v.Run("ExpectInjection", func() {
		// Get clients.
		intake := v.Env().FakeIntake.Client()
		k8s := v.Env().KubernetesCluster.Client()

		// Ensure the pod was injected.
		pod := FindPodInNamespace(v.T(), k8s, "targeted-namespace", "workload-selection-inject")
		podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
		podValidator.RequireInjection(v.T(), []string{"workload-selection-inject"})
		podValidator.RequireLibraryVersions(v.T(), map[string]string{
			"python": "v3.18.1",
		})
		podValidator.RequireInjectorVersion(v.T(), "0.52.0")

		// Ensure the service has traces.
		require.Eventually(v.T(), func() bool {
			traces := FindTracesForService(v.T(), intake, "workload-selection-inject")
			return len(traces) != 0
		}, 1*time.Minute, 10*time.Second, "did not find any traces at intake for DD_SERVICE %s", "workload-selection-inject")
	})

	v.Run("ExpectNoInjection", func() {
		pod := FindPodInNamespace(v.T(), v.Env().KubernetesCluster.Client(), "targeted-namespace", "workload-selection-expect-no-injection")
		podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
		podValidator.RequireNoInjection(v.T())
	})
}
