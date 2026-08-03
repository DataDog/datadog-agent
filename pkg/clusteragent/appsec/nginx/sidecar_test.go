// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package nginx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/record"
)

func newTestNginxSidecarPattern(t *testing.T) (*nginxSidecarPattern, *dynamicfake.FakeDynamicClient) {
	t.Helper()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			configMapGVR:    "ConfigMapList",
			ingressClassGVR: "IngressClassList",
		},
	)

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Nginx: appsecconfig.Nginx{
				InitImage:       "datadog/ingress-nginx-injection",
				ModuleMountPath: "/modules_mount",
				InitRunAsUser:   101,
				InitRunAsGroup:  82,
			},
		},
		Injection: appsecconfig.Injection{
			CommonAnnotations: map[string]string{},
		},
	}

	base := &nginxInjectionPattern{
		client: client,
		logger: logger,
		config: config,
		eventRecorder: eventRecorder{
			recorder: record.NewFakeRecorder(100),
		},
	}

	return &nginxSidecarPattern{nginxInjectionPattern: base}, client
}

func newControllerPod(name, namespace, image string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "ingress-nginx",
				"app.kubernetes.io/component": "controller",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "controller",
					Image: image,
					Args: []string{
						"/nginx-ingress-controller",
						"--configmap=$(POD_NAMESPACE)/ingress-nginx-controller",
						"--election-id=ingress-nginx-leader",
					},
				},
			},
		},
	}
}

func TestIsPodEligible(t *testing.T) {
	pattern, _ := newTestNginxSidecarPattern(t)

	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name:     "labeled controller pod is eligible",
			pod:      newControllerPod("test", "ingress-nginx", "registry.k8s.io/ingress-nginx/controller:v1.15.1"),
			expected: true,
		},
		{
			name: "controller-class arg is eligible without standard labels",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Args: []string{"--controller-class=k8s.io/ingress-nginx"},
					}},
				},
			},
			expected: true,
		},
		{
			name: "wrong name label",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":      "traefik",
						"app.kubernetes.io/component": "controller",
					},
				},
			},
			expected: false,
		},
		{
			name: "missing component label",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name": "ingress-nginx",
					},
				},
			},
			expected: false,
		},
		{
			name: "already injected controller pod is still eligible",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":      "ingress-nginx",
						"app.kubernetes.io/component": "controller",
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: initContainerName},
					},
				},
			},
			expected: true,
		},
		{
			name: "neither labels nor controller-class arg is ineligible",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Args: []string{"--controller-class=example.com/not-ingress-nginx"},
					}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, pattern.IsPodEligible(tt.pod, tt.pod.Namespace))
		})
	}
}

