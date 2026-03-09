// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
	clock "k8s.io/utils/clock/testing"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

type fixture struct {
	*autoscaling.ControllerFixture

	clock           *clock.FakeClock
	recorder        *record.FakeRecorder
	store           *store
	autoscalingHeap *autoscaling.HashHeap[model.PodAutoscalerInternal]
	scaler          *fakeScaler
	podWatcher      *fakePodWatcher
}

const testMaxAutoscalerObjects int = 2

func newFixture(t *testing.T, testTime time.Time) *fixture {
	store := autoscaling.NewStore[model.PodAutoscalerInternal]()

	clock := clock.NewFakeClock(testTime)
	recorder := record.NewFakeRecorder(100)
	hashHeap := autoscaling.NewHashHeap(testMaxAutoscalerObjects, store, (*model.PodAutoscalerInternal).CreationTimestamp)
	scaler := newFakeScaler()
	podWatcher := newFakePodWatcher()
	return &fixture{
		ControllerFixture: autoscaling.NewFixture(
			t, podAutoscalerGVR,
			func(fakeClient *fake.FakeDynamicClient, informer dynamicinformer.DynamicSharedInformerFactory, isLeader func() bool) (*autoscaling.Controller, error) {
				c, err := NewController(clock, "cluster-id1", recorder, nil, nil, fakeClient, informer, isLeader, store, podWatcher, nil, hashHeap)
				if err != nil {
					return nil, err
				}

				// Patching controller and horizontal controller scaler to use the fake scaler
				c.scaler = scaler
				c.horizontalController.scaler = scaler
				return c.Controller, err
			},
		),
		clock:           clock,
		recorder:        recorder,
		store:           store,
		autoscalingHeap: hashHeap,
		scaler:          scaler,
		podWatcher:      podWatcher,
	}
}

func newFakePodAutoscaler(ns, name string, gen int64, creationTimestamp time.Time, spec datadoghq.DatadogPodAutoscalerSpec, status datadoghqcommon.DatadogPodAutoscalerStatus) (obj *unstructured.Unstructured, dpa *datadoghq.DatadogPodAutoscaler) {
	dpa = &datadoghq.DatadogPodAutoscaler{
		TypeMeta: podAutoscalerMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec:   spec,
		Status: status,
	}

	// Only add extra information for local owner
	if gen > 0 {
		dpa.Generation = gen
		dpa.UID = uuid.NewUUID()
		dpa.CreationTimestamp = metav1.NewTime(creationTimestamp)
	}

	obj, err := autoscaling.ToUnstructured(dpa)
	if err != nil {
		panic("Failed to construct unstructured DDM")
	}

	return
}

func TestLeaderCreateDeleteLocal(t *testing.T) {
	testTime := time.Now()
	f := newFixture(t, testTime)

	dpaSpec := datadoghq.DatadogPodAutoscalerSpec{
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			Kind:       "Deployment",
			Name:       "app-0",
			APIVersion: "apps/v1",
		},
		// Local owner means .Spec source of truth is K8S
		Owner: datadoghqcommon.DatadogPodAutoscalerLocalOwner,
	}

	defaultCreationTime := time.Time{}
	// Read newly created DPA
	dpa, dpaTyped := newFakePodAutoscaler("default", "dpa-0", 1, defaultCreationTime, dpaSpec, datadoghqcommon.DatadogPodAutoscalerStatus{})

	f.InformerObjects = append(f.InformerObjects, dpa)
	f.Objects = append(f.Objects, dpaTyped)

	f.RunControllerSync(true, "default/dpa-0")

	// Check internal store content
	expectedDPAInternal := model.FakePodAutoscalerInternal{
		Namespace:                      "default",
		Name:                           "dpa-0",
		Generation:                     1,
		Spec:                           &dpaSpec,
		CustomRecommenderConfiguration: nil,
	}
	dpaInternal, found := f.store.Get("default/dpa-0")
	assert.True(t, found)
	model.AssertPodAutoscalersEqual(t, expectedDPAInternal, dpaInternal)

	// Object deleted from Kubernetes, should be deleted from store
	f.InformerObjects = nil
	f.Objects = nil

	f.RunControllerSync(true, "default/dpa-0")
	assert.Len(t, f.store.GetAll(), 0)

	// Re-create object
	f.InformerObjects = append(f.InformerObjects, dpa)
	f.Objects = append(f.Objects, dpaTyped)

	f.RunControllerSync(true, "default/dpa-0")

	assert.True(t, found)
	model.AssertPodAutoscalersEqual(t, expectedDPAInternal, dpaInternal)
}

func TestLeaderCreateDeleteRemote(t *testing.T) {
	testTime := time.Now()
	f := newFixture(t, testTime)

	dpaSpec := datadoghq.DatadogPodAutoscalerSpec{
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			Kind:       "Deployment",
			Name:       "app-0",
			APIVersion: "apps/v1",
		},
		// Remote owner means .Spec source of truth is Datadog App
		Owner: datadoghqcommon.DatadogPodAutoscalerRemoteOwner,
	}

	dpaInternal := model.FakePodAutoscalerInternal{
		Namespace: "default",
		Name:      "dpa-0",
		Spec:      &dpaSpec,
	}
	f.store.Set("default/dpa-0", dpaInternal.Build(), controllerID)

	// Should create object in Kubernetes
	expectedDPA := &datadoghq.DatadogPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogPodAutoscaler",
			APIVersion: "datadoghq.com/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dpa-0",
			Namespace: "default",
		},
		Spec: dpaSpec,
		Status: datadoghqcommon.DatadogPodAutoscalerStatus{
			Conditions: []datadoghqcommon.DatadogPodAutoscalerCondition{
				condition(datadoghqcommon.DatadogPodAutoscalerErrorCondition, corev1.ConditionFalse, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerActiveCondition, corev1.ConditionTrue, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToRecommendCondition, corev1.ConditionUnknown, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToRecommendCondition, corev1.ConditionUnknown, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalScalingLimitedCondition, corev1.ConditionFalse, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalScalingLimitedCondition, corev1.ConditionFalse, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToScaleCondition, corev1.ConditionUnknown, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToApply, corev1.ConditionUnknown, "", "", testTime),
			},
		},
	}
	expectedUnstructured := mustUnstructured(t, expectedDPA)
	f.ExpectCreateAction(expectedUnstructured)
	f.RunControllerSync(true, "default/dpa-0")

	// We flag the object as deleted in the store, we expect delete operation in Kubernetes
	dpaInternal.Deleted = true
	f.store.Set("default/dpa-0", dpaInternal.Build(), controllerID)
	f.InformerObjects = append(f.InformerObjects, expectedUnstructured)
	f.Objects = append(f.Objects, expectedDPA)
	f.Actions = nil

	f.ExpectDeleteAction("default", "dpa-0")
	f.RunControllerSync(true, "default/dpa-0")
	assert.Len(t, f.store.GetAll(), 1) // Still in store

	// Next reconcile the controller is going to remove the object from the store
	f.InformerObjects = nil
	f.Objects = nil
	f.Actions = nil
	f.RunControllerSync(true, "default/dpa-0")
	assert.Len(t, f.store.GetAll(), 0)
}

