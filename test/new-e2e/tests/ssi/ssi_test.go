// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ssi: single suite that provisions one cluster and calls UpdateEnv before
// each test group (injection mode, local SDK, namespace selection, workload selection)
// to update the environment instead of provisioning 4 separate clusters.

package ssi

import (
	_ "embed"
	"fmt"
	"testing"
	"time"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"
	kubeClient "k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/ssi/testutils"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/singlestep"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	compkube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

//go:embed testdata/base.yaml
var baseHelmValues string

//go:embed testdata/injection_mode.yaml
var injectionModeHelmValues string

//go:embed testdata/local_sdk_injection.yaml
var localSDKInjectionHelmValues string

//go:embed testdata/namespace_selection.yaml
var namespaceSelectionHelmValues string

//go:embed testdata/workload_selection.yaml
var workloadSelectionHelmValues string

//go:embed testdata/registry_allow_list.yaml
var registryAllowListHelmValues string

//go:embed testdata/rc_policies.yaml
var rcPoliciesHelmValues string

//go:embed testdata/rc_host_linux_only_policy.json
var rcHostLinuxOnlyPolicyJSON []byte

//go:embed testdata/rc_namespace_other_policy.json
var rcNamespaceOtherPolicyJSON []byte

//go:embed testdata/rc_deny_targeted_namespace_policy.json
var rcDenyTargetedNamespacePolicyJSON []byte

//go:embed testdata/rc_last_wins_other_policy.json
var rcLastWinsOtherPolicyJSON []byte

const (
	apmPoliciesRCProduct              = "APM_POLICIES"
	rcHostLinuxOnlyConfigID           = "1.kubernetes.host-linux-only"
	rcHostLinuxOnlyConfigName         = "config"
	rcNamespaceOtherConfigID          = "1.kubernetes.namespace-other"
	rcNamespaceOtherConfigName        = "config"
	rcDenyTargetedNamespaceConfigID   = "1.kubernetes.deny-targeted-namespace"
	rcDenyTargetedNamespaceConfigName = "config"
	rcLastWinsOtherConfigID           = "1.kubernetes.last-wins-other"
	rcLastWinsOtherConfigName         = "config"
	rcFakeIntakeDefaultOrgID          = "42"
	rcHelmTargetNamespace             = "targeted-namespace"
	rcHelmTargetApp                   = "rc-target-python"
	rcOtherNamespace                  = "other"
	rcAnnotatedPodApp                 = "rc-ann-only"
	rcUnannotatedPodApp               = "rc-unannotated"
	rcHelmTargetName                  = "python-apps"
	rcNamespaceOtherPolicyName        = "namespace other: matches admission namespace fact"
)

// ssiSuite runs all SSI test groups on a single cluster, calling UpdateEnv at the start of
// each group to update the env (workloads, helm values).
type ssiSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestSSISuite is the single entry point: one cluster is provisioned once with the base config,
// then UpdateEnv is called at the start of each test group.
func TestSSISuite(t *testing.T) {
	if getProvisionerType() == ProvisionerAKS {
		flake.Mark(t)
	}

	opts := ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(baseHelmValues),
		},
	}
	if isOpenShift() {
		opts.PreAgentHook = openShiftSCC
	}

	e2e.Run(t, &ssiSuite{}, e2e.WithProvisioner(Provisioner(opts)))
}

