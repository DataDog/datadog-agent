// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package workload

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	k8sutil "github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	appsv1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

type mockPodWatcher struct {
	mock.Mock
}

// GetPodsForOwner is a mocked function
func (m *mockPodWatcher) GetPodsForOwner(owner NamespacedPodOwner) []*workloadmeta.KubernetesPod {
	args := m.Called(owner)
	return args.Get(0).([]*workloadmeta.KubernetesPod)
}

// Start is a mocked function
func (m *mockPodWatcher) Start(ctx context.Context) {
	m.Called(ctx)
}

func TestMalformedPodAutoscaler(t *testing.T) {
	resources := model.ScalingValues{
		Vertical: &model.VerticalScalingValues{
			ContainerResources: []v1alpha1.DatadogPodAutoscalerContainerResources{
				{
					Name:     "container1",
					Requests: corev1.ResourceList{"cpu": resource.MustParse("100m")},
					Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
				},
			},
		},
	}
	for _, tt := range []struct {
		name          string
		workload      *model.PodAutoscalerInternal
		pods          []*workloadmeta.KubernetesPod
		expected      processResult
		expectedError bool
	}{
		{
			name: "missing spec",
			workload: &model.PodAutoscalerInternal{
				Namespace:         "test",
				ScalingValues:     resources,
				ScalingValuesHash: "hash",
			},
			expected: processResult{
				ProcessResult: autoscaling.NoRequeue,
				updateStatus:  true,
			},
			expectedError: true,
		},
		{
			name: "missing targetRef name",
			workload: &model.PodAutoscalerInternal{
				Namespace: "test",
				Spec: &v1alpha1.DatadogPodAutoscalerSpec{
					TargetRef: v2.CrossVersionObjectReference{
						Kind:       k8sutil.DeploymentKind,
						APIVersion: "apps/v1",
					},
				},
				ScalingValues:     resources,
				ScalingValuesHash: "hash",
			},
			expected: processResult{
				ProcessResult: autoscaling.NoRequeue,
				updateStatus:  true,
			},
			expectedError: true,
		},
		{
			name: "missing targetRef kind",
			workload: &model.PodAutoscalerInternal{
				Namespace: "test",
				Spec: &v1alpha1.DatadogPodAutoscalerSpec{
					TargetRef: v2.CrossVersionObjectReference{
						Name:       "datadog-cluster-agent",
						APIVersion: "apps/v1",
					},
				},
				ScalingValues:     resources,
				ScalingValuesHash: "hash",
			},
			expected: processResult{
				ProcessResult: autoscaling.NoRequeue,
				updateStatus:  true,
			},
			expectedError: true,
		},
		{
			name: "missing scaling hash",
			workload: &model.PodAutoscalerInternal{
				Namespace: "test",
				Spec: &v1alpha1.DatadogPodAutoscalerSpec{
					TargetRef: v2.CrossVersionObjectReference{
						Name:       "datadog-cluster-agent",
						Kind:       k8sutil.DeploymentKind,
						APIVersion: "apps/v1",
					},
				},
				ScalingValues: resources,
			},
			expected: processResult{
				ProcessResult: autoscaling.NoRequeue,
				updateStatus:  true,
			},
			expectedError: true,
		},
		{
			name: "missing API version in targetRef",
			workload: &model.PodAutoscalerInternal{
				Namespace: "test",
				Spec: &v1alpha1.DatadogPodAutoscalerSpec{
					TargetRef: v2.CrossVersionObjectReference{
						Name: "datadog-cluster-agent",
						Kind: k8sutil.DeploymentKind,
					},
				},
				ScalingValues:     resources,
				ScalingValuesHash: "hash",
			},
			expected: processResult{
				ProcessResult: autoscaling.NoRequeue,
				updateStatus:  true,
			},
			expectedError: true,
		},
		{
			name: "incorrect API version in targetRef",
			workload: &model.PodAutoscalerInternal{
				Namespace: "test",
				Spec: &v1alpha1.DatadogPodAutoscalerSpec{
					TargetRef: v2.CrossVersionObjectReference{
						Name:       "datadog-cluster-agent",
						Kind:       k8sutil.DeploymentKind,
						APIVersion: "abc/bcd/efg",
					},
				},
				ScalingValues:     resources,
				ScalingValuesHash: "hash",
			},
			expected: processResult{
				ProcessResult: autoscaling.NoRequeue,
				updateStatus:  true,
			},
			expectedError: true,
		},
		{
			name: "unsupported owner kind",
			workload: &model.PodAutoscalerInternal{
				Namespace: "test",
				Spec: &v1alpha1.DatadogPodAutoscalerSpec{
					TargetRef: v2.CrossVersionObjectReference{
						Name:       "datadog-cluster-agent",
						Kind:       k8sutil.StatefulSetKind,
						APIVersion: "abc",
					},
				},
				ScalingValues:     resources,
				ScalingValuesHash: "hash",
			},
			expected: processResult{
				ProcessResult: autoscaling.NoRequeue,
				updateStatus:  true,
			},
			expectedError: true,
		},
		{
			name: "pods already match the scaling values",
			workload: &model.PodAutoscalerInternal{
				Namespace: "test",
				Spec: &v1alpha1.DatadogPodAutoscalerSpec{
					TargetRef: v2.CrossVersionObjectReference{
						Name:       "datadog-cluster-agent",
						Kind:       k8sutil.DeploymentKind,
						APIVersion: "apps/v1",
					},
				},
				ScalingValues:     resources,
				ScalingValuesHash: "hash",
			},
			pods: []*workloadmeta.KubernetesPod{
				{
					Containers: []workloadmeta.OrchestratorContainer{
						{
							Name:     resources.Vertical.ContainerResources[0].Name,
							Requests: map[string]string{"cpu": "100m"},
							Limits:   map[string]string{"cpu": "200m"},
						},
					},
				},
			},
			expected: processResult{
				ProcessResult: autoscaling.NoRequeue,
				updateStatus:  false,
			},
		},
		{
			name: "pods owned by a deployment with different rs",
			workload: &model.PodAutoscalerInternal{
				Namespace: "test",
				Spec: &v1alpha1.DatadogPodAutoscalerSpec{
					TargetRef: v2.CrossVersionObjectReference{
						Name:       "datadog-cluster-agent",
						Kind:       k8sutil.DeploymentKind,
						APIVersion: "apps/v1",
					},
				},
				ScalingValues:     resources,
				ScalingValuesHash: "hash",
			},
			pods: []*workloadmeta.KubernetesPod{
				{
					Containers: []workloadmeta.OrchestratorContainer{
						{
							Name:     resources.Vertical.ContainerResources[0].Name,
							Requests: map[string]string{"cpu": "10m"},
							Limits:   map[string]string{"cpu": "20m"},
						},
					},
					Owners: []workloadmeta.KubernetesPodOwner{
						{
							Kind: k8sutil.ReplicaSetKind,
							ID:   "fdc1335a-27ae-40d0-9eaa-e43b26555cff",
						},
					},
				},
				{
					Containers: []workloadmeta.OrchestratorContainer{
						{
							Name:     resources.Vertical.ContainerResources[0].Name,
							Requests: map[string]string{"cpu": "10m"},
							Limits:   map[string]string{"cpu": "20m"},
						},
					},
					Owners: []workloadmeta.KubernetesPodOwner{
						{
							Kind: k8sutil.ReplicaSetKind,
							ID:   "fdc1335a-aaaa-40d0-9eaa-e43b26555cff",
						},
					},
				},
			},
			expected: processResult{
				ProcessResult: autoscaling.Requeue,
				updateStatus:  true,
			},
			expectedError: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pw := &mockPodWatcher{}
			pw.On("GetPodsForOwner", mock.Anything).Return(tt.pods)
			controller := newVerticalController(dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()), pw)

			ctx := context.TODO()
			result, err := controller.sync(ctx, tt.workload)
			if tt.expectedError {
				assert.Errorf(t, err, "expected error")
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessSuccessfulPatch(t *testing.T) {
	pods := []*workloadmeta.KubernetesPod{{
		Containers: []workloadmeta.OrchestratorContainer{
			{
				Name: "container1",
				Requests: map[string]string{
					"cpu": "50m",
				},
				Limits: map[string]string{
					"cpu": "100m",
				},
			},
		},
	}}

	podAutoscalerInternal := &model.PodAutoscalerInternal{
		Namespace: "test",
		Spec: &v1alpha1.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Kind:       k8sutil.DeploymentKind,
				Name:       "test-deployment",
				APIVersion: "apps/v1",
			},
		},
		ScalingValues: model.ScalingValues{
			Vertical: &model.VerticalScalingValues{
				ContainerResources: []v1alpha1.DatadogPodAutoscalerContainerResources{
					{
						Name:     "container1",
						Requests: corev1.ResourceList{"cpu": resource.MustParse("100m")},
						Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
					},
				},
			},
		},
		ScalingValuesHash: "hash",
	}
	pw := &mockPodWatcher{}
	pw.On("GetPodsForOwner", mock.Anything).Return(pods)

	cl := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	// Create the fake deployment
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       k8sutil.DeploymentKind,
			"metadata": map[string]interface{}{
				"name":      "test-deployment",
				"namespace": "test",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"key":      "value",
							annotation: "0x1",
						},
					},
				},
			},
		},
	}

	gvr := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}
	_, err := cl.Resource(gvr).Namespace("test").Create(context.TODO(), deployment, metav1.CreateOptions{})
	assert.NoError(t, err)

	controller := newVerticalController(cl, pw)
	ctx := context.TODO()
	result, err := controller.sync(ctx, podAutoscalerInternal)
	assert.NoError(t, err)
	assert.Equal(t, result, processResult{
		ProcessResult: autoscaling.ProcessResult{
			RequeueAfter: requeueAfterSuccessTime,
		},
		updateStatus: true,
	})
	unstruct, err := cl.Resource(gvr).Namespace("test").Get(context.TODO(), "test-deployment", metav1.GetOptions{})
	assert.NoError(t, err)
	newDep := &appsv1.Deployment{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstruct.UnstructuredContent(), newDep)
	assert.NoError(t, err)
	assert.Equal(t, "value", newDep.Spec.Template.GetAnnotations()["key"])
	assert.Equal(t, "hash", newDep.Spec.Template.GetAnnotations()[annotation])
}
