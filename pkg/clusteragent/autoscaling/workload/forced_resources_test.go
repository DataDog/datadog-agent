// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	autoscalingstore "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/store"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// newForcedResourcesAutoscaler builds an autoscaler whose recommendation is 500m CPU and 256Mi
// memory for container1, with the given annotations and spec, and runs the source selection the
// controller runs on every sync.
func newForcedResourcesAutoscaler(annotations map[string]string, spec *datadoghq.DatadogPodAutoscalerSpec) model.PodAutoscalerInternal {
	if spec == nil {
		spec = &datadoghq.DatadogPodAutoscalerSpec{}
	}
	spec.TargetRef = autoscalingv2.CrossVersionObjectReference{
		Kind:       "Deployment",
		APIVersion: "apps/v1",
		Name:       "test-deployment",
	}

	pai := model.FakePodAutoscalerInternal{
		Namespace: "ns1",
		Name:      "autoscaler1",
		Spec:      spec,
		MainScalingValues: model.ScalingValues{
			Vertical: &model.VerticalScalingValues{
				Source:        datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp:     time.Now().Add(-time.Minute),
				ResourcesHash: "version1",
				ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{{
					Name:     "container1",
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi")},
				}},
			},
		},
	}.Build()
	pai.UpdateFromOpsAnnotations(annotations)

	horizontalSource, verticalSource := getActiveScalingSources(time.Now(), &pai)
	pai.SetActiveScalingValues(time.Now(), horizontalSource, verticalSource)
	return pai
}

func newForcedResourcesPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns1",
			Name:      "pod1",
			OwnerReferences: []metav1.OwnerReference{{
				Kind:       "ReplicaSet",
				Name:       "test-deployment-968f49d86",
				APIVersion: "apps/v1",
			}},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "container1",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
				},
			}},
		},
	}
}

func applyWithPatcher(t *testing.T, pai model.PodAutoscalerInternal) *corev1.Pod {
	t.Helper()
	s := autoscalingstore.NewStore[model.PodAutoscalerInternal]()
	item, _ := s.Get(pai.ID())
	item.Upsert(pai, "")

	pod := newForcedResourcesPod()
	_, err := NewPodPatcher(s, nil, nil).ApplyRecommendations(pod)
	require.NoError(t, err)
	return pod
}

// TestPatcherApplyForcedResources covers the admission webhook, which re-derives constraints on
// every replica: forced values reach new pods, bounded by the constraints like recommended values,
// unless pause or Preview suppress them.
func TestPatcherApplyForcedResources(t *testing.T) {
	constraints := &datadoghqcommon.DatadogPodAutoscalerConstraints{
		Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{{
			Name:       "container1",
			MaxAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("128Mi")},
		}},
	}
	preview := &datadoghq.DatadogPodAutoscalerApplyPolicy{Mode: datadoghq.DatadogPodAutoscalerApplyModePreview}
	burstable := &datadoghqcommon.DatadogPodAutoscalerOptions{Burstable: pointer.Ptr(true)}

	for _, tt := range []struct {
		name         string
		annotations  map[string]string
		spec         datadoghq.DatadogPodAutoscalerSpec
		expectedCPU  string // cpu request on the patched pod; the initial 100m/64Mi when suppressed
		expectedMem  string // memory request on the patched pod
		noCPULimit   bool
		expectAnnoID bool // the pod carries the active recommendation ID
	}{
		{
			name:        "forced and recommended values are both clamped",
			annotations: map[string]string{model.ForceResourcesAnnotationKey: `[{"name": "container1", "requests": {"cpu": "2"}}]`},
			spec:        datadoghq.DatadogPodAutoscalerSpec{Constraints: constraints},
			expectedCPU: "1", expectedMem: "128Mi", expectAnnoID: true,
		},
		{
			name:        "a forced value within the constraints is applied",
			annotations: map[string]string{model.ForceResourcesAnnotationKey: `[{"name": "container1", "requests": {"cpu": "800m"}}]`},
			spec:        datadoghq.DatadogPodAutoscalerSpec{Constraints: constraints},
			expectedCPU: "800m", expectedMem: "128Mi",
		},
		{
			name:        "burstable: a forced request is applied and the cpu limit stays removed",
			annotations: map[string]string{model.ForceResourcesAnnotationKey: `[{"name": "container1", "requests": {"cpu": "800m"}}]`},
			spec:        datadoghq.DatadogPodAutoscalerSpec{Constraints: constraints, Options: burstable},
			expectedCPU: "800m", expectedMem: "128Mi", noCPULimit: true,
		},
		{
			name:        "without the annotation the recommendation is clamped",
			spec:        datadoghq.DatadogPodAutoscalerSpec{Constraints: constraints},
			expectedCPU: "500m", expectedMem: "128Mi",
		},
		{
			name:        "Preview suppresses the override",
			annotations: map[string]string{model.ForceResourcesAnnotationKey: `[{"name": "container1", "requests": {"cpu": "2"}}]`},
			spec:        datadoghq.DatadogPodAutoscalerSpec{ApplyPolicy: preview},
			expectedCPU: "100m", expectedMem: "64Mi",
		},
		{
			name:        "pause suppresses the override",
			annotations: map[string]string{model.ForceResourcesAnnotationKey: `[{"name": "container1", "requests": {"cpu": "2"}}]`, model.PauseAnnotationKey: "true"},
			expectedCPU: "100m", expectedMem: "64Mi",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pai := newForcedResourcesAutoscaler(tt.annotations, tt.spec.DeepCopy())
			pod := applyWithPatcher(t, pai)

			resources := pod.Spec.Containers[0].Resources
			assert.Equal(t, tt.expectedCPU, resources.Requests.Cpu().String())
			assert.Equal(t, tt.expectedMem, resources.Requests.Memory().String())
			if tt.noCPULimit {
				assert.NotContains(t, resources.Limits, corev1.ResourceCPU, "burstable removes the cpu limit")
			}
			if tt.expectAnnoID {
				assert.Equal(t, pai.ScalingValues().Vertical.ResourcesHash, pod.Annotations[model.RecommendationIDAnnotation])
			}
		})
	}
}