func TestLeaderCreateDeleteRemoteDefaultedSpec(t *testing.T) {
	testTime := time.Now()
	f := newFixture(t, testTime)

	dpaSpec := datadoghq.DatadogPodAutoscalerSpec{
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			Kind:       "Deployment",
			Name:       "app-0",
			APIVersion: "apps/v1",
		},
		// Remote owner means .Spec source of truth is Datadog App
		Owner:         datadoghqcommon.DatadogPodAutoscalerRemoteOwner,
		RemoteVersion: pointer.Ptr[uint64](1000),
	}

	dpaInternal := model.FakePodAutoscalerInternal{
		Namespace: "default",
		Name:      "dpa-0",
		Spec:      &dpaSpec,
	}
	f.store.Set("default/dpa-0", dpaInternal.Build(), controllerID)

	// Should create object in Kubernetes
	expectedDPA := &datadoghq.DatadogPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogPodAutoscaler",
			APIVersion: "datadoghq.com/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dpa-0",
			Namespace: "default",
		},
		Spec: dpaSpec,
		Status: datadoghqcommon.DatadogPodAutoscalerStatus{
			Conditions: []datadoghqcommon.DatadogPodAutoscalerCondition{
				condition(datadoghqcommon.DatadogPodAutoscalerErrorCondition, corev1.ConditionFalse, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerActiveCondition, corev1.ConditionTrue, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToRecommendCondition, corev1.ConditionUnknown, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToRecommendCondition, corev1.ConditionUnknown, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalScalingLimitedCondition, corev1.ConditionFalse, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalScalingLimitedCondition, corev1.ConditionFalse, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToScaleCondition, corev1.ConditionUnknown, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToApply, corev1.ConditionUnknown, "", "", testTime),
			},
		},
	}
	expectedUnstructured := mustUnstructured(t, expectedDPA)

	// We need to add a reactor to create the object with the correct generation and creation timestamp
	f.FakeClientCustomHook = func(client *fake.FakeDynamicClient) {
		client.PrependReactor("create", "datadogpodautoscalers", func(k8stesting.Action) (handled bool, ret runtime.Object, err error) {
			returnedObj := expectedUnstructured.DeepCopy()
			returnedObj.SetGeneration(1)
			returnedObj.SetCreationTimestamp(metav1.NewTime(testTime))
			return true, returnedObj, nil
		})
	}
	f.ExpectCreateAction(expectedUnstructured)
	f.RunControllerSync(true, "default/dpa-0")

	// Now next sync we actually get the object with defaulted spec, so hashes won't match remote version
	fallbackDefaulted := &datadoghq.DatadogFallbackPolicy{
		Horizontal: datadoghq.DatadogPodAutoscalerHorizontalFallbackPolicy{
			Enabled:   true,
			Direction: datadoghq.DatadogPodAutoscalerFallbackDirectionScaleUp,
			Triggers: datadoghq.HorizontalFallbackTriggers{
				StaleRecommendationThresholdSeconds: 600,
			},
		},
	}
	defaultedDPA := expectedDPA.DeepCopy()
	defaultedDPA.Generation = 1
	defaultedDPA.CreationTimestamp = metav1.NewTime(testTime)
	defaultedDPA.Spec.Fallback = fallbackDefaulted

	f.InformerObjects = append(f.InformerObjects, mustUnstructured(t, defaultedDPA))
	f.Objects = append(f.Objects, defaultedDPA)
	f.Actions = nil
	f.FakeClientCustomHook = nil

	// The controller is going to try to reconcile now
	f.scaler.mockGet(model.FakePodAutoscalerInternal{
		Namespace: "default",
		Name:      "dpa-0",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       "app-0",
				APIVersion: "apps/v1",
			},
		},
		TargetGVK: schema.GroupVersionKind{
			Group:   "apps",
			Version: "v1",
			Kind:    "Deployment",
		},
	}, 1, 1, nil)
	f.podWatcher.mockGetPodsForOwner(NamespacedPodOwner{
		Namespace: "default",
		Kind:      "Deployment",
		Name:      "app-0",
	}, []*workloadmeta.KubernetesPod{{}})

	expectedDPA.Generation = 1
	expectedDPA.CreationTimestamp = metav1.NewTime(testTime)
	expectedDPA.Spec = datadoghq.DatadogPodAutoscalerSpec{}
	expectedDPA.Status.CurrentReplicas = pointer.Ptr[int32](1)
	f.ExpectUpdateStatusAction(mustUnstructured(t, expectedDPA))

	// The controller should do nothing (no update calls)
	f.RunControllerSync(true, "default/dpa-0")
}

