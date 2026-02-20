// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	kubernetesresourceparsers "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func Test_CollectEventsWithFullPod(t *testing.T) {
	t.Parallel()
	objectMeta := metav1.ObjectMeta{
		Name:   "test-pod",
		Labels: map[string]string{"test-label": "test-value"},
		UID:    types.UID("test-pod-uid"),
	}

	createResource := func(cl *fake.Clientset) error {
		_, err := cl.CoreV1().Pods(metav1.NamespaceAll).Create(context.TODO(), &corev1.Pod{ObjectMeta: objectMeta}, metav1.CreateOptions{})
		return err
	}
	expected := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.KubernetesPod{
					Containers: []workloadmeta.OrchestratorContainer{},
					EntityID: workloadmeta.EntityID{
						ID:   string(objectMeta.UID),
						Kind: workloadmeta.KindKubernetesPod,
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:   objectMeta.Name,
						Labels: objectMeta.Labels,
					},
					Owners: []workloadmeta.KubernetesPodOwner{},
				},
				IsComplete: true,
			},
		},
	}

	testCollectEvent(t, createResource, newPodStoreWithTypedClient, expected)
}

func Test_CollectEventsWithMinimalPod(t *testing.T) {
	objectMeta := metav1.ObjectMeta{
		Name:   "test-pod",
		Labels: map[string]string{"test-label": "test-value"},
		UID:    types.UID("test-pod-uid"),
	}

	overrides := map[string]interface{}{
		"cluster_agent.collect_kubernetes_tags": true,
	}

	wmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component {
			return config.NewMockWithOverrides(t, overrides)
		}),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	store := newPodReflectorStoreWithMinimalPodParser(wmeta, wmeta.GetConfig())

	ch := wmeta.Subscribe(dummySubscriber, workloadmeta.NormalPriority, nil)
	defer wmeta.Unsubscribe(ch)

	bundleCh := make(chan workloadmeta.EventBundle, 1)
	doneCh := make(chan struct{})
	defer close(doneCh)

	go func() {
		for {
			select {
			case bundle := <-ch:
				bundle.Acknowledge()
				if len(bundle.Events) > 0 {
					bundleCh <- bundle
					return
				}
			case <-doneCh:
				return
			}
		}
	}()

	pod := &MinimalPod{
		ObjectMeta: objectMeta,
		Spec:       MinimalPodSpec{Containers: []MinimalContainer{}},
	}
	err := store.Add(pod)
	require.NoError(t, err)

	var bundle workloadmeta.EventBundle
	select {
	case bundle = <-bundleCh:
		// Received bundle. Continue.
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	expected := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.KubernetesPod{
					Containers: []workloadmeta.OrchestratorContainer{},
					EntityID: workloadmeta.EntityID{
						ID:   string(objectMeta.UID),
						Kind: workloadmeta.KindKubernetesPod,
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      objectMeta.Name,
						Namespace: objectMeta.Namespace,
						Labels:    objectMeta.Labels,
					},
					Owners: []workloadmeta.KubernetesPodOwner{},
				},
				IsComplete: true,
			},
		},
	}

	bundle.Ch = nil // to avoid comparing the channel
	assert.Equal(t, expected, bundle)
}

