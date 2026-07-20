// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package envoygateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/record"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/sidecar"
)

func envoyGatewaySidecarListKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		gatewayGVR:        "GatewayList",
		extensionGVR:      "EnvoyExtensionPolicyList",
		backendGVR:        "BackendList",
		configMapGVR:      "ConfigMapList",
		referenceGrantGVR: "ReferenceGrantList",
	}
}

func newTestEnvoyGatewaySidecarPattern(t *testing.T, client dynamic.Interface, logger log.Component, recorder *record.FakeRecorder) appsecconfig.SidecarInjectionPattern {
	t.Helper()

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Mode: appsecconfig.InjectionModeSidecar,
			Sidecar: appsecconfig.Sidecar{
				UDSPath:   "/var/run/datadog/extproc.sock",
				RunAsUser: 65532,
			},
		},
		Injection: appsecconfig.Injection{
			CommonLabels:      map[string]string{"app": "datadog"},
			CommonAnnotations: map[string]string{"managed-by": "datadog"},
		},
	}

	pattern, ok := New(client, logger, config, recorder).(appsecconfig.SidecarInjectionPattern)
	require.True(t, ok)
	return pattern
}

func newEnvoyGatewayDataPlanePod(name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: envoyGatewaySystemNamespace,
			Labels: map[string]string{
				owningGatewayNameLabel:      "eg",
				owningGatewayNamespaceLabel: "eg-ns",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: envoyProxyContainerName}},
		},
	}
}

func findContainer(pod *corev1.Pod, name string) *corev1.Container {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == name {
			return &pod.Spec.Containers[i]
		}
	}
	return nil
}

func countContainers(pod *corev1.Pod, name string) int {
	count := 0
	for _, container := range pod.Spec.Containers {
		if container.Name == name {
			count++
		}
	}
	return count
}

func countVolumes(pod *corev1.Pod, name string) int {
	count := 0
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == name {
			count++
		}
	}
	return count
}

func TestEnvoyGatewaySidecarMutatePodHappyPath(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewaySidecarListKinds())
	recorder := record.NewFakeRecorder(100)
	pattern := newTestEnvoyGatewaySidecarPattern(t, client, logger, recorder)
	pod := newEnvoyGatewayDataPlanePod("envoy-eg")

	require.True(t, pattern.IsPodEligible(pod, envoyGatewaySystemNamespace))
	outcome, err := pattern.MutatePod(pod, envoyGatewaySystemNamespace, client)
	require.NoError(t, err)
	require.Equal(t, appsecconfig.MutationMutated, outcome)

	assert.Equal(t, 1, countVolumes(pod, sidecar.SharedSocketVolumeName))
	assert.NotNil(t, pod.Spec.Volumes[0].EmptyDir)

	envoy := findContainer(pod, envoyProxyContainerName)
	require.NotNil(t, envoy)
	require.Len(t, envoy.VolumeMounts, 1)
	assert.Equal(t, sidecar.SharedSocketVolumeName, envoy.VolumeMounts[0].Name)
	assert.Equal(t, "/var/run/datadog", envoy.VolumeMounts[0].MountPath)

	processor := findContainer(pod, sidecar.SidecarContainerName)
	require.NotNil(t, processor)
	assert.Contains(t, processor.Env, corev1.EnvVar{Name: "DD_SERVICE_EXTENSION_UDS_PATH", Value: "/var/run/datadog/extproc.sock"})
	require.NotNil(t, processor.SecurityContext)
	require.NotNil(t, processor.SecurityContext.RunAsUser)
	assert.EqualValues(t, 65532, *processor.SecurityContext.RunAsUser)
	require.NotNil(t, pod.Spec.SecurityContext)
	require.NotNil(t, pod.Spec.SecurityContext.FSGroup)
	assert.EqualValues(t, 65532, *pod.Spec.SecurityContext.FSGroup)

	backend, err := client.Resource(backendGVR).Namespace("eg-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, extProcName, backend.GetName())

	policy, err := client.Resource(extensionGVR).Namespace("eg-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.NoError(t, err)
	extProcs, found, err := unstructured.NestedSlice(policy.Object, "spec", "extProc")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, extProcs, 1)
	extProc, ok := extProcs[0].(map[string]any)
	require.True(t, ok)
	backendRefs, ok := extProc["backendRefs"].([]any)
	require.True(t, ok)
	require.Len(t, backendRefs, 1)
	backendRef, ok := backendRefs[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "gateway.envoyproxy.io", backendRef["group"])
	assert.Equal(t, "Backend", backendRef["kind"])
	assert.Equal(t, extProcName, backendRef["name"])

	grants, err := client.Resource(referenceGrantGVR).Namespace("datadog").List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, grants.Items)
}