func TestDatadogPodAutoscalerTargetingClusterAgentErrors(t *testing.T) {
	tests := []struct {
		name      string
		targetRef autoscalingv2.CrossVersionObjectReference
	}{
		{
			"target set to cluster agent deployment",
			autoscalingv2.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       "datadog-agent-cluster-agent",
				APIVersion: "apps/v1",
			},
		},
		{
			"target set to cluster agent replicaset",
			autoscalingv2.CrossVersionObjectReference{
				Kind:       "ReplicaSet",
				Name:       "datadog-agent-cluster-agent-7dbf798595",
				APIVersion: "apps/v1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testTime := time.Now()
			f := newFixture(t, testTime)

			t.Setenv("DD_POD_NAME", "datadog-agent-cluster-agent-7dbf798595-tp9lg")
			currentNs := namespace.GetMyNamespace()
			id := currentNs + "/dpa-dca"

			dpaSpec := datadoghq.DatadogPodAutoscalerSpec{
				TargetRef: tt.targetRef,
				// Local owner means .Spec source of truth is K8S
				Owner: datadoghqcommon.DatadogPodAutoscalerLocalOwner,
			}

			// Create object in store
			dpa, dpaTyped := newFakePodAutoscaler(currentNs, "dpa-dca", 1, testTime, dpaSpec, datadoghqcommon.DatadogPodAutoscalerStatus{})
			f.InformerObjects = append(f.InformerObjects, dpa)
			f.Objects = append(f.Objects, dpaTyped)

			f.RunControllerSync(true, id)
			_, found := f.store.Get(id)
			assert.True(t, found)

			// Test that object gets updated with correct error status
			expectedDPAError := &datadoghq.DatadogPodAutoscaler{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DatadogPodAutoscaler",
					APIVersion: "datadoghq.com/v1alpha2",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "dpa-dca",
					Namespace:         currentNs,
					Generation:        1,
					UID:               dpa.GetUID(),
					CreationTimestamp: metav1.NewTime(testTime),
				},
				Spec: datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "",
						Name:       "",
						APIVersion: "",
					},
					Owner: "",
				},
				Status: datadoghqcommon.DatadogPodAutoscalerStatus{
					Conditions: []datadoghqcommon.DatadogPodAutoscalerCondition{
						condition(datadoghqcommon.DatadogPodAutoscalerErrorCondition, corev1.ConditionTrue, "InvalidTarget", "Autoscaling target cannot be set to the cluster agent", testTime),
						condition(datadoghqcommon.DatadogPodAutoscalerActiveCondition, corev1.ConditionTrue, "", "", testTime),
						condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToRecommendCondition, corev1.ConditionUnknown, "", "", testTime),
						condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToRecommendCondition, corev1.ConditionUnknown, "", "", testTime),
						condition(datadoghqcommon.DatadogPodAutoscalerHorizontalScalingLimitedCondition, corev1.ConditionFalse, "", "", testTime),
						condition(datadoghqcommon.DatadogPodAutoscalerVerticalScalingLimitedCondition, corev1.ConditionFalse, "", "", testTime),
						condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToScaleCondition, corev1.ConditionUnknown, "", "", testTime),
						condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToApply, corev1.ConditionUnknown, "", "", testTime),
					},
				},
			}

			f.ExpectUpdateStatusAction(mustUnstructured(t, expectedDPAError))
			f.RunControllerSync(true, id)
			assert.Len(t, f.store.GetAll(), 1)
			pai, found := f.store.Get(id)
			assert.Truef(t, found, "Expected to find DatadogPodAutoscaler in store")
			assert.EqualError(t, pai.Error(), "Autoscaling target cannot be set to the cluster agent")
		})
	}
}

func TestPodAutoscalerClearStatusOnScalingModeChange(t *testing.T) {
	testTime := time.Now()
	creationTime := testTime.Add(-2 * time.Hour)
	f := newFixture(t, testTime)

	// Starting case, multi-dim DPA
	dpaSpec := datadoghq.DatadogPodAutoscalerSpec{
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			Kind:       "Deployment",
			Name:       "app-0",
			APIVersion: "apps/v1",
		},
		Owner: datadoghqcommon.DatadogPodAutoscalerLocalOwner,
		Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{{
			Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
			PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
				Name: corev1.ResourceCPU,
				Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
					Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
					Utilization: pointer.Ptr[int32](80),
				},
			},
		}},
	}
	dpa, dpaTyped := newFakePodAutoscaler("default", "dpa-0", 0, creationTime, dpaSpec, datadoghqcommon.DatadogPodAutoscalerStatus{})
	f.InformerObjects = []*unstructured.Unstructured{dpa}
	f.Objects = []runtime.Object{dpaTyped}

	// Horizontal and Vertical mocks
	f.scaler.On("get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&autoscalingv1.Scale{
		Spec: autoscalingv1.ScaleSpec{
			Replicas: 4,
		},
		Status: autoscalingv1.ScaleStatus{
			Replicas: 4,
		},
	}, schema.GroupResource{}, nil).Maybe()
	f.podWatcher.mockGetPodsForOwner(NamespacedPodOwner{
		Namespace: "default",
		Kind:      "Deployment",
		Name:      "app-0",
	}, []*workloadmeta.KubernetesPod{{}})

	// Recs are vertical error, horizontal able to recommend
	dpaInternal := model.FakePodAutoscalerInternal{
		Namespace:         "default",
		Name:              "dpa-0",
		Spec:              &dpaSpec,
		CreationTimestamp: creationTime,
		MainScalingValues: model.ScalingValues{
			VerticalError: errors.New("no data available"),
			Vertical: &model.VerticalScalingValues{
				Source:        datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp:     testTime.Add(-8 * time.Hour),
				ResourcesHash: "abc123",
				ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{{
					Name: "app",
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				}},
			},
			Horizontal: &model.HorizontalScalingValues{
				Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp: testTime,
				Replicas:  4,
			},
		},
		HorizontalLastActions: []datadoghqcommon.DatadogPodAutoscalerHorizontalAction{{
			Time:                metav1.NewTime(testTime.Add(-1 * time.Minute)),
			FromReplicas:        3,
			ToReplicas:          4,
			RecommendedReplicas: pointer.Ptr[int32](4),
		}},
		HorizontalLastRecommendations: []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{{
			Source:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
			GeneratedAt: metav1.NewTime(testTime),
			Replicas:    4,
		}},
		VerticalLastAction: &datadoghqcommon.DatadogPodAutoscalerVerticalAction{
			Time:    metav1.NewTime(testTime.Add(-8 * time.Hour)),
			Version: "abc123",
		},
	}
	f.store.Set("default/dpa-0", dpaInternal.Build(), controllerID)

	// Check generated status based on current state (both directions activated)
	cpuReqSum, memReqSum := dpaInternal.MainScalingValues.Vertical.SumCPUMemoryRequests()
	expectedDPA := &datadoghq.DatadogPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogPodAutoscaler",
			APIVersion: "datadoghq.com/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "default",
			Name:              "dpa-0",
			Generation:        dpaTyped.Generation,
			UID:               dpaTyped.GetUID(),
			CreationTimestamp: dpaTyped.CreationTimestamp,
		},
		Spec: datadoghq.DatadogPodAutoscalerSpec{},
		Status: datadoghqcommon.DatadogPodAutoscalerStatus{
			CurrentReplicas: pointer.Ptr[int32](1),
			Conditions: []datadoghqcommon.DatadogPodAutoscalerCondition{
				condition(datadoghqcommon.DatadogPodAutoscalerErrorCondition, corev1.ConditionFalse, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerActiveCondition, corev1.ConditionTrue, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToRecommendCondition, corev1.ConditionTrue, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToRecommendCondition, corev1.ConditionFalse, "", "no data available", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalScalingLimitedCondition, corev1.ConditionFalse, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalScalingLimitedCondition, corev1.ConditionFalse, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToScaleCondition, corev1.ConditionTrue, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToApply, corev1.ConditionTrue, "", "", testTime),
			},
			Horizontal: &datadoghqcommon.DatadogPodAutoscalerHorizontalStatus{
				Target: &datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
					Source:      dpaInternal.MainScalingValues.Horizontal.Source,
					GeneratedAt: metav1.NewTime(dpaInternal.MainScalingValues.Horizontal.Timestamp),
					Replicas:    dpaInternal.MainScalingValues.Horizontal.Replicas,
				},
				LastActions:         dpaInternal.HorizontalLastActions,
				LastRecommendations: dpaInternal.HorizontalLastRecommendations,
			},
			Vertical: &datadoghqcommon.DatadogPodAutoscalerVerticalStatus{
				Target: &datadoghqcommon.DatadogPodAutoscalerVerticalTargetStatus{
					Source:           dpaInternal.MainScalingValues.Vertical.Source,
					GeneratedAt:      metav1.NewTime(dpaInternal.MainScalingValues.Vertical.Timestamp),
					Version:          dpaInternal.MainScalingValues.Vertical.ResourcesHash,
					DesiredResources: dpaInternal.MainScalingValues.Vertical.ContainerResources,
					PodCPURequest:    cpuReqSum,
					PodMemoryRequest: memReqSum,
					Scaled:           pointer.Ptr[int32](0),
				},
				LastAction: dpaInternal.VerticalLastAction,
			},
		},
	}

	// Check current status is as expected
	f.ExpectUpdateStatusAction(mustUnstructured(t, expectedDPA))
	f.RunControllerSync(true, "default/dpa-0")

	// Now DPA is updated to be horizontal only, no change to internal DPA
	dpaTyped.Generation = 1
	dpaTyped.Status = expectedDPA.Status
	dpaTyped.Spec.ApplyPolicy = &datadoghq.DatadogPodAutoscalerApplyPolicy{
		Mode: datadoghq.DatadogPodAutoscalerApplyModeApply,
		Update: &datadoghqcommon.DatadogPodAutoscalerUpdatePolicy{
			Strategy: datadoghqcommon.DatadogPodAutoscalerDisabledUpdateStrategy,
		},
	}
	f.InformerObjects = []*unstructured.Unstructured{mustUnstructured(t, dpaTyped)}
	f.Objects = []runtime.Object{dpaTyped}

	// Check status has been cleared for vertical
	expectedDPA.Generation = 1
	expectedDPA.Status.Vertical = nil
	expectedDPA.Status.Conditions = []datadoghqcommon.DatadogPodAutoscalerCondition{
		condition(datadoghqcommon.DatadogPodAutoscalerErrorCondition, corev1.ConditionFalse, "", "", testTime),
		condition(datadoghqcommon.DatadogPodAutoscalerActiveCondition, corev1.ConditionTrue, "", "", testTime),
		condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToRecommendCondition, corev1.ConditionTrue, "", "", testTime),
		condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToRecommendCondition, corev1.ConditionUnknown, "", "", testTime),
		condition(datadoghqcommon.DatadogPodAutoscalerHorizontalScalingLimitedCondition, corev1.ConditionFalse, "", "", testTime),
		condition(datadoghqcommon.DatadogPodAutoscalerVerticalScalingLimitedCondition, corev1.ConditionFalse, "", "", testTime),
		condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToScaleCondition, corev1.ConditionTrue, "", "", testTime),
		condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToApply, corev1.ConditionUnknown, "", "", testTime),
	}

	// Check current status is as expected
	f.Actions = nil
	f.ExpectUpdateStatusAction(mustUnstructured(t, expectedDPA))
	f.RunControllerSync(true, "default/dpa-0")
}

