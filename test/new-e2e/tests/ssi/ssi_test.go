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
	"testing"
	"time"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ssi/testutils"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/singlestep"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	compkube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
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
	}{
		{"injection-mode-app-csi", testutils.InjectionModeCSI},
		{"injection-mode-app-init-container", testutils.InjectionModeInitContainer},
		{"injection-mode-app-image-volume", testutils.InjectionModeImageVolume},
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

func isOpenShift() bool {
	switch getProvisionerType() {
	case ProvisionerOpenShift, ProvisionerOpenShiftLocal:
		return true
	default:
		return false
	}
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