// TestVerticalConstraintsForcedResources covers the vertical controller order: forced values are
// overlaid on the recommendation first, so the constraints clamp them like recommended values.
func TestVerticalConstraintsForcedResources(t *testing.T) {
	spec := &datadoghq.DatadogPodAutoscalerSpec{
		Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
			Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{{
				Name:       "container1",
				MaxAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("128Mi")},
			}},
		},
	}

	for _, tt := range []struct {
		name           string
		annotation     string
		expectedCPU    string
		expectedMemory string
	}{
		{name: "forced value above maxAllowed is clamped", annotation: `[{"name": "container1", "requests": {"cpu": "2"}}]`, expectedCPU: "1", expectedMemory: "128Mi"},
		{name: "forced value within the constraints is kept", annotation: `[{"name": "container1", "requests": {"cpu": "800m"}}]`, expectedCPU: "800m", expectedMemory: "128Mi"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pai := newForcedResourcesAutoscaler(map[string]string{model.ForceResourcesAnnotationKey: tt.annotation}, spec.DeepCopy())

			constrained := pai.ScalingValues().Vertical.DeepCopy()
			_, err := applyForcedResources(constrained, pai.ForcedResources())
			require.NoError(t, err)
			_, err = applyVerticalConstraints(constrained, pai.Spec().Constraints, pai.IsBurstable())
			require.NoError(t, err)

			container := constrained.ContainerResources[0]
			assert.Equal(t, tt.expectedCPU, container.Requests.Cpu().String())
			assert.Equal(t, tt.expectedMemory, container.Requests.Memory().String(), "a recommended value is clamped too")
		})
	}
}