func TestPodAutoscalerLocalOwnerObjectsLimit(t *testing.T) {
	testTime := time.Now()
	f := newFixture(t, testTime)

	dpaSpec := datadoghq.DatadogPodAutoscalerSpec{
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			Kind:       "Deployment",
			Name:       "app-0",
			APIVersion: "apps/v1",
		},
		// Local owner means .Spec source of truth is K8S
		Owner: datadoghqcommon.DatadogPodAutoscalerLocalOwner,
	}

	currentNs := namespace.GetMyNamespace()
	dpaID := currentNs + "/dpa-0"
	dpa1ID := currentNs + "/dpa-1"
	dpa2ID := currentNs + "/dpa-2"

	dpaTime := testTime.Add(-1 * time.Hour)
	dpa1Time := testTime
	dpa2Time := testTime.Add(1 * time.Hour)

	// Read newly created DPA
	dpa, dpaTyped := newFakePodAutoscaler(currentNs, "dpa-0", 1, dpaTime, dpaSpec, datadoghqcommon.DatadogPodAutoscalerStatus{})
	dpa1, dpaTyped1 := newFakePodAutoscaler(currentNs, "dpa-1", 1, dpa1Time, dpaSpec, datadoghqcommon.DatadogPodAutoscalerStatus{})
	dpa2, dpaTyped2 := newFakePodAutoscaler(currentNs, "dpa-2", 1, dpa2Time, dpaSpec, datadoghqcommon.DatadogPodAutoscalerStatus{})

	// Setup scaler mock to handle any get calls during concurrent processing
	f.scaler.On("get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&autoscalingv1.Scale{}, schema.GroupResource{}, nil).Maybe()

	f.InformerObjects = append(f.InformerObjects, dpa, dpa1)
	f.Objects = append(f.Objects, dpaTyped, dpaTyped1)

	f.RunControllerSync(true, dpa1ID)
	// Check that DatadogPodAutoscaler object is inserted into heap
	assert.Equal(t, 1, f.autoscalingHeap.MaxHeap.Len())
	assert.Equal(t, dpa1ID, f.autoscalingHeap.MaxHeap.Peek().Key)
	assert.Truef(t, f.autoscalingHeap.Keys[dpa1ID], "Expected dpa-1 to be in heap")

	f.InformerObjects = append(f.InformerObjects, dpa2)
	f.Objects = append(f.Objects, dpaTyped2)
	// Check that multiple objects can be inserted with ordering preserved
	f.RunControllerSync(true, dpa2ID)
	assert.Equal(t, 2, f.autoscalingHeap.MaxHeap.Len())
	assert.Equal(t, dpa2ID, f.autoscalingHeap.MaxHeap.Peek().Key)
	assert.Truef(t, f.autoscalingHeap.Keys[dpa1ID], "Expected dpa-1 to be in heap")
	assert.Truef(t, f.autoscalingHeap.Keys[dpa2ID], "Expected dpa-2 to be in heap")

	f.RunControllerSync(true, dpaID)
	// Check that heap ordering is preserved and limit is not exceeeded
	assert.Equal(t, 2, f.autoscalingHeap.MaxHeap.Len())
	assert.Equal(t, dpa1ID, f.autoscalingHeap.MaxHeap.Peek().Key)
	assert.Truef(t, f.autoscalingHeap.Keys[dpaID], "Expected dpa-0 to be in heap")
	assert.Truef(t, f.autoscalingHeap.Keys[dpa1ID], "Expected dpa-1 to be in heap")
	assert.Falsef(t, f.autoscalingHeap.Keys[dpa2ID], "Expected dpa-2 to not be in heap")

	// Check that when object (dpa1) is deleted from Kubernetes, heap is updated accordingly
	f.InformerObjects = nil
	f.Objects = nil
	f.RunControllerSync(true, dpa1ID)

	dpaStatusUpdate := &datadoghq.DatadogPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogPodAutoscaler",
			APIVersion: "datadoghq.com/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "dpa-2",
			Namespace:         currentNs,
			Generation:        1,
			UID:               dpa2.GetUID(),
			CreationTimestamp: metav1.NewTime(dpa2Time),
		},
		Spec: datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "",
				Name:       "",
				APIVersion: "",
			},
			Owner: "",
		},
		Status: datadoghqcommon.DatadogPodAutoscalerStatus{
			Conditions: []datadoghqcommon.DatadogPodAutoscalerCondition{
				condition(datadoghqcommon.DatadogPodAutoscalerErrorCondition, corev1.ConditionTrue, "ClusterAutoscalerLimitReached", fmt.Sprintf("Autoscaler disabled as maximum number per cluster reached (%d)", testMaxAutoscalerObjects), testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerActiveCondition, corev1.ConditionTrue, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToRecommendCondition, corev1.ConditionUnknown, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToRecommendCondition, corev1.ConditionUnknown, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalScalingLimitedCondition, corev1.ConditionFalse, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalScalingLimitedCondition, corev1.ConditionFalse, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToScaleCondition, corev1.ConditionUnknown, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToApply, corev1.ConditionUnknown, "", "", testTime),
			},
		},
	}
	f.ExpectUpdateStatusAction(mustUnstructured(t, dpaStatusUpdate))
	assert.Len(t, f.store.GetAll(), 2)
	f.InformerObjects = append(f.InformerObjects, dpa2)
	f.Objects = append(f.Objects, dpaTyped2)
	f.RunControllerSync(true, dpa2ID)

	assert.Equal(t, 2, f.autoscalingHeap.MaxHeap.Len())
	assert.Equal(t, dpa2ID, f.autoscalingHeap.MaxHeap.Peek().Key)
	assert.Truef(t, f.autoscalingHeap.Keys[dpaID], "Expected dpa-0 to be in heap")
	assert.Falsef(t, f.autoscalingHeap.Keys[dpa1ID], "Expected dpa-1 to not be in heap")
	assert.Truef(t, f.autoscalingHeap.Keys[dpa2ID], "Expected dpa-2 to be in heap")
}