func TestMutatePod(t *testing.T) {
	ctx := t.Context()
	pattern, client := newTestNginxSidecarPattern(t)

	// Create original ConfigMap in the fake client
	originalCM := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "ingress-nginx-controller",
				"namespace": "ingress-nginx",
			},
		},
	}
	_, err := client.Resource(configMapGVR).Namespace("ingress-nginx").Create(ctx, originalCM, metav1.CreateOptions{})
	require.NoError(t, err)

	pod := newControllerPod("test-pod", "ingress-nginx", "registry.k8s.io/ingress-nginx/controller:v1.15.1@sha256:abc123")

	outcome, err := pattern.MutatePod(pod, "ingress-nginx", client)
	require.NoError(t, err)
	assert.Equal(t, appsecconfig.MutationMutated, outcome)

	// Verify init container was added
	require.Len(t, pod.Spec.InitContainers, 1)
	ic := pod.Spec.InitContainers[0]
	assert.Equal(t, initContainerName, ic.Name)
	assert.Equal(t, "datadog/ingress-nginx-injection:v1.15.1", ic.Image)
	assert.Equal(t, []string{"/bin/sh", "/datadog/init_module.sh", "/modules_mount"}, ic.Command)

	// The injection image runs as root, so RunAsNonRoot must be paired with an
	// explicit non-root UID/GID or the kubelet rejects the container with
	// "container has runAsNonRoot and image will run as root".
	require.NotNil(t, ic.SecurityContext)
	require.NotNil(t, ic.SecurityContext.RunAsNonRoot)
	assert.True(t, *ic.SecurityContext.RunAsNonRoot)
	require.NotNil(t, ic.SecurityContext.RunAsUser, "RunAsUser must be set so kubelet accepts a root image under RunAsNonRoot")
	assert.NotZero(t, *ic.SecurityContext.RunAsUser, "RunAsUser must be non-zero (non-root)")
	require.NotNil(t, ic.SecurityContext.RunAsGroup)
	assert.NotZero(t, *ic.SecurityContext.RunAsGroup)

	// Verify volume was added
	require.Len(t, pod.Spec.Volumes, 1)
	assert.Equal(t, moduleVolumeName, pod.Spec.Volumes[0].Name)
	assert.NotNil(t, pod.Spec.Volumes[0].EmptyDir)

	// Verify volume mount was added to controller container
	controller := pod.Spec.Containers[0]
	require.Len(t, controller.VolumeMounts, 1)
	assert.Equal(t, moduleVolumeName, controller.VolumeMounts[0].Name)
	assert.Equal(t, "/modules_mount", controller.VolumeMounts[0].MountPath)

	// Verify --configmap was redirected
	assert.Equal(t, "--configmap=ingress-nginx/datadog-appsec-ingress-nginx-controller", controller.Args[1])

	// Verify DD ConfigMap was created
	ddCM, err := client.Resource(configMapGVR).Namespace("ingress-nginx").Get(ctx, "datadog-appsec-ingress-nginx-controller", metav1.GetOptions{})
	require.NoError(t, err)
	data, found, err := unstructured.NestedStringMap(ddCM.UnstructuredContent(), "data")
	require.NoError(t, err)
	require.True(t, found, "DD ConfigMap should have data field")
	assert.Contains(t, data[mainSnippetKey], "load_module /modules_mount/ngx_http_datadog_module.so;")

	// Verify DD ConfigMap has proxy-type label for label-based cleanup
	ddLabels := ddCM.GetLabels()
	assert.Equal(t, string(appsecconfig.ProxyTypeIngressNginx), ddLabels[appsecconfig.AppsecProcessorProxyTypeAnnotation],
		"DD ConfigMap must have proxy-type label so it can be cleaned up by label selector")
}

func TestFindControllerConfigMapArg(t *testing.T) {
	tests := []struct {
		name         string
		pod          *corev1.Pod
		podNamespace string
		wantNS       string
		wantName     string
		wantFound    bool
		wantErr      error
	}{
		{
			name:         "standard $(POD_NAMESPACE) form is accepted",
			pod:          newControllerPod("test", "ingress-nginx", "img:v1"),
			podNamespace: "ingress-nginx",
			wantNS:       "ingress-nginx",
			wantName:     "ingress-nginx-controller",
			wantFound:    true,
		},
		{
			name: "hardcoded same namespace is accepted",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Args: []string{"--configmap=ingress-nginx/my-config"},
					}},
				},
			},
			podNamespace: "ingress-nginx",
			wantNS:       "ingress-nginx",
			wantName:     "my-config",
			wantFound:    true,
		},
		{
			name: "hardcoded foreign namespace is rejected (confused-deputy guard)",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Args: []string{"--configmap=custom-ns/my-config"},
					}},
				},
			},
			podNamespace: "ingress-nginx",
			wantErr:      errCrossNamespaceConfigMap,
		},
		{
			name: "kube-system reference is rejected",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Args: []string{"--configmap=kube-system/coredns"},
					}},
				},
			},
			podNamespace: "attacker-ns",
			wantErr:      errCrossNamespaceConfigMap,
		},
		{
			name: "leading slash with empty namespace is rejected",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Args: []string{"--configmap=/foo"},
					}},
				},
			},
			podNamespace: "ingress-nginx",
			wantErr:      errCrossNamespaceConfigMap,
		},
		{
			name: "trailing slash with empty name is rejected",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Args: []string{"--configmap=ingress-nginx/"},
					}},
				},
			},
			podNamespace: "ingress-nginx",
			wantErr:      errEmptyConfigMapName,
		},
		{
			name: "no slash skips the arg and falls through to not-found",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Args: []string{"--configmap=foo"},
					}},
				},
			},
			podNamespace: "ingress-nginx",
			wantFound:    false,
		},
		{
			name: "no configmap arg falls back to defaults",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Args: []string{"--election-id=test"},
					}},
				},
			},
			podNamespace: "ingress-nginx",
			wantFound:    false,
		},
		{
			name: "multi-container first match wins - malicious arg rejected even if later container is benign",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Args: []string{"--configmap=kube-system/coredns"}},
						{Args: []string{"--configmap=ingress-nginx/legit"}},
					},
				},
			},
			podNamespace: "ingress-nginx",
			wantErr:      errCrossNamespaceConfigMap,
		},
		{
			name: "multi-container second container holds the arg",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Args: []string{"--election-id=x"}},
						{Args: []string{"--configmap=ingress-nginx/legit"}},
					},
				},
			},
			podNamespace: "ingress-nginx",
			wantNS:       "ingress-nginx",
			wantName:     "legit",
			wantFound:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, ns, name, found, err := findControllerConfigMapArg(tt.pod, tt.podNamespace)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.False(t, found)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFound, found)
			if tt.wantFound {
				assert.Equal(t, tt.wantNS, ns)
				assert.Equal(t, tt.wantName, name)
			}
		})
	}
}