func (v *ssiSuite) TestInjectionMode() {
	var namespaceLabels map[string]string
	var csiPodSecurityContext *corev1.PodSecurityContextArgs
	var csiContainerSecurityContext *corev1.SecurityContextArgs
	opts := ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(injectionModeHelmValues),
		},
		AgentDependentWorkloadAppFunc: func(e config.Env, kubeProvider *kubernetes.Provider, dependsOnAgent pulumi.ResourceOption) (*compkube.Workload, error) {
			return singlestep.Scenario(e, kubeProvider, "injection-mode", []singlestep.Namespace{
				{
					Name:   "injection-mode",
					Labels: namespaceLabels,
					Apps: []singlestep.App{
						{
							Name:                     "injection-mode-app-csi",
							Image:                    "registry.datadoghq.com/injector-dev/python",
							Version:                  "16ad9d4b",
							Port:                     8080,
							PodSecurityContext:       csiPodSecurityContext,
							ContainerSecurityContext: csiContainerSecurityContext,
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
						{
							Name:    "injection-mode-app-image-volume",
							Image:   "registry.datadoghq.com/injector-dev/python",
							Version: "16ad9d4b",
							Port:    8080,
							PodAnnotations: map[string]string{
								"admission.datadoghq.com/apm-inject.injection-mode": "image_volume",
							},
						},
						{
							// The cluster-agent is started with
							// DD_APM_INSTRUMENTATION_CSI_DRIVER_DETECTION_ENABLED=true
							// (see injection_mode.yaml) and the Datadog CSI driver is
							// installed in this suite. The AutoProvider must therefore
							// pick the CSI provider for this "auto" pod. The pod
							// security context mirrors the csi pod since the resulting
							// volume is a CSI volume.
							Name:                     "injection-mode-app-auto",
							Image:                    "registry.datadoghq.com/injector-dev/python",
							Version:                  "16ad9d4b",
							Port:                     8080,
							PodSecurityContext:       csiPodSecurityContext,
							ContainerSecurityContext: csiContainerSecurityContext,
							PodAnnotations: map[string]string{
								"admission.datadoghq.com/apm-inject.injection-mode": "auto",
							},
						},
					},
				},
			}, dependsOnAgent)
		},
	}
	if isOpenShift() {
		opts.PreAgentHook = openShiftSCC
		namespaceLabels = openShiftInjectionModeNamespaceLabels()
		csiPodSecurityContext, csiContainerSecurityContext = openShiftCSIAppSecurityContexts()
	}

	v.UpdateEnv(Provisioner(opts))

	testCases := []struct {
		name string
		mode testutils.InjectionMode
		// effectiveMode is the value the webhook records in the
		// internal.apm.datadoghq.com/effective-injection-mode annotation. For
		// explicit modes it is the bare mode; for "auto" it is the resolved
		// mode suffixed with " (auto)".
		effectiveMode string
	}{
		{"injection-mode-app-csi", testutils.InjectionModeCSI, string(testutils.InjectionModeCSI)},
		{"injection-mode-app-init-container", testutils.InjectionModeInitContainer, string(testutils.InjectionModeInitContainer)},
		{"injection-mode-app-image-volume", testutils.InjectionModeImageVolume, string(testutils.InjectionModeImageVolume)},
		// "auto" mode with CSI auto-detection enabled and the Datadog CSI
		// driver installed must resolve to the CSI provider.
		{"injection-mode-app-auto", testutils.InjectionModeCSI, testutils.EffectiveAutoMode(testutils.InjectionModeCSI)},
	}

	k8s := v.Env().KubernetesCluster.Client()
	intake := v.Env().FakeIntake.Client()

	for _, tc := range testCases {
		v.Run(tc.name, func() {
			pod := WaitForMutatedPodInNamespace(v.T(), k8s, "injection-mode", tc.name)

			// The cluster-agent's CSI driver watcher learns about the CSIDriver
			// asynchronously from workloadmeta. The auto pod may have been admitted
			// before the watcher synced, resolving it to init_container (pods are never
			// re-mutated). If the pod did not observe the APM-enabled driver, re-admit it
			// once: this case runs last, so the watcher is synced by now and the fresh
			// admission resolves to the CSI provider.
			if tc.name == "injection-mode-app-auto" && pod.Annotations[testutils.CSIDriverStatusAnnotation] != testutils.CSIDriverStatusAPMEnabled {
				RestartPod(v.T(), k8s, "injection-mode", tc.name)
				pod = FindPodInNamespace(v.T(), k8s, "injection-mode", tc.name)
			}

			podValidator := testutils.NewPodValidator(pod, tc.mode)
			podValidator.RequireInjection(v.T(), []string{tc.name})
			podValidator.RequireInjectorVersion(v.T(), "0.54.0")
			podValidator.RequireLibraryVersions(v.T(), map[string]string{"python": "v3.18.1"})

			// Validate the webhook outcome annotations.
			podValidator.RequireEffectiveInjectionMode(v.T(), tc.effectiveMode)
			podValidator.RequireInjectionStatus(v.T(), testutils.InjectionStatusInjected)
			podValidator.RequireInjectedLibraries(v.T(), map[string]string{"injector": "injected", "python": "injected"})

			// csi-driver-status records the watcher's state at admission time for every
			// mode, but only the auto pod is re-admitted above once the watcher has synced.
			// The explicit modes do not depend on the watcher and may carry a stale status
			// if they were admitted during the initial sync window, so only assert it here.
			if tc.name == "injection-mode-app-auto" {
				podValidator.RequireCSIDriverStatus(v.T(), testutils.CSIDriverStatusAPMEnabled)
			}

			require.Eventually(v.T(), func() bool {
				traces := FindTracesForService(v.T(), intake, tc.name)
				return traces != 0
			}, 1*time.Minute, 10*time.Second, "did not find any traces at intake for DD_SERVICE %s", tc.name)
		})
	}
}