func TestEnvoyGatewaySidecarAddedIsNoOp(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewaySidecarListKinds())
	recorder := record.NewFakeRecorder(100)
	pattern := newTestEnvoyGatewaySidecarPattern(t, client, logger, recorder)

	require.NoError(t, pattern.Added(ctx, newTestGateway("eg-ns", "eg")))

	_, err := client.Resource(backendGVR).Namespace("eg-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.Error(t, err)
	_, err = client.Resource(extensionGVR).Namespace("eg-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.Error(t, err)
}

func TestEnvoyGatewaySidecarMutatePodRecreatesBackendWhenPolicyExists(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	existingPolicy := newTestEnvoyExtensionPolicy("eg-ns")
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewaySidecarListKinds(), existingPolicy)
	recorder := record.NewFakeRecorder(100)
	pattern := newTestEnvoyGatewaySidecarPattern(t, client, logger, recorder)
	pod := newEnvoyGatewayDataPlanePod("envoy-eg")

	outcome, err := pattern.MutatePod(pod, envoyGatewaySystemNamespace, client)
	require.NoError(t, err)
	require.Equal(t, appsecconfig.MutationMutated, outcome)

	backend, err := client.Resource(backendGVR).Namespace("eg-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, extProcName, backend.GetName())
}

func TestEnvoyGatewaySidecarMutatePodIsIdempotent(t *testing.T) {
	logger := logmock.New(t)
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewaySidecarListKinds())
	recorder := record.NewFakeRecorder(100)
	pattern := newTestEnvoyGatewaySidecarPattern(t, client, logger, recorder)
	pod := newEnvoyGatewayDataPlanePod("envoy-eg")

	require.True(t, pattern.IsPodEligible(pod, envoyGatewaySystemNamespace))
	outcome, err := pattern.MutatePod(pod, envoyGatewaySystemNamespace, client)
	require.NoError(t, err)
	require.Equal(t, appsecconfig.MutationMutated, outcome)

	assert.True(t, pattern.IsPodEligible(pod, envoyGatewaySystemNamespace))
	outcome, err = pattern.MutatePod(pod, envoyGatewaySystemNamespace, client)
	require.Error(t, err)
	assert.Equal(t, appsecconfig.MutationSkipped, outcome)
	var skipped *appsecconfig.MutationSkippedReason
	require.True(t, errors.As(err, &skipped))
	assert.Equal(t, appsecconfig.SkipReasonAlreadySidecar, skipped.Reason)
	assert.Equal(t, 1, countContainers(pod, sidecar.SidecarContainerName))
	assert.Equal(t, 1, countVolumes(pod, sidecar.SharedSocketVolumeName))
}