func TestParseControllerVersion(t *testing.T) {
	tests := []struct {
		name    string
		image   string
		want    string
		wantErr bool
	}{
		{
			name:  "full image with digest",
			image: "registry.k8s.io/ingress-nginx/controller:v1.15.1@sha256:594ceea76b01c592858f803f9ff4d2cb40542cae2060410b2c95f75907d659e1",
			want:  "v1.15.1",
		},
		{
			name:  "image with tag only",
			image: "registry.k8s.io/ingress-nginx/controller:v1.10.0",
			want:  "v1.10.0",
		},
		{
			name:  "custom registry",
			image: "my-registry.example.com/ingress-nginx/controller:v1.12.3",
			want:  "v1.12.3",
		},
		{
			name:    "latest tag",
			image:   "registry.k8s.io/ingress-nginx/controller:latest",
			wantErr: true,
		},
		{
			name:    "no tag",
			image:   "registry.k8s.io/ingress-nginx/controller",
			wantErr: true,
		},
		{
			name:    "digest only",
			image:   "registry.k8s.io/ingress-nginx/controller@sha256:abc123",
			wantErr: true,
		},
		{
			name:    "registry with port and no tag",
			image:   "myregistry.com:5000/ingress-nginx/controller",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := parseControllerVersion(tt.image)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, version)
			}
		})
	}
}

func TestMatchCondition(t *testing.T) {
	pattern, _ := newTestNginxSidecarPattern(t)
	mc := pattern.MatchCondition()

	// Verify CEL expression checks label existence before access
	assert.Contains(t, mc.Expression, "'app.kubernetes.io/name' in object.metadata.labels")
	assert.Contains(t, mc.Expression, "'app.kubernetes.io/component' in object.metadata.labels")
}

func TestMode(t *testing.T) {
	pattern, _ := newTestNginxSidecarPattern(t)
	// nginx always returns sidecar mode regardless of config
	assert.Equal(t, appsecconfig.InjectionModeSidecar, pattern.Mode())
}