func Test_MinimalPodDeepCopy(t *testing.T) {
	runtimeClassName := "test-runtime"

	original := &MinimalPod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			UID:       types.UID("test-uid-12345"),
			Labels: map[string]string{
				"app": "test-app",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "ReplicaSet",
					Name:       "test-rs",
					UID:        types.UID("owner-uid"),
					APIVersion: "apps/v1",
				},
			},
			ResourceVersion: "12345",
		},
		Spec: MinimalPodSpec{
			Containers: []MinimalContainer{
				{
					Name: "test-container-1",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: *resource.NewMilliQuantity(100, resource.DecimalSI),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: *resource.NewMilliQuantity(200, resource.DecimalSI),
						},
					},
				},
				{
					Name: "test-container-2",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: *resource.NewMilliQuantity(50, resource.DecimalSI),
						},
					},
				},
			},
			Volumes: []MinimalVolume{
				{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "test-pvc",
						ReadOnly:  false,
					},
				},
			},
			RuntimeClassName:  &runtimeClassName,
			PriorityClassName: "high-priority",
		},
		Status: MinimalPodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
			PodIP:    "10.0.0.1",
			QOSClass: corev1.PodQOSBurstable,
		},
	}

	copied := original.DeepCopyObject()

	copiedPod, ok := copied.(*MinimalPod)
	require.True(t, ok)

	// Verify all fields are equal
	assert.Equal(t, original.TypeMeta, copiedPod.TypeMeta)
	assert.Equal(t, original.ObjectMeta, copiedPod.ObjectMeta)
	assert.Equal(t, original.Spec, copiedPod.Spec)
	assert.Equal(t, original.Status, copiedPod.Status)

	// Verify it's a deep copy by modifying a few fields of the copy and
	// checking the original did not change
	copiedPod.Name = "modified-name"
	assert.NotEqual(t, copiedPod.Name, original.Name)

	copiedPod.Labels["new-label"] = "new-value"
	_, existsInOriginal := original.Labels["new-label"]
	assert.False(t, existsInOriginal)

	copiedPod.Spec.Containers[0].Name = "modified-container"
	assert.NotEqual(t, copiedPod.Spec.Containers[0].Name, original.Spec.Containers[0].Name)

	copiedPod.Status.PodIP = "10.0.0.2"
	assert.NotEqual(t, copiedPod.Status.PodIP, original.Status.PodIP)
}

// Test_ParsersProduceSameOutput verifies that the minimalPodParser produces
// the same output as the full podParser.
func Test_ParsersProduceSameOutput(t *testing.T) {
	runtimeClassName := "test-runtime"

	fullPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			UID:       types.UID("test-uid-12345"),
			Labels: map[string]string{
				"app":     "test-app",
				"version": "v1",
			},
			Annotations: map[string]string{
				"annotation-1": "value-1",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
					Name: "test-rs",
					UID:  types.UID("owner-uid"),
				},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "container-1",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("200m"),
						},
					},
				},
				{
					Name: "container-2",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("50m"),
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "data-volume",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "test-pvc",
						},
					},
				},
				{
					Name: "config-volume",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "test-cm"},
						},
					},
				},
			},
			RuntimeClassName:  &runtimeClassName,
			PriorityClassName: "high-priority",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
			PodIP:    "10.0.0.1",
			QOSClass: corev1.PodQOSBurstable,
		},
	}

	// this MinimalPod is the simplification of the fullPod defined above
	minimalPod := &MinimalPod{
		ObjectMeta: fullPod.ObjectMeta,
		Spec: MinimalPodSpec{
			Containers: []MinimalContainer{
				{
					Name:      "container-1",
					Resources: fullPod.Spec.Containers[0].Resources,
				},
				{
					Name:      "container-2",
					Resources: fullPod.Spec.Containers[1].Resources,
				},
			},
			Volumes: []MinimalVolume{
				{
					PersistentVolumeClaim: fullPod.Spec.Volumes[0].PersistentVolumeClaim,
				},
				// The second volume is not a PVC, so it is not included in the MinimalPod
			},
			RuntimeClassName:  fullPod.Spec.RuntimeClassName,
			PriorityClassName: fullPod.Spec.PriorityClassName,
		},
		Status: MinimalPodStatus{
			Phase:      fullPod.Status.Phase,
			Conditions: fullPod.Status.Conditions,
			PodIP:      fullPod.Status.PodIP,
			QOSClass:   fullPod.Status.QOSClass,
		},
	}

	fullParser, err := kubernetesresourceparsers.NewPodParser(nil)
	require.NoError(t, err)
	fullParserResult := fullParser.Parse(fullPod)

	minimalParser := minimalPodParser{}
	minimalParserResult := minimalParser.Parse(minimalPod)

	assert.Equal(t, fullParserResult, minimalParserResult)
}