func TestEnvoyGatewaySidecarMutatePodFailOpenWhenEnvoyContainerMissing(t *testing.T) {
	logger := logmock.New(t)
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewaySidecarListKinds())
	recorder := record.NewFakeRecorder(100)
	pattern := newTestEnvoyGatewaySidecarPattern(t, client, logger, recorder)
	pod := newEnvoyGatewayDataPlanePod("envoy-eg")
	pod.Spec.Containers = []corev1.Container{{Name: "shutdown-manager"}}

	assert.False(t, pattern.IsPodEligible(pod, envoyGatewaySystemNamespace))
	outcome, err := pattern.MutatePod(pod, envoyGatewaySystemNamespace, client)
	require.Error(t, err)
	assert.Equal(t, appsecconfig.MutationError, outcome)
	assert.Contains(t, err.Error(), "failed to mount appsec socket into envoy container")
	assert.Nil(t, findContainer(pod, sidecar.SidecarContainerName))

	event := ""
	for i := 0; i < 3; i++ {
		select {
		case evt := <-recorder.Events:
			if strings.Contains(evt, EventReasonSidecarInjectionSkipped) {
				event = evt
			}
		case <-time.After(time.Second):
		}
		if event != "" {
			break
		}
	}
	assert.True(t, strings.Contains(event, corev1.EventTypeWarning))
	assert.True(t, strings.Contains(event, EventReasonSidecarInjectionSkipped))
	assert.True(t, strings.Contains(event, "envoy container not found"))
}

func TestEnvoyGatewaySidecarIsPodEligibleRequiresOwnership(t *testing.T) {
	logger := logmock.New(t)
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewaySidecarListKinds())
	recorder := record.NewFakeRecorder(100)
	pattern := newTestEnvoyGatewaySidecarPattern(t, client, logger, recorder)

	tests := []struct {
		name string
		pod  func() *corev1.Pod
		ns   string
		want bool
	}{
		{
			name: "false when missing owning gateway name label",
			pod: func() *corev1.Pod {
				pod := newEnvoyGatewayDataPlanePod("envoy-eg")
				delete(pod.Labels, owningGatewayNameLabel)
				return pod
			},
			ns: envoyGatewaySystemNamespace,
		},
		{
			name: "false when missing owning gateway namespace label",
			pod: func() *corev1.Pod {
				pod := newEnvoyGatewayDataPlanePod("envoy-eg")
				delete(pod.Labels, owningGatewayNamespaceLabel)
				return pod
			},
			ns: envoyGatewaySystemNamespace,
		},
		{
			name: "false when envoy container is absent",
			pod: func() *corev1.Pod {
				pod := newEnvoyGatewayDataPlanePod("envoy-eg")
				pod.Spec.Containers = []corev1.Container{{Name: "shutdown-manager"}}
				return pod
			},
			ns: envoyGatewaySystemNamespace,
		},
		{
			name: "false when namespace differs from envoy gateway namespace",
			pod:  func() *corev1.Pod { return newEnvoyGatewayDataPlanePod("envoy-eg") },
			ns:   "custom-eg",
		},
		{
			name: "true when pod is correctly owned",
			pod:  func() *corev1.Pod { return newEnvoyGatewayDataPlanePod("envoy-eg") },
			ns:   envoyGatewaySystemNamespace,
			want: true,
		},
		{
			name: "true when correctly owned pod already has sidecar",
			pod: func() *corev1.Pod {
				pod := newEnvoyGatewayDataPlanePod("envoy-eg")
				pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: sidecar.SidecarContainerName})
				return pod
			},
			ns:   envoyGatewaySystemNamespace,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, pattern.IsPodEligible(tt.pod(), tt.ns))
		})
	}
}

func TestEnvoyGatewayNewModeSelection(t *testing.T) {
	logger := logmock.New(t)
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewaySidecarListKinds())
	recorder := record.NewFakeRecorder(100)

	sidecarConfig := appsecconfig.Config{Product: appsecconfig.Product{Mode: appsecconfig.InjectionModeSidecar}}
	_, ok := New(client, logger, sidecarConfig, recorder).(appsecconfig.SidecarInjectionPattern)
	assert.True(t, ok)

	externalConfig := appsecconfig.Config{Product: appsecconfig.Product{Mode: appsecconfig.InjectionModeExternal}}
	_, ok = New(client, logger, externalConfig, recorder).(appsecconfig.SidecarInjectionPattern)
	assert.False(t, ok)
}