func TestPodAutoscalerRemoteOwnerObjectsLimit(t *testing.T) {
	testTime := time.Now()
	f := newFixture(t, testTime)

	dpaSpec := datadoghq.DatadogPodAutoscalerSpec{
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			Kind:       "Deployment",
			Name:       "app-0",
			APIVersion: "apps/v1",
		},
		// Remote owner means .Spec source of truth is Datadog App
		Owner: datadoghqcommon.DatadogPodAutoscalerRemoteOwner,
	}

	dpa1Spec := datadoghq.DatadogPodAutoscalerSpec{
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			Kind:       "Deployment",
			Name:       "app-1",
			APIVersion: "apps/v1",
		},
		// Remote owner means .Spec source of truth is Datadog App
		Owner: datadoghqcommon.DatadogPodAutoscalerRemoteOwner,
	}
	dpa2Spec := datadoghq.DatadogPodAutoscalerSpec{
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			Kind:       "Deployment",
			Name:       "app-2",
			APIVersion: "apps/v1",
		},
		// Remote owner means .Spec source of truth is Datadog App
		Owner: datadoghqcommon.DatadogPodAutoscalerRemoteOwner,
	}

	dpaInternal := model.FakePodAutoscalerInternal{
		Namespace: "default",
		Name:      "dpa-0",
		Spec:      &dpaSpec,
	}
	f.store.Set("default/dpa-0", dpaInternal.Build(), controllerID)

	dpaInternal1 := model.FakePodAutoscalerInternal{
		Namespace: "default",
		Name:      "dpa-1",
		Spec:      &dpa1Spec,
	}
	f.store.Set("default/dpa-1", dpaInternal1.Build(), controllerID)

	dpaInternal2 := model.FakePodAutoscalerInternal{
		Namespace: "default",
		Name:      "dpa-2",
		Spec:      &dpa2Spec,
	}
	f.store.Set("default/dpa-2", dpaInternal2.Build(), controllerID)

	// Should create object in Kubernetes
	expectedStatus := datadoghqcommon.DatadogPodAutoscalerStatus{
		Conditions: []datadoghqcommon.DatadogPodAutoscalerCondition{
			condition(datadoghqcommon.DatadogPodAutoscalerErrorCondition, corev1.ConditionFalse, "", "", testTime),
			condition(datadoghqcommon.DatadogPodAutoscalerActiveCondition, corev1.ConditionTrue, "", "", testTime),
			condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToRecommendCondition, corev1.ConditionUnknown, "", "", testTime),
			condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToRecommendCondition, corev1.ConditionUnknown, "", "", testTime),
			condition(datadoghqcommon.DatadogPodAutoscalerHorizontalScalingLimitedCondition, corev1.ConditionFalse, "", "", testTime),
			condition(datadoghqcommon.DatadogPodAutoscalerVerticalScalingLimitedCondition, corev1.ConditionFalse, "", "", testTime),
			condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToScaleCondition, corev1.ConditionUnknown, "", "", testTime),
			condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToApply, corev1.ConditionUnknown, "", "", testTime),
		},
	}
	expectedUnstructured, _ := newFakePodAutoscaler("default", "dpa-0", -1, time.Time{}, dpaSpec, expectedStatus)
	f.ExpectCreateAction(expectedUnstructured)
	f.RunControllerSync(true, "default/dpa-0")

	expectedUnstructured1, _ := newFakePodAutoscaler("default", "dpa-1", -1, time.Time{}, dpa1Spec, expectedStatus)
	f.Actions = nil
	f.ExpectCreateAction(expectedUnstructured1)
	f.RunControllerSync(true, "default/dpa-1")

	expectedUnstructured2, _ := newFakePodAutoscaler("default", "dpa-2", -1, time.Time{}, dpa2Spec, expectedStatus)
	f.Actions = nil
	f.ExpectCreateAction(expectedUnstructured2)
	f.RunControllerSync(true, "default/dpa-2")
	assert.Len(t, f.store.GetAll(), 3)

	dpaTime := testTime.Add(-1 * time.Hour)
	dpa1Time := testTime
	dpa2Time := testTime.Add(1 * time.Hour)

	dpa, dpaTyped := newFakePodAutoscaler("default", "dpa-0", 1, dpaTime, dpaSpec, expectedStatus)
	dpa1, dpaTyped1 := newFakePodAutoscaler("default", "dpa-1", 1, dpa1Time, dpa1Spec, expectedStatus)
	dpa2, dpaTyped2 := newFakePodAutoscaler("default", "dpa-2", 1, dpa2Time, dpa2Spec, expectedStatus)

	// Setup scaler mock to handle any get calls during concurrent processing
	f.scaler.On("get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&autoscalingv1.Scale{}, schema.GroupResource{}, nil).Maybe()

	f.Actions = nil
	f.InformerObjects = append(f.InformerObjects, dpa, dpa1, dpa2)
	f.Objects = append(f.Objects, dpaTyped, dpaTyped1, dpaTyped2)

	// DPA object with exceeds max autoscalers error
	expectedDPAError := &datadoghq.DatadogPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogPodAutoscaler",
			APIVersion: "datadoghq.com/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "dpa-1",
			Namespace:         "default",
			Generation:        1,
			UID:               dpa1.GetUID(),
			CreationTimestamp: dpa1.GetCreationTimestamp(),
		},
		Spec: datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "",
				Name:       "",
				APIVersion: "",
			},
			Owner: "",
		},
		Status: datadoghqcommon.DatadogPodAutoscalerStatus{
			Conditions: []datadoghqcommon.DatadogPodAutoscalerCondition{
				condition(datadoghqcommon.DatadogPodAutoscalerErrorCondition, corev1.ConditionTrue, "ClusterAutoscalerLimitReached", fmt.Sprintf("Autoscaler disabled as maximum number per cluster reached (%d)", testMaxAutoscalerObjects), testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerActiveCondition, corev1.ConditionTrue, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToRecommendCondition, corev1.ConditionUnknown, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToRecommendCondition, corev1.ConditionUnknown, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalScalingLimitedCondition, corev1.ConditionFalse, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalScalingLimitedCondition, corev1.ConditionFalse, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToScaleCondition, corev1.ConditionUnknown, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToApply, corev1.ConditionUnknown, "", "", testTime),
			},
		},
	}
	// dpa1, dpaTyped1 = newFakePodAutoscaler("default", "dpa-1", 1, dpa1Time, dpa1Spec, errorStatus)
	f.ExpectUpdateStatusAction(mustUnstructured(t, expectedDPAError))
	f.RunControllerSync(true, "default/dpa-1")
	// f.RunControllerSync(true, "default/dpa-1")
	assert.Equal(t, 1, f.autoscalingHeap.MaxHeap.Len())
	assert.Equal(t, "default/dpa-1", f.autoscalingHeap.MaxHeap.Peek().Key)
	assert.Truef(t, f.autoscalingHeap.Keys["default/dpa-1"], "Expected dpa-1 to be in heap")

	// Check that multiple objects can be inserted with ordering preserved
	f.Actions = nil
	expectedDPAError.CreationTimestamp = dpa2.GetCreationTimestamp()
	expectedDPAError.Name = "dpa-2"
	expectedDPAError.UID = dpa2.GetUID()
	f.ExpectUpdateStatusAction(mustUnstructured(t, expectedDPAError))

	f.RunControllerSync(true, "default/dpa-2")
	assert.Equal(t, 2, f.autoscalingHeap.MaxHeap.Len())
	assert.Equal(t, "default/dpa-2", f.autoscalingHeap.MaxHeap.Peek().Key)
	assert.Truef(t, f.autoscalingHeap.Keys["default/dpa-1"], "Expected dpa-1 to be in heap")
	assert.Truef(t, f.autoscalingHeap.Keys["default/dpa-2"], "Expected dpa-2 to be in heap")

	// Check that heap ordering is preserved and limit is not exceeeded
	f.Actions = nil
	expectedDPAError.CreationTimestamp = dpa.GetCreationTimestamp()
	expectedDPAError.Name = "dpa-0"
	expectedDPAError.UID = dpa.GetUID()
	f.ExpectUpdateStatusAction(mustUnstructured(t, expectedDPAError))

	f.RunControllerSync(true, "default/dpa-0")
	assert.Equal(t, 2, f.autoscalingHeap.MaxHeap.Len())
	assert.Equal(t, "default/dpa-1", f.autoscalingHeap.MaxHeap.Peek().Key)
	assert.Truef(t, f.autoscalingHeap.Keys["default/dpa-0"], "Expected dpa-0 to be in heap")
	assert.Truef(t, f.autoscalingHeap.Keys["default/dpa-1"], "Expected dpa-1 to be in heap")
	assert.Falsef(t, f.autoscalingHeap.Keys["default/dpa-2"], "Expected dpa-2 to not be in heap")

	// Check that when object (dpa1) is deleted, heap is updated accordingly
	dpaInternal1.Deleted = true
	f.store.Set("default/dpa-1", dpaInternal1.Build(), controllerID)
	f.Actions = nil
	f.ExpectDeleteAction("default", "dpa-1")
	f.RunControllerSync(true, "default/dpa-1")
	assert.Len(t, f.store.GetAll(), 3)

	f.InformerObjects = nil
	f.Objects = nil
	f.Actions = nil

	f.RunControllerSync(true, "default/dpa-1")

	f.Actions = nil
	expectedDPAError.CreationTimestamp = dpa2.GetCreationTimestamp()
	expectedDPAError.Name = "dpa-2"
	expectedDPAError.UID = dpa2.GetUID()
	f.InformerObjects = append(f.InformerObjects, mustUnstructured(t, expectedDPAError))
	f.Objects = append(f.Objects, expectedDPAError)

	f.RunControllerSync(true, "default/dpa-2")
	assert.Len(t, f.store.GetAll(), 2)
	assert.Truef(t, f.autoscalingHeap.Keys["default/dpa-0"], "Expected dpa-0 to be in heap")
	assert.Falsef(t, f.autoscalingHeap.Keys["default/dpa-1"], "Expected dpa-1 to not be in heap")
	assert.Truef(t, f.autoscalingHeap.Keys["default/dpa-2"], "Expected dpa-2 to be in heap")
}