func (v *ssiSuite) TestLocalSDKInjection() {
	v.UpdateEnv(Provisioner(ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(localSDKInjectionHelmValues),
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
		pod := WaitForMutatedPodInNamespace(v.T(), k8s, "application", "local-sdk-injection-app")
		podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
		podValidator.RequireInjection(v.T(), []string{"local-sdk-injection-app"})
		podValidator.RequireLibraryVersions(v.T(), map[string]string{
			"python": "v3.18.1",
		})
		podValidator.RequireInjectorVersion(v.T(), "0.52.0")

		// CSI driver detection is not enabled in this suite, so "auto" mode
		// resolves to init containers and the webhook reports a successful injection.
		// The csi-driver-status annotation must be absent since detection is off.
		podValidator.RequireEffectiveInjectionMode(v.T(), testutils.EffectiveAutoMode(testutils.InjectionModeInitContainer))
		podValidator.RequireInjectionStatus(v.T(), testutils.InjectionStatusInjected)
		podValidator.RequireInjectedLibraries(v.T(), map[string]string{"injector": "injected", "python": "injected"})
		podValidator.RequireMissingAnnotations(v.T(), []string{testutils.CSIDriverStatusAnnotation})

		// Ensure the service has traces.
		require.Eventually(v.T(), func() bool {
			traces := FindTracesForService(v.T(), intake, "local-sdk-injection-app")
			return traces != 0
		}, 1*time.Minute, 10*time.Second, "did not find any traces at intake for DD_SERVICE %s", "local-sdk-injection-app")
	})

	v.Run("ExpectNoInjection", func() {
		pod := FindPodInNamespace(v.T(), v.Env().KubernetesCluster.Client(), "application", "local-sdk-expect-no-injection")
		podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
		podValidator.RequireNoInjection(v.T())
	})
}