func TestEnvoyGatewaySidecarMutatePodHonorsGatewayOptOut(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewaySidecarListKinds())
	gw := newTestGateway("eg-ns", "eg")
	gw.SetLabels(map[string]string{appsecEnabledLabel: "false"})
	_, err := client.Resource(gatewayGVR).Namespace("eg-ns").Create(ctx, gw, metav1.CreateOptions{})
	require.NoError(t, err)
	recorder := record.NewFakeRecorder(100)
	pattern := newTestEnvoyGatewaySidecarPattern(t, client, logger, recorder)
	pod := newEnvoyGatewayDataPlanePod("envoy-eg")

	outcome, err := pattern.MutatePod(pod, envoyGatewaySystemNamespace, client)
	require.Error(t, err)
	assert.Equal(t, appsecconfig.MutationSkipped, outcome)
	var skipped *appsecconfig.MutationSkippedReason
	require.True(t, errors.As(err, &skipped))
	assert.Equal(t, appsecconfig.SkipReasonGatewayOptOut, skipped.Reason)
	assert.Nil(t, findContainer(pod, sidecar.SidecarContainerName))
	assert.Equal(t, 0, countVolumes(pod, sidecar.SharedSocketVolumeName))

	_, err = client.Resource(backendGVR).Namespace("eg-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.Error(t, err)
	_, err = client.Resource(extensionGVR).Namespace("eg-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.Error(t, err)
}

func TestEnvoyGatewaySidecarMutatePodInjectsWhenGatewayOptedIn(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewaySidecarListKinds())
	gw := newTestGateway("eg-ns", "eg")
	gw.SetLabels(map[string]string{appsecEnabledLabel: "true"})
	_, err := client.Resource(gatewayGVR).Namespace("eg-ns").Create(ctx, gw, metav1.CreateOptions{})
	require.NoError(t, err)
	recorder := record.NewFakeRecorder(100)
	pattern := newTestEnvoyGatewaySidecarPattern(t, client, logger, recorder)
	pod := newEnvoyGatewayDataPlanePod("envoy-eg")

	outcome, err := pattern.MutatePod(pod, envoyGatewaySystemNamespace, client)
	require.NoError(t, err)
	require.Equal(t, appsecconfig.MutationMutated, outcome)
	assert.NotNil(t, findContainer(pod, sidecar.SidecarContainerName))

	_, err = client.Resource(backendGVR).Namespace("eg-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.NoError(t, err)
}

func TestEnvoyGatewaySidecarMutatePodFailsOpenOnEmptyUDSPath(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewaySidecarListKinds())
	recorder := record.NewFakeRecorder(100)

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Mode: appsecconfig.InjectionModeSidecar,
			Sidecar: appsecconfig.Sidecar{
				UDSPath:   "",
				RunAsUser: 65532,
			},
		},
		Injection: appsecconfig.Injection{
			CommonLabels:      map[string]string{"app": "datadog"},
			CommonAnnotations: map[string]string{"managed-by": "datadog"},
		},
	}
	pattern, ok := New(client, logger, config, recorder).(appsecconfig.SidecarInjectionPattern)
	require.True(t, ok)
	pod := newEnvoyGatewayDataPlanePod("envoy-eg")

	outcome, err := pattern.MutatePod(pod, envoyGatewaySystemNamespace, client)
	require.Error(t, err)
	assert.Equal(t, appsecconfig.MutationSkipped, outcome)
	var skipped *appsecconfig.MutationSkippedReason
	require.True(t, errors.As(err, &skipped))
	assert.Equal(t, appsecconfig.SkipReasonMissingUDSPath, skipped.Reason)
	assert.Nil(t, findContainer(pod, sidecar.SidecarContainerName))
	assert.Equal(t, 0, countVolumes(pod, sidecar.SharedSocketVolumeName))

	_, err = client.Resource(backendGVR).Namespace("eg-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.Error(t, err)
}