func TestDeletedCleansUpDDConfigMaps(t *testing.T) {
	ctx := t.Context()
	pattern, client := newTestNginxSidecarPattern(t)

	// Pre-create a DD ConfigMap with our proxy-type label (simulating what MutatePod creates)
	ddCM := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "datadog-appsec-ingress-nginx-controller",
				"namespace": "ingress-nginx",
				"labels": map[string]interface{}{
					"app.kubernetes.io/component":                   "datadog-appsec-injector",
					"app.kubernetes.io/part-of":                     "datadog",
					"app.kubernetes.io/managed-by":                  "datadog-cluster-agent",
					appsecconfig.AppsecProcessorProxyTypeAnnotation: string(appsecconfig.ProxyTypeIngressNginx),
				},
			},
			"data": map[string]interface{}{
				"main-snippet": "load_module /modules_mount/ngx_http_datadog_module.so;",
			},
		},
	}
	_, err := client.Resource(configMapGVR).Namespace("ingress-nginx").Create(ctx, ddCM, metav1.CreateOptions{})
	require.NoError(t, err)

	// Simulate IngressClass deletion
	ingressClass := newIngressClass("nginx", "k8s.io/ingress-nginx")
	err = pattern.Deleted(ctx, ingressClass)
	require.NoError(t, err)

	// Verify DD ConfigMap was deleted
	_, err = client.Resource(configMapGVR).Namespace("ingress-nginx").Get(ctx, "datadog-appsec-ingress-nginx-controller", metav1.GetOptions{})
	assert.True(t, k8serrors.IsNotFound(err), "DD ConfigMap should be deleted after IngressClass deletion")
}

func TestDeletedSkipsWhenOtherIngressClassesExist(t *testing.T) {
	ctx := t.Context()
	pattern, client := newTestNginxSidecarPattern(t)

	// Pre-create a DD ConfigMap
	ddCM := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "datadog-appsec-ingress-nginx-controller",
				"namespace": "ingress-nginx",
				"labels": map[string]interface{}{
					"app.kubernetes.io/component":                   "datadog-appsec-injector",
					"app.kubernetes.io/part-of":                     "datadog",
					"app.kubernetes.io/managed-by":                  "datadog-cluster-agent",
					appsecconfig.AppsecProcessorProxyTypeAnnotation: string(appsecconfig.ProxyTypeIngressNginx),
				},
			},
		},
	}
	_, err := client.Resource(configMapGVR).Namespace("ingress-nginx").Create(ctx, ddCM, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create two ingress-nginx IngressClasses
	ic1 := newIngressClass("nginx", "k8s.io/ingress-nginx")
	ic2 := newIngressClass("nginx-internal", "k8s.io/ingress-nginx")
	_, err = client.Resource(ingressClassGVR).Create(ctx, ic1, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = client.Resource(ingressClassGVR).Create(ctx, ic2, metav1.CreateOptions{})
	require.NoError(t, err)

	// Delete one IngressClass — but the other still exists
	err = pattern.Deleted(ctx, ic1)
	require.NoError(t, err)

	// DD ConfigMap should NOT be deleted because another ingress-nginx IngressClass still exists
	_, err = client.Resource(configMapGVR).Namespace("ingress-nginx").Get(ctx, "datadog-appsec-ingress-nginx-controller", metav1.GetOptions{})
	assert.NoError(t, err, "DD ConfigMap should NOT be deleted when other ingress-nginx IngressClasses still exist")
}

func TestMutatePodInitImageResolution(t *testing.T) {
	tests := []struct {
		name            string
		initImage       string
		controllerImage string
		wantErr         string
		wantInitImage   string
	}{
		{
			name:            "no tag derives version from controller",
			initImage:       "datadog/ingress-nginx-injection",
			controllerImage: "registry.k8s.io/ingress-nginx/controller:v1.15.1",
			wantInitImage:   "datadog/ingress-nginx-injection:v1.15.1",
		},
		{
			name:            "no tag with unparseable controller version",
			initImage:       "datadog/ingress-nginx-injection",
			controllerImage: "registry.k8s.io/ingress-nginx/controller:latest",
			wantErr:         "manual extraModules",
		},
		{
			name:            "tagged init image used as-is",
			initImage:       "myregistry.com/datadog/ingress-nginx-injection:custom-v1.2.3",
			controllerImage: "registry.k8s.io/ingress-nginx/controller:v1.15.1",
			wantInitImage:   "myregistry.com/datadog/ingress-nginx-injection:custom-v1.2.3",
		},
		{
			name:            "tagged init image with unparseable controller version",
			initImage:       "myregistry.com/datadog/ingress-nginx-injection:custom-v1.2.3",
			controllerImage: "registry.k8s.io/ingress-nginx/controller:latest",
			wantInitImage:   "myregistry.com/datadog/ingress-nginx-injection:custom-v1.2.3",
		},
		{
			name:            "digest-pinned init image used as-is",
			initImage:       "myregistry.com/datadog/ingress-nginx-injection@sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4",
			controllerImage: "registry.k8s.io/ingress-nginx/controller:v1.15.1",
			wantInitImage:   "myregistry.com/datadog/ingress-nginx-injection@sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4",
		},
		{
			name:            "invalid init image falls through to version derivation",
			initImage:       "!!!invalid",
			controllerImage: "registry.k8s.io/ingress-nginx/controller:v1.15.1",
			wantInitImage:   "!!!invalid:v1.15.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			pattern, client := newTestNginxSidecarPatternWithConfig(t, appsecconfig.Nginx{
				InitImage:       tt.initImage,
				ModuleMountPath: "/modules_mount",
			})

			originalCM := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "ingress-nginx-controller",
						"namespace": "ingress-nginx",
					},
				},
			}
			_, err := client.Resource(configMapGVR).Namespace("ingress-nginx").Create(ctx, originalCM, metav1.CreateOptions{})
			require.NoError(t, err)

			pod := newControllerPod("test-pod", "ingress-nginx", tt.controllerImage)
			outcome, err := pattern.MutatePod(pod, "ingress-nginx", client)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				assert.Equal(t, appsecconfig.MutationError, outcome)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, appsecconfig.MutationMutated, outcome)
			require.Len(t, pod.Spec.InitContainers, 1)
			assert.Equal(t, tt.wantInitImage, pod.Spec.InitContainers[0].Image)
		})
	}
}