// TestPatcherApplyForcedResourcesMultipleContainers covers an annotation that overrides several
// containers at once, each with different fields, next to a container it does not list and one
// that has no recommendation at all.
func TestPatcherApplyForcedResourcesMultipleContainers(t *testing.T) {
	pai := model.FakePodAutoscalerInternal{
		Namespace: "ns1",
		Name:      "autoscaler1",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", APIVersion: "apps/v1", Name: "test-deployment"},
			Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
				Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{{
					Name:       "*",
					MaxAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
				}},
			},
		},
		MainScalingValues: model.ScalingValues{
			Vertical: &model.VerticalScalingValues{
				Source:        datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp:     time.Now().Add(-time.Minute),
				ResourcesHash: "version1",
				ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
					{
						Name:     "app",
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi")},
					},
					{
						Name:     "sidecar",
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
						Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("128Mi")},
					},
					{
						Name:     "logger",
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m")},
					},
				},
			},
		},
	}.Build()
	pai.UpdateFromOpsAnnotations(map[string]string{model.ForceResourcesAnnotationKey: `[
		{"name": "app", "requests": {"cpu": "2"}},
		{"name": "sidecar", "limits": {"memory": "512Mi"}},
		{"name": "worker", "requests": {"cpu": "300m", "memory": "1Gi"}, "limits": {"cpu": "600m"}}
	]`})
	require.NotNil(t, pai.ForcedResources(), "a multi-container annotation must parse")
	require.Len(t, pai.ForcedResources(), 3)
	for i, name := range []string{"app", "sidecar", "worker"} {
		assert.Equal(t, name, pai.ForcedResources()[i].Name)
	}
	horizontalSource, verticalSource := getActiveScalingSources(time.Now(), &pai)
	pai.SetActiveScalingValues(time.Now(), horizontalSource, verticalSource)

	s := autoscalingstore.NewStore[model.PodAutoscalerInternal]()
	item, _ := s.Get(pai.ID())
	item.Upsert(pai, "")

	pod := newForcedResourcesPod()
	pod.Spec.Containers = []corev1.Container{{Name: "app"}, {Name: "sidecar"}, {Name: "logger"}, {Name: "worker"}}
	_, err := NewPodPatcher(s, nil, nil).ApplyRecommendations(pod)
	require.NoError(t, err)

	resources := map[string]*corev1.ResourceRequirements{}
	for i := range pod.Spec.Containers {
		resources[pod.Spec.Containers[i].Name] = &pod.Spec.Containers[i].Resources
	}

	assert.Equal(t, "1", resources["app"].Requests.Cpu().String(), "app: forced cpu request 2, clamped by maxAllowed")
	assert.Equal(t, "256Mi", resources["app"].Requests.Memory().String(), "app: memory keeps its recommendation")

	assert.Equal(t, "512Mi", resources["sidecar"].Limits.Memory().String(), "sidecar: forced memory limit")
	assert.Equal(t, "64Mi", resources["sidecar"].Requests.Memory().String(), "sidecar: memory request keeps its recommendation")
	assert.Equal(t, "100m", resources["sidecar"].Requests.Cpu().String(), "sidecar: cpu keeps its recommendation")

	assert.Equal(t, "50m", resources["logger"].Requests.Cpu().String(), "logger: unlisted, keeps its recommendation")

	assert.Equal(t, "300m", resources["worker"].Requests.Cpu().String(), "worker: forced without any recommendation")
	assert.Equal(t, "600m", resources["worker"].Limits.Cpu().String())
	assert.Equal(t, "1Gi", resources["worker"].Requests.Memory().String())
	_, hasMemoryLimit := resources["worker"].Limits[corev1.ResourceMemory]
	assert.False(t, hasMemoryLimit, "worker: a limit neither forced nor recommended is left untouched")
}

// flattenResources lists the requests and limits of the containers as "container/kind/resource": value,
// which compares quantities by their canonical string.
func flattenResources(values *model.VerticalScalingValues) map[string]string {
	flat := map[string]string{}
	for _, container := range values.ContainerResources {
		for name, quantity := range container.Requests {
			flat[container.Name+"/requests/"+string(name)] = quantity.String()
		}
		for name, quantity := range container.Limits {
			flat[container.Name+"/limits/"+string(name)] = quantity.String()
		}
	}
	return flat
}