func (v *ssiSuite) TestNamespaceSelection() {
	v.UpdateEnv(Provisioner(ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(namespaceSelectionHelmValues),
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
		pod := WaitForMutatedPodInNamespace(v.T(), k8s, "expect-injection", "namespace-selection-inject")
		podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
		podValidator.RequireInjection(v.T(), []string{"namespace-selection-inject"})
		podValidator.RequireLibraryVersions(v.T(), map[string]string{
			"python": "v3.18.1",
		})
		podValidator.RequireInjectorVersion(v.T(), "0.52.0")

		// CSI driver detection is not enabled in this suite, so "auto" mode
		// resolves to init containers and the webhook reports a successful injection.
		// The csi-driver-status annotation must be absent since detection is off.
		podValidator.RequireEffectiveInjectionMode(v.T(), testutils.EffectiveAutoMode(testutils.InjectionModeInitContainer))
		podValidator.RequireInjectionStatus(v.T(), testutils.InjectionStatusInjected)
		podValidator.RequireInjectedLibraries(v.T(), map[string]string{"injector": "injected", "python": "injected"})
		podValidator.RequireMissingAnnotations(v.T(), []string{testutils.CSIDriverStatusAnnotation})

		// Ensure the service has traces.
		require.Eventually(v.T(), func() bool {
			traces := FindTracesForService(v.T(), intake, "namespace-selection-inject")
			return traces != 0
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
	v.UpdateEnv(Provisioner(ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(workloadSelectionHelmValues),
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
		pod := WaitForMutatedPodInNamespace(v.T(), k8s, "targeted-namespace", "workload-selection-inject")
		podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
		podValidator.RequireInjection(v.T(), []string{"workload-selection-inject"})
		podValidator.RequireLibraryVersions(v.T(), map[string]string{
			"python": "v3.18.1",
		})
		podValidator.RequireInjectorVersion(v.T(), "0.52.0")

		// CSI driver detection is not enabled in this suite, so "auto" mode
		// resolves to init containers and the webhook reports a successful injection.
		// The csi-driver-status annotation must be absent since detection is off.
		podValidator.RequireEffectiveInjectionMode(v.T(), testutils.EffectiveAutoMode(testutils.InjectionModeInitContainer))
		podValidator.RequireInjectionStatus(v.T(), testutils.InjectionStatusInjected)
		podValidator.RequireInjectedLibraries(v.T(), map[string]string{"injector": "injected", "python": "injected"})
		podValidator.RequireMissingAnnotations(v.T(), []string{testutils.CSIDriverStatusAnnotation})

		// Ensure the service has traces.
		require.Eventually(v.T(), func() bool {
			traces := FindTracesForService(v.T(), intake, "workload-selection-inject")
			return traces != 0
		}, 1*time.Minute, 10*time.Second, "did not find any traces at intake for DD_SERVICE %s", "workload-selection-inject")
	})

	v.Run("ExpectNoInjection", func() {
		pod := FindPodInNamespace(v.T(), v.Env().KubernetesCluster.Client(), "targeted-namespace", "workload-selection-expect-no-injection")
		podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
		podValidator.RequireNoInjection(v.T())
	})
}

func (v *ssiSuite) TestRegistryAllowList() {
	if isGKEAutopilot() {
		v.T().Skip("registry allow-list is cluster-agent logic already covered by other provisioners; " +
			"on GKE Autopilot the Helm chart forces images to gcr.io/datadoghq, which is unrelated to SSI behavior")
	}
	// All three apps run in the same cluster with allow list = registry.datadoghq.com.
	// The default container registry for injector and library images is registry.datadoghq.com.
	// - "allowed": default injector and library, both from registry.datadoghq.com — injection proceeds.
	// - "injector-blocked": injector image overridden to fake.registry.invalid — injection blocked.
	// - "library-blocked": injector is allowed, but python-lib.custom-image points to
	//   fake.registry.invalid — injection blocked by library registry check.
	v.UpdateEnv(Provisioner(ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(registryAllowListHelmValues),
		},
		AgentDependentWorkloadAppFunc: func(e config.Env, kubeProvider *kubernetes.Provider, dependsOnAgent pulumi.ResourceOption) (*compkube.Workload, error) {
			return singlestep.Scenario(e, kubeProvider, "registry-allow-list", []singlestep.Namespace{
				{
					Name: "registry-allow-list",
					Apps: []singlestep.App{
						{
							Name:    "registry-allow-list-allowed",
							Image:   "registry.datadoghq.com/injector-dev/python",
							Version: "16ad9d4b",
							Port:    8080,
						},
						{
							Name:    "registry-allow-list-injector-blocked",
							Image:   "registry.datadoghq.com/injector-dev/python",
							Version: "16ad9d4b",
							Port:    8080,
							PodAnnotations: map[string]string{
								// Override injector to a registry not in the allow list.
								"admission.datadoghq.com/apm-inject.custom-image": "fake.registry.invalid/apm-inject:0.54.0",
							},
						},
						{
							Name:    "registry-allow-list-library-blocked",
							Image:   "registry.datadoghq.com/injector-dev/python",
							Version: "16ad9d4b",
							Port:    8080,
							// admission.datadoghq.com/enabled triggers annotation-based (local SDK)
							// injection so the webhook processes python-lib.custom-image. Without
							// this label, the target match takes over and the annotation is ignored.
							PodLabels: map[string]string{
								"admission.datadoghq.com/enabled": "true",
							},
							PodAnnotations: map[string]string{
								// Override python library to a registry not in the allow list.
								"admission.datadoghq.com/python-lib.custom-image": "fake.registry.invalid/dd-lib-python-init:v3.18.1",
							},
						},
					},
				},
			}, dependsOnAgent)
		},
	}))

	v.Run("InjectionAllowedByAllowList", func() {
		intake := v.Env().FakeIntake.Client()
		k8s := v.Env().KubernetesCluster.Client()

		pod := WaitForMutatedPodInNamespace(v.T(), k8s, "registry-allow-list", "registry-allow-list-allowed")
		podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
		podValidator.RequireInjection(v.T(), []string{"registry-allow-list-allowed"})
		podValidator.RequireInjectorVersion(v.T(), "0.54.0")
		podValidator.RequireLibraryVersions(v.T(), map[string]string{"python": "v3.18.1"})
		podValidator.RequireEffectiveInjectionMode(v.T(), testutils.EffectiveAutoMode(testutils.InjectionModeInitContainer))
		podValidator.RequireInjectionStatus(v.T(), testutils.InjectionStatusInjected)
		podValidator.RequireInjectedLibraries(v.T(), map[string]string{"injector": "injected", "python": "injected"})
		podValidator.RequireMissingAnnotations(v.T(), []string{testutils.CSIDriverStatusAnnotation})

		require.Eventually(v.T(), func() bool {
			traces := FindTracesForService(v.T(), intake, "registry-allow-list-allowed")
			return traces != 0
		}, 1*time.Minute, 10*time.Second, "did not find any traces at intake for DD_SERVICE %s", "registry-allow-list-allowed")
	})

	v.Run("InjectorRegistryBlockedByAllowList", func() {
		k8s := v.Env().KubernetesCluster.Client()
		pod := WaitForMutatedPodInNamespace(v.T(), k8s, "registry-allow-list", "registry-allow-list-injector-blocked")

		// The injector image is overridden to fake.registry.invalid via pod annotation,
		// which is not in the allow list. No SSI artifacts should be injected, even if KPI
		// env vars such as DD_INSTRUMENTATION_INSTALL_TYPE are still present.
		podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
		podValidator.RequireNoInjectionArtifacts(v.T())
		// The allow-list rejection happens before injection starts, so the webhook
		// records a "skipped" status alongside the injection-error annotation.
		podValidator.RequireInjectionStatus(v.T(), testutils.InjectionStatusSkipped)

		errAnnotation := pod.Annotations["internal.apm.datadoghq.com/injection-error"]
		require.NotEmpty(v.T(), errAnnotation, "expected injection-error annotation to be set")
		require.Contains(v.T(), errAnnotation, "not in the allow list")
	})

	v.Run("LibraryRegistryBlockedByAllowList", func() {
		k8s := v.Env().KubernetesCluster.Client()
		pod := WaitForMutatedPodInNamespace(v.T(), k8s, "registry-allow-list", "registry-allow-list-library-blocked")

		// The injector is from the allowed registry, but the python library is overridden
		// to fake.registry.invalid via annotation. No SSI artifacts should be injected, even
		// if KPI env vars such as DD_INSTRUMENTATION_INSTALL_TYPE are still present.
		podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
		podValidator.RequireNoInjectionArtifacts(v.T())
		// The allow-list rejection happens before injection starts, so the webhook
		// records a "skipped" status alongside the injection-error annotation.
		podValidator.RequireInjectionStatus(v.T(), testutils.InjectionStatusSkipped)

		errAnnotation := pod.Annotations["internal.apm.datadoghq.com/injection-error"]
		require.NotEmpty(v.T(), errAnnotation, "expected injection-error annotation to be set")
		require.Contains(v.T(), errAnnotation, "not in the allow list")
	})
}

func (v *ssiSuite) TestRemoteConfig() {
	// rc_policies.yaml: helm SSI target (namespace injection=yes, pod language=python) plus two
	// workloads in namespace "other", outside that target: annotated lib-injection and unannotated.
	// RC uptake is asserted by pod mutation. RestartUntil re-admits until the
	// expected outcome appears so we do not race the cluster-agent RC client.
	agentOptions := []kubernetesagentparams.Option{
		kubernetesagentparams.WithHelmValues(rcPoliciesHelmValues),
		kubernetesagentparams.WithTimeout(600),
	}
	provisionerOpts := ProvisionerOptions{
		AgentOptions: agentOptions,
		AgentDependentWorkloadAppFunc: func(e config.Env, kubeProvider *kubernetes.Provider, dependsOnAgent pulumi.ResourceOption) (*compkube.Workload, error) {
			return singlestep.Scenario(e, kubeProvider, "rc-policies", []singlestep.Namespace{
				{
					Name: rcHelmTargetNamespace,
					Labels: map[string]string{
						"injection": "yes",
					},
					Apps: []singlestep.App{
						{
							Name:    rcHelmTargetApp,
							Image:   "gcr.io/datadoghq/injector-dev/python",
							Version: "d425e7df",
							Port:    8080,
							PodLabels: map[string]string{
								"language": "python",
							},
						},
					},
				},
				{
					Name: rcOtherNamespace,
					Apps: []singlestep.App{
						{
							Name:    rcAnnotatedPodApp,
							Image:   "gcr.io/datadoghq/injector-dev/python",
							Version: "d425e7df",
							Port:    8080,
							PodLabels: map[string]string{
								"admission.datadoghq.com/enabled": "true",
							},
							PodAnnotations: map[string]string{
								"admission.datadoghq.com/python-lib.version": "v3.18.1",
							},
						},
						{
							Name:    rcUnannotatedPodApp,
							Image:   "gcr.io/datadoghq/injector-dev/python",
							Version: "d425e7df",
							Port:    8080,
						},
					},
				},
			}, dependsOnAgent)
		},
	}

	v.UpdateEnv(Provisioner(provisionerOpts))

	fi := v.Env().FakeIntake.Client()

	// Host-only RC cannot match K8s admission facts. A matching policy is published first so
	// the annotated pod leaves the helm baseline; replacing that same RC document with the
	// host-only payload must restore lib-injection. Delete-then-add would restore the
	// baseline as soon as the matching policy is gone, before host-only is applied.
	v.Run("HostOnlyPolicyDoesNotMatchK8s", func() {
		k8s := v.Env().KubernetesCluster.Client()

		cleanup := v.pushAPMPolicy(fi, rcHostLinuxOnlyConfigID, rcHostLinuxOnlyConfigName, rcNamespaceOtherPolicyJSON)
		defer cleanup()

		RestartUntil(v.T(), k8s, rcOtherNamespace, rcAnnotatedPodApp, hasInstallType(rcAnnotatedPodApp, "k8s_single_step"))

		_ = v.pushAPMPolicy(fi, rcHostLinuxOnlyConfigID, rcHostLinuxOnlyConfigName, rcHostLinuxOnlyPolicyJSON)
		pod := RestartUntil(v.T(), k8s, rcOtherNamespace, rcAnnotatedPodApp, hasInstallType(rcAnnotatedPodApp, "k8s_lib_injection"))

		podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
		podValidator.RequireInjection(v.T(), []string{rcAnnotatedPodApp})
		podValidator.RequireInstallType(v.T(), "k8s_lib_injection", []string{rcAnnotatedPodApp})
		podValidator.RequireMissingEnvs(v.T(), []string{"DD_TRACE_ENABLED"}, []string{rcAnnotatedPodApp})
		// Library annotations short-circuit applied-target / applied-policy metadata.
		podValidator.RequireMissingAnnotations(v.T(), []string{testutils.AppliedTargetAnnotation, testutils.AppliedPolicyAnnotation})

		RestartPod(v.T(), k8s, rcOtherNamespace, rcUnannotatedPodApp)
		unannotated := FindPodInNamespace(v.T(), k8s, rcOtherNamespace, rcUnannotatedPodApp)
		unannotatedValidator := testutils.NewPodValidator(unannotated, testutils.InjectionModeAuto)
		unannotatedValidator.RequireNoInjection(v.T())
		unannotatedValidator.RequireMissingAnnotations(v.T(), []string{testutils.AppliedTargetAnnotation, testutils.AppliedPolicyAnnotation})

		v.requireHelmTargetStillSSI(k8s)
	})

	// RC policy matching namespace "other" enables SSI on the annotated pod and on the unannotated
	// pod (true on-demand, still outside the helm target). Helm local targeting is unchanged.
	v.Run("NamespacePolicyEnablesSSIOutsideHelmTarget", func() {
		k8s := v.Env().KubernetesCluster.Client()

		cleanup := v.pushAPMPolicy(fi, rcNamespaceOtherConfigID, rcNamespaceOtherConfigName, rcNamespaceOtherPolicyJSON)
		defer cleanup()

		pod := RestartUntil(v.T(), k8s, rcOtherNamespace, rcAnnotatedPodApp, hasInstallType(rcAnnotatedPodApp, "k8s_single_step"))

		podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
		podValidator.RequireInjection(v.T(), []string{rcAnnotatedPodApp})
		podValidator.RequireInstallType(v.T(), "k8s_single_step", []string{rcAnnotatedPodApp})
		podValidator.RequireEnvs(v.T(), map[string]string{"DD_TRACE_ENABLED": "true"}, []string{rcAnnotatedPodApp})
		// SSI mode follows the policy match; annotation short-circuit still skips applied-policy JSON.
		podValidator.RequireMissingAnnotations(v.T(), []string{testutils.AppliedTargetAnnotation, testutils.AppliedPolicyAnnotation})

		RestartPod(v.T(), k8s, rcOtherNamespace, rcUnannotatedPodApp)
		unannotated := WaitForMutatedPodInNamespace(v.T(), k8s, rcOtherNamespace, rcUnannotatedPodApp)
		unannotatedValidator := testutils.NewPodValidator(unannotated, testutils.InjectionModeAuto)
		unannotatedValidator.RequireInjection(v.T(), []string{rcUnannotatedPodApp})
		unannotatedValidator.RequireInstallType(v.T(), "k8s_single_step", []string{rcUnannotatedPodApp})
		unannotatedValidator.RequireEnvs(v.T(), map[string]string{"DD_TRACE_ENABLED": "true"}, []string{rcUnannotatedPodApp})
		unannotatedValidator.RequireAppliedPolicyName(v.T(), rcNamespaceOtherPolicyName)
		unannotatedValidator.RequireMissingAnnotations(v.T(), []string{testutils.AppliedTargetAnnotation})

		v.requireHelmTargetStillSSI(k8s)
	})

	// A remote deny matching targeted-namespace overrides the helm python
	// target. An allow matching "other" is published first so a pod leaves
	// the helm baseline; replacing that same document with the deny must
	// restore lib-injection in "other" (the deny does not match that
	// namespace) and uninject the helm-targeted workload.
	v.Run("RemoteDenyOverridesHelmTarget", func() {
		k8s := v.Env().KubernetesCluster.Client()

		cleanup := v.pushAPMPolicy(fi, rcDenyTargetedNamespaceConfigID, rcDenyTargetedNamespaceConfigName, rcNamespaceOtherPolicyJSON)
		defer cleanup()

		RestartUntil(v.T(), k8s, rcOtherNamespace, rcAnnotatedPodApp, hasInstallType(rcAnnotatedPodApp, "k8s_single_step"))

		_ = v.pushAPMPolicy(fi, rcDenyTargetedNamespaceConfigID, rcDenyTargetedNamespaceConfigName, rcDenyTargetedNamespacePolicyJSON)
		annotated := RestartUntil(v.T(), k8s, rcOtherNamespace, rcAnnotatedPodApp, hasInstallType(rcAnnotatedPodApp, "k8s_lib_injection"))
		annotatedValidator := testutils.NewPodValidator(annotated, testutils.InjectionModeAuto)
		annotatedValidator.RequireInjection(v.T(), []string{rcAnnotatedPodApp})
		annotatedValidator.RequireInstallType(v.T(), "k8s_lib_injection", []string{rcAnnotatedPodApp})
		annotatedValidator.RequireMissingEnvs(v.T(), []string{"DD_TRACE_ENABLED"}, []string{rcAnnotatedPodApp})
		annotatedValidator.RequireMissingAnnotations(v.T(), []string{testutils.AppliedTargetAnnotation, testutils.AppliedPolicyAnnotation})

		RestartPod(v.T(), k8s, rcOtherNamespace, rcUnannotatedPodApp)
		unannotated := FindPodInNamespace(v.T(), k8s, rcOtherNamespace, rcUnannotatedPodApp)
		unannotatedValidator := testutils.NewPodValidator(unannotated, testutils.InjectionModeAuto)
		unannotatedValidator.RequireNoInjection(v.T())
		unannotatedValidator.RequireMissingAnnotations(v.T(), []string{testutils.AppliedTargetAnnotation, testutils.AppliedPolicyAnnotation})

		helm := RestartUntil(v.T(), k8s, rcHelmTargetNamespace, rcHelmTargetApp, noInjection(rcHelmTargetApp))
		helmValidator := testutils.NewPodValidator(helm, testutils.InjectionModeAuto)
		helmValidator.RequireNoInjection(v.T())
		helmValidator.RequireMissingAnnotations(v.T(), []string{testutils.AppliedTargetAnnotation, testutils.AppliedPolicyAnnotation})
	})

	// Two RC policies both match namespace "other": allow then deny. Last TRUE wins,
	// so the unannotated pod is uninjected. An allow-only document is published first
	// so the pod leaves the helm baseline; replacing it with allow+deny must restore
	// no-injection. Delete-then-add would restore the baseline as soon as the allow is
	// gone, before last-wins is applied. First-wins would keep SSI after the replace.
	v.Run("LastMatchingRemotePolicyWins", func() {
		k8s := v.Env().KubernetesCluster.Client()

		cleanup := v.pushAPMPolicy(fi, rcLastWinsOtherConfigID, rcLastWinsOtherConfigName, rcNamespaceOtherPolicyJSON)
		defer cleanup()

		RestartUntil(v.T(), k8s, rcOtherNamespace, rcUnannotatedPodApp, hasInstallType(rcUnannotatedPodApp, "k8s_single_step"))

		_ = v.pushAPMPolicy(fi, rcLastWinsOtherConfigID, rcLastWinsOtherConfigName, rcLastWinsOtherPolicyJSON)
		unannotated := RestartUntil(v.T(), k8s, rcOtherNamespace, rcUnannotatedPodApp, noInjection(rcUnannotatedPodApp))
		unannotatedValidator := testutils.NewPodValidator(unannotated, testutils.InjectionModeAuto)
		unannotatedValidator.RequireNoInjection(v.T())
		unannotatedValidator.RequireMissingAnnotations(v.T(), []string{testutils.AppliedTargetAnnotation, testutils.AppliedPolicyAnnotation})

		RestartPod(v.T(), k8s, rcOtherNamespace, rcAnnotatedPodApp)
		annotated := WaitForMutatedPodInNamespace(v.T(), k8s, rcOtherNamespace, rcAnnotatedPodApp)
		annotatedValidator := testutils.NewPodValidator(annotated, testutils.InjectionModeAuto)
		annotatedValidator.RequireInjection(v.T(), []string{rcAnnotatedPodApp})
		annotatedValidator.RequireInstallType(v.T(), "k8s_lib_injection", []string{rcAnnotatedPodApp})
		annotatedValidator.RequireMissingEnvs(v.T(), []string{"DD_TRACE_ENABLED"}, []string{rcAnnotatedPodApp})
		annotatedValidator.RequireMissingAnnotations(v.T(), []string{testutils.AppliedTargetAnnotation, testutils.AppliedPolicyAnnotation})

		v.requireHelmTargetStillSSI(k8s)
	})
}

func (v *ssiSuite) requireHelmTargetStillSSI(k8s kubeClient.Interface) {
	v.T().Helper()

	pod := RestartUntil(v.T(), k8s, rcHelmTargetNamespace, rcHelmTargetApp, hasInstallType(rcHelmTargetApp, "k8s_single_step"))
	podValidator := testutils.NewPodValidator(pod, testutils.InjectionModeAuto)
	podValidator.RequireInjection(v.T(), []string{rcHelmTargetApp})
	podValidator.RequireInstallType(v.T(), "k8s_single_step", []string{rcHelmTargetApp})
	podValidator.RequireLibraryVersions(v.T(), map[string]string{"python": "v3.18.1"})
	podValidator.RequireAppliedTargetName(v.T(), rcHelmTargetName)
	podValidator.RequireMissingAnnotations(v.T(), []string{testutils.AppliedPolicyAnnotation})
}

func (v *ssiSuite) pushAPMPolicy(fi *fakeintake.Client, configID, configName string, payload []byte) func() {
	v.T().Helper()
	require.NoError(v.T(), fi.RCAddConfig("", apmPoliciesRCProduct, configID, configName, payload))
	return func() {
		require.NoError(v.T(), fi.RCDeleteConfig(fmt.Sprintf("%s/%s/%s/%s", rcFakeIntakeDefaultOrgID, apmPoliciesRCProduct, configID, configName)))
	}
}

func isOpenShift() bool {
	switch getProvisionerType() {
	case ProvisionerOpenShift, ProvisionerOpenShiftLocal:
		return true
	default:
		return false
	}
}

func isGKEAutopilot() bool {
	return getProvisionerType() == ProvisionerGKEAutopilot
}

func openShiftInjectionModeNamespaceLabels() map[string]string {
	return map[string]string{
		"pod-security.kubernetes.io/enforce": "privileged",
		"pod-security.kubernetes.io/warn":    "privileged",
		"pod-security.kubernetes.io/audit":   "privileged",
	}
}

func openShiftCSIAppSecurityContexts() (*corev1.PodSecurityContextArgs, *corev1.SecurityContextArgs) {
	return &corev1.PodSecurityContextArgs{
			SeLinuxOptions: &corev1.SELinuxOptionsArgs{
				User:  pulumi.String("system_u"),
				Role:  pulumi.String("system_r"),
				Type:  pulumi.String("spc_t"),
				Level: pulumi.String("s0"),
			},
		}, &corev1.SecurityContextArgs{
			Privileged:               pulumi.Bool(true),
			AllowPrivilegeEscalation: pulumi.Bool(true),
			RunAsUser:                pulumi.Int(0),
			RunAsNonRoot:             pulumi.Bool(false),
		}
}

func openShiftSCC(e config.Env, kubeProvider *kubernetes.Provider) error {
	resourceOpts := []pulumi.ResourceOption{pulumi.Provider(kubeProvider)}

	for _, binding := range []struct {
		name      string
		roleName  string
		namespace string
	}{
		{name: "datadog-csi-driver-privileged", roleName: "system:openshift:scc:privileged", namespace: "datadog"},
		{name: "datadog-csi-driver-hostmount-anyuid", roleName: "system:openshift:scc:hostmount-anyuid", namespace: "datadog"},
		{name: "injection-mode-privileged", roleName: "system:openshift:scc:privileged", namespace: "injection-mode"},
		{name: "injection-mode-hostmount-anyuid", roleName: "system:openshift:scc:hostmount-anyuid", namespace: "injection-mode"},
	} {
		if _, err := rbacv1.NewClusterRoleBinding(e.Ctx(), e.CommonNamer().ResourceName(binding.name), &rbacv1.ClusterRoleBindingArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String(binding.name),
			},
			RoleRef: &rbacv1.RoleRefArgs{
				ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
				Kind:     pulumi.String("ClusterRole"),
				Name:     pulumi.String(binding.roleName),
			},
			Subjects: rbacv1.SubjectArray{
				&rbacv1.SubjectArgs{
					Kind:      pulumi.String("ServiceAccount"),
					Name:      pulumi.String("default"),
					Namespace: pulumi.String(binding.namespace),
				},
			},
		}, resourceOpts...); err != nil {
			return err
		}
	}

	return nil
}