func newTestNginxSidecarPatternWithConfig(t *testing.T, nginx appsecconfig.Nginx) (*nginxSidecarPattern, *dynamicfake.FakeDynamicClient) {
	t.Helper()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			configMapGVR:    "ConfigMapList",
			ingressClassGVR: "IngressClassList",
		},
	)
	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Nginx: nginx,
		},
		Injection: appsecconfig.Injection{
			CommonAnnotations: map[string]string{},
		},
	}
	base := &nginxInjectionPattern{
		client: client,
		logger: logger,
		config: config,
		eventRecorder: eventRecorder{
			recorder: record.NewFakeRecorder(100),
		},
	}
	return &nginxSidecarPattern{nginxInjectionPattern: base}, client
}

// TestMutatePod_CrossNamespaceConfigMapRefused is the bisect anchor for the
// confused-deputy ConfigMap mitigation. It MUST fail against the unpatched
// code (which trusted the pod's --configmap arg verbatim) and pass against the
// patched code.
func TestMutatePod_CrossNamespaceConfigMapRefused(t *testing.T) {
	pattern, client := newTestNginxSidecarPattern(t)

	pod := newControllerPod("attacker", "attacker-ns", "registry.k8s.io/ingress-nginx/controller:v1.15.1")
	pod.Spec.Containers[0].Args = []string{
		"/nginx-ingress-controller",
		"--configmap=kube-system/coredns",
		"--election-id=ingress-nginx-leader",
	}

	outcome, err := pattern.MutatePod(pod, "attacker-ns", client)
	assert.Equal(t, appsecconfig.MutationSkipped, outcome, "MutatePod must skip mutation on cross-ns refs")
	var skip *appsecconfig.MutationSkippedReason
	require.ErrorAs(t, err, &skip)
	assert.Equal(t, appsecconfig.SkipReasonCrossNamespaceConfigMap, skip.Reason)

	assert.Empty(t, client.Actions(), "no API operations may occur on rejection")

	assert.Equal(t, "--configmap=kube-system/coredns", pod.Spec.Containers[0].Args[1],
		"pod arg must be unmodified")
	assert.Empty(t, pod.Spec.InitContainers, "no init container must be injected")
	assert.Empty(t, pod.Spec.Volumes, "no volume must be added")
	assert.Empty(t, pod.Spec.Containers[0].VolumeMounts, "no volume mount must be added")
}