func TestApplyForcedResources(t *testing.T) {
	recommendation := func() *model.VerticalScalingValues {
		return &model.VerticalScalingValues{
			Source:        datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
			ResourcesHash: "recommendation-hash",
			ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
				{
					Name:     "app",
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi")},
					Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("512Mi")},
				},
				{
					Name:     "sidecar",
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
				},
			},
		}
	}
	mustParse := func(t *testing.T, value string) []datadoghqcommon.DatadogPodAutoscalerContainerResources {
		pai := model.PodAutoscalerInternal{}
		pai.UpdateFromOpsAnnotations(map[string]string{model.ForceResourcesAnnotationKey: value})
		require.NotNil(t, pai.ForcedResources(), "valid annotation")
		return pai.ForcedResources()
	}
	apply := func(t *testing.T, forced []datadoghqcommon.DatadogPodAutoscalerContainerResources, values *model.VerticalScalingValues) {
		limitErr, err := applyForcedResources(values, forced)
		require.NoError(t, err)
		require.Error(t, limitErr, "a forced override is reported as a limit")
	}
	baseline := flattenResources(recommendation())

	for _, tt := range []struct {
		name       string
		annotation string
		// changes are applied on the recommendation to get the expected values.
		changes map[string]string
	}{
		{
			name:       "only the forced fields are overridden",
			annotation: `[{"name": "app", "requests": {"cpu": "750m"}}]`,
			changes:    map[string]string{"app/requests/cpu": "750m"},
		},
		{
			name:       "a container without recommendation gets only its forced values",
			annotation: `[{"name": "worker", "limits": {"memory": "1Gi"}}]`,
			changes:    map[string]string{"worker/limits/memory": "1Gi"},
		},
		{
			name:       "a forced request above the recommended limit raises the limit",
			annotation: `[{"name": "app", "requests": {"cpu": "2"}}]`,
			changes:    map[string]string{"app/requests/cpu": "2", "app/limits/cpu": "2"},
		},
		{
			name:       "a forced limit below the recommended request lowers the request",
			annotation: `[{"name": "app", "limits": {"memory": "128Mi"}}]`,
			changes:    map[string]string{"app/requests/memory": "128Mi", "app/limits/memory": "128Mi"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			values := recommendation()
			apply(t, mustParse(t, tt.annotation), values)

			expected := maps.Clone(baseline)
			maps.Copy(expected, tt.changes)
			assert.Equal(t, expected, flattenResources(values))
			assert.NotEqual(t, "recommendation-hash", values.ResourcesHash, "the hash reflects the forced values")
		})
	}

	t.Run("no recommendation at all", func(t *testing.T) {
		values := &model.VerticalScalingValues{}
		apply(t, mustParse(t, `[{"name": "app", "requests": {"cpu": "2"}, "limits": {"cpu": "4"}}]`), values)

		require.Len(t, values.ContainerResources, 1)
		assert.NotEmpty(t, values.ResourcesHash, "a hash is required for the vertical controller to act")
	})

	t.Run("the remove-limit sentinel is kept unless the limit is forced", func(t *testing.T) {
		values := recommendation()
		values.ContainerResources[0].Limits[corev1.ResourceCPU] = resource.MustParse("-1")
		apply(t, mustParse(t, `[{"name": "app", "requests": {"cpu": "2"}}]`), values)
		assert.True(t, values.ContainerResources[0].Limits[corev1.ResourceCPU].Equal(resource.MustParse("-1")), "burstable removes the limit")

		apply(t, mustParse(t, `[{"name": "app", "limits": {"cpu": "4"}}]`), values)
		assert.True(t, values.ContainerResources[0].Limits[corev1.ResourceCPU].Equal(resource.MustParse("4")), "a forced limit wins over burstable")
	})

	t.Run("applying twice is a no-op", func(t *testing.T) {
		forced := mustParse(t, `[{"name": "app", "requests": {"cpu": "2"}}, {"name": "worker", "limits": {"memory": "1Gi"}}]`)
		once := recommendation()
		apply(t, forced, once)
		twice := once.DeepCopy()
		apply(t, forced, twice)

		assert.Equal(t, once.ResourcesHash, twice.ResourcesHash)
		assert.Equal(t, flattenResources(once), flattenResources(twice))
	})

	t.Run("only usable values are overlaid", func(t *testing.T) {
		// The annotation is not validated when parsed: unsupported resources, non-positive quantities,
		// nameless entries and runtime values are dropped when merged.
		values := recommendation()
		forced := mustParse(t, `[
			{"name": "app", "requests": {"cpu": "0", "nvidia.com/gpu": "1", "memory": "300Mi"}, "limits": {"cpu": "-1"}, "runtime": {"gomemlimit": "1Gi"}},
			{"requests": {"cpu": "1"}},
			{"name": "other", "requests": {"cpu": "-2"}}
		]`)
		limitErr, err := applyForcedResources(values, forced)
		require.NoError(t, err)
		require.Error(t, limitErr)
		assert.Contains(t, limitErr.Error(), "containers app by")

		expected := maps.Clone(baseline)
		expected["app/requests/memory"] = "300Mi"
		assert.Equal(t, expected, flattenResources(values), "only the positive memory request of app is forced")
		assert.Nil(t, values.ContainerResources[0].Runtime)
	})

	t.Run("nothing usable is not an override", func(t *testing.T) {
		values := recommendation()
		limitErr, err := applyForcedResources(values, mustParse(t, `[{"name": "app", "requests": {"cpu": "0"}}]`))
		require.NoError(t, err)
		assert.NoError(t, limitErr, "nothing is reported as forced")
		assert.Equal(t, "recommendation-hash", values.ResourcesHash)
		assert.Equal(t, baseline, flattenResources(values))
	})
}
