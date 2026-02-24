// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ssi

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/ssi/testutils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/singlestep"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	compkube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

type injectionModeSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestInjectionModeSuite(t *testing.T) {
	helmValues, err := os.ReadFile("testdata/injection_mode.yaml")
	require.NoError(t, err, "Could not open helm values file for test")
	e2e.Run(t, &injectionModeSuite{}, e2e.WithProvisioner(Provisioner(ProvisionerOptions{
		KubernetesVersion: "1.35",
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(string(helmValues)),
		},
		AgentDependentWorkloadAppFunc: func(e config.Env, kubeProvider *kubernetes.Provider, dependsOnAgent pulumi.ResourceOption) (*compkube.Workload, error) {
			return singlestep.Scenario(e, kubeProvider, "injection-mode", []singlestep.Namespace{
				{
					Name: "injection-mode",
					Apps: []singlestep.App{
						{
							Name:    "app-csi",
							Image:   "registry.datadoghq.com/injector-dev/python",
							Version: "16ad9d4b",
							Port:    8080,
							PodAnnotations: map[string]string{
								// Explicitly use CSI injection mode
								"admission.datadoghq.com/apm-inject.injection-mode": "csi",
							},
						},
						{
							Name:    "app-init-container",
							Image:   "registry.datadoghq.com/injector-dev/python",
							Version: "16ad9d4b",
							Port:    8080,
							PodAnnotations: map[string]string{
								// Explicitly use init_container injection mode
								"admission.datadoghq.com/apm-inject.injection-mode": "init_container",
							},
						},
						{
							Name:    "app-image-volume",
							Image:   "registry.datadoghq.com/injector-dev/python",
							Version: "16ad9d4b",
							Port:    8080,
							PodAnnotations: map[string]string{
								// Explicitly use image_volume injection mode
								"admission.datadoghq.com/apm-inject.injection-mode": "image_volume",
							},
						},
					},
				},
			}, dependsOnAgent)
		},
	})))
}

func (v *injectionModeSuite) TestInjectionModes() {
	testCases := []struct {
		name string
		mode testutils.InjectionMode
	}{
		{"app-csi", testutils.InjectionModeCSI},
		{"app-init-container", testutils.InjectionModeInitContainer},
		{"app-image-volume", testutils.InjectionModeImageVolume},
	}

	k8s := v.Env().KubernetesCluster.Client()
	intake := v.Env().FakeIntake.Client()

	for _, tc := range testCases {
		v.Run(tc.name, func() {
			pod := FindPodInNamespace(v.T(), k8s, "injection-mode", tc.name)
			if tc.name == "app-image-volume" {
				logPodDiagnostics(v.T(), pod, tc.name)
			}
			podValidator := testutils.NewPodValidator(pod, tc.mode)

			podValidator.RequireInjection(v.T(), []string{tc.name})
			podValidator.RequireInjectorVersion(v.T(), "0.54.0")
			podValidator.RequireLibraryVersions(v.T(), map[string]string{
				"python": "v3.18.1",
			})

			// Ensure the service has traces (proves injection actually works)
			require.Eventually(v.T(), func() bool {
				traces := FindTracesForService(v.T(), intake, tc.name)
				return len(traces) != 0
			}, 1*time.Minute, 10*time.Second, "did not find any traces at intake for DD_SERVICE %s", tc.name)
		})
	}
}

func logPodDiagnostics(t *testing.T, pod *corev1.Pod, containerName string) {
	t.Helper()
	t.Logf("=== Pod diagnostics for %s (pod=%s) ===", containerName, pod.Name)
	t.Logf("Annotations: %s", mustJSON(pod.Annotations))

	t.Logf("Volumes (%d):", len(pod.Spec.Volumes))
	for _, v := range pod.Spec.Volumes {
		t.Logf("  - %s (image=%v, emptyDir=%v, csi=%v)",
			v.Name, v.VolumeSource.Image != nil, v.VolumeSource.EmptyDir != nil, v.VolumeSource.CSI != nil)
	}

	t.Logf("Init containers (%d):", len(pod.Spec.InitContainers))
	for _, c := range pod.Spec.InitContainers {
		t.Logf("  - %s image=%s", c.Name, c.Image)
	}

	for _, c := range pod.Spec.Containers {
		t.Logf("Container %q volume mounts (%d):", c.Name, len(c.VolumeMounts))
		for _, m := range c.VolumeMounts {
			t.Logf("  - name=%s mountPath=%s subPath=%s readOnly=%v", m.Name, m.MountPath, m.SubPath, m.ReadOnly)
		}
		t.Logf("Container %q env vars (%d):", c.Name, len(c.Env))
		for _, e := range c.Env {
			if e.Value != "" {
				t.Logf("  - %s=%s", e.Name, e.Value)
			} else {
				t.Logf("  - %s (valueFrom)", e.Name)
			}
		}
	}
	t.Logf("=== End pod diagnostics ===")
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