func TestEnvoyGatewaySidecarIsPodEligibleHonorsConfiguredNamespace(t *testing.T) {
	logger := logmock.New(t)
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewaySidecarListKinds())
	recorder := record.NewFakeRecorder(100)

	def := newTestEnvoyGatewaySidecarPattern(t, client, logger, recorder)
	pod := newEnvoyGatewayDataPlanePod("envoy-eg")
	assert.True(t, def.IsPodEligible(pod, envoyGatewaySystemNamespace), "default config must accept envoy-gateway-system")
	assert.False(t, def.IsPodEligible(pod, "custom-eg"))

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Mode:    appsecconfig.InjectionModeSidecar,
			Sidecar: appsecconfig.Sidecar{UDSPath: "/var/run/datadog/extproc.sock", RunAsUser: 65532},
		},
		Injection: appsecconfig.Injection{EnvoyGatewayNamespace: "custom-eg"},
	}
	custom, ok := New(client, logger, config, recorder).(appsecconfig.SidecarInjectionPattern)
	require.True(t, ok)
	assert.True(t, custom.IsPodEligible(pod, "custom-eg"), "configured namespace must be eligible")
	assert.False(t, custom.IsPodEligible(pod, envoyGatewaySystemNamespace), "non-configured namespace must be rejected")
}

func TestEnvoyGatewaySidecarMutatePodSkipsAlreadySidecarOwnedPod(t *testing.T) {
	logger := logmock.New(t)
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewaySidecarListKinds())
	recorder := record.NewFakeRecorder(100)
	pattern := newTestEnvoyGatewaySidecarPattern(t, client, logger, recorder)
	pod := newEnvoyGatewayDataPlanePod("envoy-eg")
	pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: sidecar.SidecarContainerName})

	require.True(t, pattern.IsPodEligible(pod, envoyGatewaySystemNamespace))
	outcome, err := pattern.MutatePod(pod, envoyGatewaySystemNamespace, client)

	require.Error(t, err)
	assert.Equal(t, appsecconfig.MutationSkipped, outcome)
	var skipped *appsecconfig.MutationSkippedReason
	require.True(t, errors.As(err, &skipped))
	assert.Equal(t, appsecconfig.SkipReasonAlreadySidecar, skipped.Reason)
}

func TestEnvoyGatewaySidecarPodDeletedReturnsMutatedNoop(t *testing.T) {
	logger := logmock.New(t)
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewaySidecarListKinds())
	recorder := record.NewFakeRecorder(100)
	pattern := newTestEnvoyGatewaySidecarPattern(t, client, logger, recorder)

	outcome, err := pattern.PodDeleted(newEnvoyGatewayDataPlanePod("envoy-eg"), envoyGatewaySystemNamespace, client)

	require.NoError(t, err)
	assert.Equal(t, appsecconfig.MutationMutated, outcome)
}

func TestEnvoyGatewaySidecarBackendCheckUsesControllerNamespace(t *testing.T) {
	logger := logmock.New(t)
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewaySidecarListKinds())
	recorder := record.NewFakeRecorder(100)

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Mode:    appsecconfig.InjectionModeSidecar,
			Sidecar: appsecconfig.Sidecar{UDSPath: "/var/run/datadog/extproc.sock", RunAsUser: 65532},
		},
		Injection: appsecconfig.Injection{
			EnvoyGatewayNamespace:           "custom-dataplane",
			EnvoyGatewayControllerNamespace: "custom-control",
			CommonLabels:                    map[string]string{"app": "datadog"},
			CommonAnnotations:               map[string]string{"managed-by": "datadog"},
		},
	}
	seedConfigMap(t, client, "custom-control", "extensionApis:\n  enableBackend: true\n")

	pattern, ok := New(client, logger, config, recorder).(appsecconfig.SidecarInjectionPattern)
	require.True(t, ok)
	pod := newEnvoyGatewayDataPlanePod("envoy-eg")

	outcome, err := pattern.MutatePod(pod, envoyGatewaySystemNamespace, client)
	require.NoError(t, err)
	require.Equal(t, appsecconfig.MutationMutated, outcome)

	for draining := true; draining; {
		select {
		case evt := <-recorder.Events:
			assert.NotContains(t, evt, EventReasonBackendExtensionDisabled,
				"backend-disabled warning must not fire: controller-namespace ConfigMap has enableBackend=true")
		case <-time.After(200 * time.Millisecond):
			draining = false
		}
	}
}