func TestIsTimestampStale(t *testing.T) {
	currentTime := time.Now()
	receivedTime := currentTime.Add(-1 * time.Minute)

	// no fallback policy, use default stale timestamp threshold
	assert.False(t, isTimestampStale(currentTime, receivedTime, defaultStaleTimestampThreshold))
	receivedTime = currentTime.Add(-1 * time.Minute * 31)
	assert.True(t, isTimestampStale(currentTime, receivedTime, defaultStaleTimestampThreshold))

	// fallback policy with stale recommendation threshold
	staleTimestampThreshold := time.Second * 120
	receivedTime = currentTime.Add(-1 * time.Minute)
	assert.False(t, isTimestampStale(currentTime, receivedTime, staleTimestampThreshold))
	receivedTime = currentTime.Add(-1 * time.Minute * 2)
	assert.False(t, isTimestampStale(currentTime, receivedTime, staleTimestampThreshold))
	receivedTime = currentTime.Add(-1 * time.Minute * 3)
	assert.True(t, isTimestampStale(currentTime, receivedTime, staleTimestampThreshold))
}

func TestGetActiveScalingSources(t *testing.T) {
	currentTime := time.Now()
	tests := []struct {
		name                  string
		podAutoscalerInternal model.FakePodAutoscalerInternal
		wantHorizontalSource  *datadoghqcommon.DatadogPodAutoscalerValueSource
		wantVerticalSource    *datadoghqcommon.DatadogPodAutoscalerValueSource
	}{
		{
			name: "horizontal, vertical scaling values are available",
			podAutoscalerInternal: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "dpa-0",
				Spec:      &datadoghq.DatadogPodAutoscalerSpec{},
				MainScalingValues: model.ScalingValues{
					Horizontal: &model.HorizontalScalingValues{
						Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
						Timestamp: currentTime,
					},
					Vertical: &model.VerticalScalingValues{
						Source: datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
					},
				},
			},
			wantHorizontalSource: pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource),
			wantVerticalSource:   pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource),
		},
		{
			name: "horizontal scaling is disabled, vertical scaling values are available",
			podAutoscalerInternal: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "dpa-0",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
						ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
							Strategy: pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerDisabledStrategySelect),
						},
						ScaleDown: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
							Strategy: pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerDisabledStrategySelect),
						},
					},
				},
				MainScalingValues: model.ScalingValues{
					Horizontal: &model.HorizontalScalingValues{
						Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
						Timestamp: currentTime,
					},
					Vertical: &model.VerticalScalingValues{
						Source: datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
					},
				},
			},
			wantHorizontalSource: pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource),
			wantVerticalSource:   pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource),
		},
		{
			name: "horizontal scaling values are in error, vertical scaling values are available",
			podAutoscalerInternal: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "dpa-0",
				Spec:      &datadoghq.DatadogPodAutoscalerSpec{},
				MainScalingValues: model.ScalingValues{
					Horizontal:      nil,
					HorizontalError: errors.New("test horizontal error"),
					Vertical: &model.VerticalScalingValues{
						Source: datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
					},
				},
			},
			wantHorizontalSource: nil,
			wantVerticalSource:   pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource),
		},
		{
			name: "horizontal scaling values are stale, fallback values are available",
			podAutoscalerInternal: model.FakePodAutoscalerInternal{
				Namespace:         "default",
				Name:              "dpa-0",
				Spec:              &datadoghq.DatadogPodAutoscalerSpec{},
				CreationTimestamp: currentTime.Add(-60 * time.Minute),
				MainScalingValues: model.ScalingValues{
					Horizontal: &model.HorizontalScalingValues{
						Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
						Timestamp: currentTime.Add(-31 * time.Minute),
					},
					Vertical: nil,
				},
				FallbackScalingValues: model.ScalingValues{
					Horizontal: &model.HorizontalScalingValues{
						Source:    datadoghqcommon.DatadogPodAutoscalerLocalValueSource,
						Timestamp: currentTime.Add(-30 * time.Second),
					},
					Vertical: nil,
				},
			},
			wantHorizontalSource: pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerLocalValueSource),
			wantVerticalSource:   nil,
		},
		{
			name: "no main horizontal values, current scaling values are stale, dpa is not new, fallback values are available",
			podAutoscalerInternal: model.FakePodAutoscalerInternal{
				Namespace:         "default",
				Name:              "dpa-0",
				Spec:              &datadoghq.DatadogPodAutoscalerSpec{},
				CreationTimestamp: currentTime.Add(-60 * time.Minute),
				ScalingValues: model.ScalingValues{
					Horizontal: &model.HorizontalScalingValues{
						Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
						Timestamp: currentTime.Add(-31 * time.Minute),
					},
					Vertical: nil,
				},
				MainScalingValues: model.ScalingValues{
					Horizontal: nil,
					Vertical:   nil,
				},
				FallbackScalingValues: model.ScalingValues{
					Horizontal: &model.HorizontalScalingValues{
						Source:    datadoghqcommon.DatadogPodAutoscalerLocalValueSource,
						Timestamp: currentTime.Add(-30 * time.Second),
					},
					Vertical: nil,
				},
			},
			wantHorizontalSource: pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerLocalValueSource),
			wantVerticalSource:   nil,
		},
		{
			name: "no horizontal values are available, dpa is not new, fallback values are available",
			podAutoscalerInternal: model.FakePodAutoscalerInternal{
				Namespace:         "default",
				Name:              "dpa-0",
				Spec:              &datadoghq.DatadogPodAutoscalerSpec{},
				CreationTimestamp: currentTime.Add(-60 * time.Minute),
				ScalingValues: model.ScalingValues{
					Horizontal: nil,
					Vertical:   nil,
				},
				MainScalingValues: model.ScalingValues{
					Horizontal: nil,
					Vertical:   nil,
				},
				FallbackScalingValues: model.ScalingValues{
					Horizontal: &model.HorizontalScalingValues{
						Source:    datadoghqcommon.DatadogPodAutoscalerLocalValueSource,
						Timestamp: currentTime.Add(-30 * time.Second),
					},
					Vertical: nil,
				},
			},
			wantHorizontalSource: pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerLocalValueSource),
			wantVerticalSource:   nil,
		},
		{
			name: "horizontal scaling values are stale, fallback values are stale",
			podAutoscalerInternal: model.FakePodAutoscalerInternal{
				Namespace:         "default",
				Name:              "dpa-0",
				Spec:              &datadoghq.DatadogPodAutoscalerSpec{},
				CreationTimestamp: currentTime.Add(-60 * time.Minute),
				MainScalingValues: model.ScalingValues{
					Horizontal: &model.HorizontalScalingValues{
						Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
						Timestamp: currentTime.Add(-31 * time.Minute),
					},
					Vertical: nil,
				},
				FallbackScalingValues: model.ScalingValues{
					Horizontal: &model.HorizontalScalingValues{
						Source:    datadoghqcommon.DatadogPodAutoscalerLocalValueSource,
						Timestamp: currentTime.Add(-31 * time.Minute),
					},
					Vertical: nil,
				},
			},
			wantHorizontalSource: nil,
			wantVerticalSource:   nil,
		},
		{
			name: "new autoscaler, no scaling values",
			podAutoscalerInternal: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "dpa-0",
				Spec:      &datadoghq.DatadogPodAutoscalerSpec{},
				MainScalingValues: model.ScalingValues{
					Horizontal: nil,
					Vertical:   nil,
				},
			},
			wantHorizontalSource: nil,
			wantVerticalSource:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dpai := tt.podAutoscalerInternal.Build()
			horizontalSource, verticalSource := getActiveScalingSources(currentTime, &dpai)
			assert.Equal(t, tt.wantHorizontalSource, horizontalSource)
			assert.Equal(t, tt.wantVerticalSource, verticalSource)
		})
	}
}