func TestMutatePod_EmptyConfigMapNameRefused(t *testing.T) {
	pattern, client := newTestNginxSidecarPattern(t)

	pod := newControllerPod("test", "ingress-nginx", "registry.k8s.io/ingress-nginx/controller:v1.15.1")
	pod.Spec.Containers[0].Args = []string{
		"/nginx-ingress-controller",
		"--configmap=ingress-nginx/",
	}

	outcome, err := pattern.MutatePod(pod, "ingress-nginx", client)
	assert.Equal(t, appsecconfig.MutationSkipped, outcome)
	var skip *appsecconfig.MutationSkippedReason
	require.ErrorAs(t, err, &skip)
	assert.Equal(t, appsecconfig.SkipReasonInvalidConfigMapArg, skip.Reason)
	assert.Empty(t, pod.Spec.InitContainers)
	assert.Empty(t, pod.Spec.Volumes)
}

func TestMutatePod_returns_init_container_present_skip_when_already_injected(t *testing.T) {
	pattern, client := newTestNginxSidecarPattern(t)
	pod := newControllerPod("test", "ingress-nginx", "registry.k8s.io/ingress-nginx/controller:v1.15.1")
	pod.Spec.InitContainers = []corev1.Container{{Name: initContainerName}}

	outcome, err := pattern.MutatePod(pod, "ingress-nginx", client)

	assert.Equal(t, appsecconfig.MutationSkipped, outcome)
	var skip *appsecconfig.MutationSkippedReason
	require.ErrorAs(t, err, &skip)
	assert.Equal(t, appsecconfig.SkipReasonAlreadyInitSidecar, skip.Reason)
	assert.Empty(t, client.Actions(), "no API operations may occur when init container is already present")
}

func TestPodDeleted_returns_noop_when_nginx_pod_deleted(t *testing.T) {
	pattern, client := newTestNginxSidecarPattern(t)
	pod := newControllerPod("test", "ingress-nginx", "registry.k8s.io/ingress-nginx/controller:v1.15.1")

	outcome, err := pattern.PodDeleted(pod, "ingress-nginx", client)

	require.NoError(t, err)
	assert.Equal(t, appsecconfig.MutationMutated, outcome)
}

func TestBuildInitContainerRunAsConfig(t *testing.T) {
	tests := []struct {
		name           string
		runAsUser      int64
		runAsGroup     int64
		wantUserSet    bool
		wantUserValue  int64
		wantGroupSet   bool
		wantGroupValue int64
	}{
		{
			name:           "non-negative IDs are applied (root image needs explicit non-root UID)",
			runAsUser:      101,
			runAsGroup:     82,
			wantUserSet:    true,
			wantUserValue:  101,
			wantGroupSet:   true,
			wantGroupValue: 82,
		},
		{
			name:         "negative IDs leave the fields unset so a custom image's USER is honored",
			runAsUser:    -1,
			runAsGroup:   -1,
			wantUserSet:  false,
			wantGroupSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := buildInitContainer("repo/image:tag", "/modules_mount", tt.runAsUser, tt.runAsGroup)
			require.NotNil(t, c.SecurityContext)
			assert.True(t, *c.SecurityContext.RunAsNonRoot, "RunAsNonRoot must always be enforced")

			if tt.wantUserSet {
				require.NotNil(t, c.SecurityContext.RunAsUser)
				assert.Equal(t, tt.wantUserValue, *c.SecurityContext.RunAsUser)
			} else {
				assert.Nil(t, c.SecurityContext.RunAsUser)
			}

			if tt.wantGroupSet {
				require.NotNil(t, c.SecurityContext.RunAsGroup)
				assert.Equal(t, tt.wantGroupValue, *c.SecurityContext.RunAsGroup)
			} else {
				assert.Nil(t, c.SecurityContext.RunAsGroup)
			}
		})
	}
}