// TestVerticalConstraintsIdempotent is an end-to-end controller test verifying that when
// vertical constraints clamp a recommendation, the second reconcile does NOT produce
// a different status. If it did, updatePodAutoscalerStatus would call UpdateStatus on
// every sync, causing an infinite reconcile loop.
func TestVerticalConstraintsIdempotent(t *testing.T) {
	testTime := time.Now()
	f := newFixture(t, testTime)

	// Original (unconstrained) recommendation: CPU request=50m, limit=80m.
	// Constraint: MinAllowed CPU=200m → after clamping: request=200m, limit raised to 200m.
	constraints := &datadoghqcommon.DatadogPodAutoscalerConstraints{
		Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
			{
				Name:       "app",
				MinAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m")},
			},
		},
	}

	constrainedResources := []datadoghqcommon.DatadogPodAutoscalerContainerResources{
		{
			Name:     "app",
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m")},
			Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m")},
		},
	}
	constrainedHash, err := autoscaling.ObjectHash(constrainedResources)
	require.NoError(t, err)

	dpaSpec := datadoghq.DatadogPodAutoscalerSpec{
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			Kind:       "Deployment",
			Name:       "app-0",
			APIVersion: "apps/v1",
		},
		Owner:       datadoghqcommon.DatadogPodAutoscalerLocalOwner,
		Constraints: constraints,
	}

	dpa, dpaTyped := newFakePodAutoscaler("default", "dpa-0", 1, testTime, dpaSpec, datadoghqcommon.DatadogPodAutoscalerStatus{})

	dpaInternal := model.FakePodAutoscalerInternal{
		Namespace:         "default",
		Name:              "dpa-0",
		Generation:        1,
		CreationTimestamp: testTime,
		Spec:              &dpaSpec,
		MainScalingValues: model.ScalingValues{
			Vertical: &model.VerticalScalingValues{
				Source:        datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp:     testTime,
				ResourcesHash: "original-hash",
				ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
					{
						Name:     "app",
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m")},
						Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("80m")},
					},
				},
			},
		},
	}
	f.store.Set("default/dpa-0", dpaInternal.Build(), controllerID)

	// Pods already on the constrained hash (steady state after first patch).
	f.podWatcher.mockGetPodsForOwner(NamespacedPodOwner{
		Namespace: "default",
		Kind:      "Deployment",
		Name:      "app-0",
	}, []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "pod-1", Namespace: "default",
				Annotations: map[string]string{model.RecommendationIDAnnotation: constrainedHash},
			},
			Owners: []workloadmeta.KubernetesPodOwner{{Kind: "ReplicaSet", Name: "app-0-rs1", ID: "app-0-rs1"}},
		},
	})

	// Horizontal controller needs the scaler mock even if there are no horizontal recs.
	f.scaler.On("get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		&autoscalingv1.Scale{Spec: autoscalingv1.ScaleSpec{Replicas: 1}, Status: autoscalingv1.ScaleStatus{Replicas: 1}},
		schema.GroupResource{}, nil,
	).Maybe()

	cpuReqSum, memReqSum := (&model.VerticalScalingValues{ContainerResources: constrainedResources}).SumCPUMemoryRequests()

	// First sync: status goes from empty to populated → UpdateStatus expected.
	f.InformerObjects = []*unstructured.Unstructured{dpa}
	f.Objects = []runtime.Object{dpaTyped}

	expectedDPA := &datadoghq.DatadogPodAutoscaler{
		TypeMeta: metav1.TypeMeta{Kind: "DatadogPodAutoscaler", APIVersion: "datadoghq.com/v1alpha2"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "dpa-0", Namespace: "default",
			Generation: 1, UID: dpa.GetUID(), CreationTimestamp: metav1.NewTime(testTime),
		},
		Spec: datadoghq.DatadogPodAutoscalerSpec{},
		Status: datadoghqcommon.DatadogPodAutoscalerStatus{
			CurrentReplicas: pointer.Ptr[int32](1),
			Vertical: &datadoghqcommon.DatadogPodAutoscalerVerticalStatus{
				Target: &datadoghqcommon.DatadogPodAutoscalerVerticalTargetStatus{
					Source:           datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
					GeneratedAt:      metav1.NewTime(testTime),
					Version:          constrainedHash,
					DesiredResources: constrainedResources,
					PodCPURequest:    cpuReqSum,
					PodMemoryRequest: memReqSum,
					Scaled:           pointer.Ptr[int32](1),
				},
			},
			Conditions: []datadoghqcommon.DatadogPodAutoscalerCondition{
				condition(datadoghqcommon.DatadogPodAutoscalerErrorCondition, corev1.ConditionFalse, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerActiveCondition, corev1.ConditionTrue, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToRecommendCondition, corev1.ConditionUnknown, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToRecommendCondition, corev1.ConditionTrue, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalScalingLimitedCondition, corev1.ConditionFalse, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalScalingLimitedCondition, corev1.ConditionTrue, "LimitedByConstraint", "recommendation clamped to min/max bounds for containers: app", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToScaleCondition, corev1.ConditionUnknown, "", "", testTime),
				condition(datadoghqcommon.DatadogPodAutoscalerVerticalAbleToApply, corev1.ConditionUnknown, "", "", testTime),
			},
		},
	}
	f.ExpectUpdateStatusAction(mustUnstructured(t, expectedDPA))
	f.RunControllerSync(true, "default/dpa-0")

	// Second sync: feed back the status from the first sync into the DPA object.
	// The controller must see no status diff → no UpdateStatus call → no spurious reconcile.
	dpaTyped.Status = expectedDPA.Status
	f.InformerObjects = []*unstructured.Unstructured{mustUnstructured(t, dpaTyped)}
	f.Objects = []runtime.Object{dpaTyped}
	f.Actions = nil // expect zero actions

	f.RunControllerSync(true, "default/dpa-0")
}

func mustUnstructured(t *testing.T, structIn any) *unstructured.Unstructured {
	unstructOut, err := autoscaling.ToUnstructured(structIn)
	require.NoError(t, err)
	return unstructOut
}

func condition(conditionType datadoghqcommon.DatadogPodAutoscalerConditionType, status corev1.ConditionStatus, reason, message string, transitionTime time.Time) datadoghqcommon.DatadogPodAutoscalerCondition {
	return datadoghqcommon.DatadogPodAutoscalerCondition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(transitionTime),
	}
}
