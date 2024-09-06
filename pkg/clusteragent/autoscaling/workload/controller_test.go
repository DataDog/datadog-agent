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
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/record"
	clock "k8s.io/utils/clock/testing"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
)

type fixture struct {
	*autoscaling.ControllerFixture

	clock           *clock.FakeClock
	recorder        *record.FakeRecorder
	store           *store
	autoscalingHeap *autoscaling.HashHeap
}

const testMaxAutoscalerObjects int = 2

func newFixture(t *testing.T, testTime time.Time) *fixture {
	store := autoscaling.NewStore[model.PodAutoscalerInternal]()
	clock := clock.NewFakeClock(testTime)
	recorder := record.NewFakeRecorder(100)
	hashHeap := autoscaling.NewHashHeap(testMaxAutoscalerObjects)
	return &fixture{
		ControllerFixture: autoscaling.NewFixture(
			t, podAutoscalerGVR,
			func(fakeClient *fake.FakeDynamicClient, informer dynamicinformer.DynamicSharedInformerFactory, isLeader func() bool) (*autoscaling.Controller, error) {
				c, err := newController("cluster-id1", recorder, nil, nil, fakeClient, informer, isLeader, store, nil, nil, hashHeap)
				if err != nil {
					return nil, err
				}

				c.clock = clock
				c.horizontalController = &horizontalController{
					scaler: newFakeScaler(),
				}
				return c.Controller, err
			},
		),
		clock:           clock,
		recorder:        recorder,
		store:           store,
		autoscalingHeap: hashHeap,
	}
}

func newFakePodAutoscaler(ns, name string, gen int64, creationTimestamp time.Time, spec datadoghq.DatadogPodAutoscalerSpec, status datadoghq.DatadogPodAutoscalerStatus) (obj *unstructured.Unstructured, dpa *datadoghq.DatadogPodAutoscaler) {
	dpa = &datadoghq.DatadogPodAutoscaler{
		TypeMeta: podAutoscalerMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			Generation:        gen,
			UID:               uuid.NewUUID(),
			CreationTimestamp: metav1.NewTime(creationTimestamp),
		},
		Spec:   spec,
		Status: status,
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
		Owner: datadoghq.DatadogPodAutoscalerLocalOwner,
	}

	defaultCreationTime, _ := time.Parse("YYYY-MM-DD 00:00:00 +0000 UTC", "0001-01-01 00:00:00 +0000 UTC")
	// Read newly created DPA
	dpa, dpaTyped := newFakePodAutoscaler("default", "dpa-0", 1, defaultCreationTime, dpaSpec, datadoghq.DatadogPodAutoscalerStatus{})

	f.InformerObjects = append(f.InformerObjects, dpa)
	f.Objects = append(f.Objects, dpaTyped)

	f.RunControllerSync(true, "default/dpa-0")

	// Check internal store content
	expectedDPAInternal := model.FakePodAutoscalerInternal{
		Namespace:  "default",
		Name:       "dpa-0",
		Generation: 1,
		Spec:       &dpaSpec,
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
		Owner: datadoghq.DatadogPodAutoscalerRemoteOwner,
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
			APIVersion: "datadoghq.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dpa-0",
			Namespace: "default",
		},
		Spec: dpaSpec,
		Status: datadoghq.DatadogPodAutoscalerStatus{
			Conditions: []datadoghq.DatadogPodAutoscalerCondition{
				{
					Type:               datadoghq.DatadogPodAutoscalerErrorCondition,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: metav1.NewTime(testTime),
				},
				{
					Type:               datadoghq.DatadogPodAutoscalerActiveCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(testTime),
				},
				{
					Type:               datadoghq.DatadogPodAutoscalerHorizontalAbleToRecommendCondition,
					Status:             corev1.ConditionUnknown,
					LastTransitionTime: metav1.NewTime(testTime),
				},
				{
					Type:               datadoghq.DatadogPodAutoscalerVerticalAbleToRecommendCondition,
					Status:             corev1.ConditionUnknown,
					LastTransitionTime: metav1.NewTime(testTime),
				},
				{
					Type:               datadoghq.DatadogPodAutoscalerHorizontalScalingLimitedCondition,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: metav1.NewTime(testTime),
				},
				{
					Type:               datadoghq.DatadogPodAutoscalerHorizontalAbleToScaleCondition,
					Status:             corev1.ConditionUnknown,
					LastTransitionTime: metav1.NewTime(testTime),
				},
				{
					Type:               datadoghq.DatadogPodAutoscalerVerticalAbleToApply,
					Status:             corev1.ConditionUnknown,
					LastTransitionTime: metav1.NewTime(testTime),
				},
			},
		},
	}
	expectedUnstructured, err := autoscaling.ToUnstructured(expectedDPA)
	assert.NoError(t, err)
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

func TestLeaderCreateDeleteLocalHeap(t *testing.T) {
	testTime := time.Now()
	f := newFixture(t, testTime)

	dpaSpec := datadoghq.DatadogPodAutoscalerSpec{
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			Kind:       "Deployment",
			Name:       "app-0",
			APIVersion: "apps/v1",
		},
		// Local owner means .Spec source of truth is K8S
		Owner: datadoghq.DatadogPodAutoscalerLocalOwner,
	}

	// Read newly created DPA
	dpa, dpaTyped := newFakePodAutoscaler("default", "dpa-0", 1, time.Now().Add(-1*time.Hour), dpaSpec, datadoghq.DatadogPodAutoscalerStatus{})
	dpa1, dpaTyped1 := newFakePodAutoscaler("default", "dpa-1", 1, time.Now(), dpaSpec, datadoghq.DatadogPodAutoscalerStatus{})
	dpa2, dpaTyped2 := newFakePodAutoscaler("default", "dpa-2", 1, time.Now().Add(1*time.Hour), dpaSpec, datadoghq.DatadogPodAutoscalerStatus{})

	f.InformerObjects = append(f.InformerObjects, dpa)
	f.Objects = append(f.Objects, dpaTyped)

	f.InformerObjects = append(f.InformerObjects, dpa1)
	f.Objects = append(f.Objects, dpaTyped1)
	f.RunControllerSync(true, "default/dpa-1")
	// Check that DatadogPodAutoscaler object is inserted into heap
	assert.Equal(t, 1, f.autoscalingHeap.MaxHeap.Len())
	assert.Equal(t, "default/dpa-1", f.autoscalingHeap.MaxHeap.Peek().Key)
	assert.Truef(t, f.autoscalingHeap.Keys["default/dpa-1"], "Expected dpa-1 to be in heap")

	f.InformerObjects = append(f.InformerObjects, dpa2)
	f.Objects = append(f.Objects, dpaTyped2)
	// Check that multiple objects can be inserted with ordering preserved
	f.RunControllerSync(true, "default/dpa-2")
	assert.Equal(t, 2, f.autoscalingHeap.MaxHeap.Len())
	assert.Equal(t, "default/dpa-2", f.autoscalingHeap.MaxHeap.Peek().Key)
	assert.Truef(t, f.autoscalingHeap.Keys["default/dpa-1"], "Expected dpa-1 to be in heap")
	assert.Truef(t, f.autoscalingHeap.Keys["default/dpa-2"], "Expected dpa-2 to be in heap")

	f.RunControllerSync(true, "default/dpa-0")
	// Check that heap ordering is preserved and limit is not exceeeded
	assert.Equal(t, 2, f.autoscalingHeap.MaxHeap.Len())
	assert.Equal(t, "default/dpa-1", f.autoscalingHeap.MaxHeap.Peek().Key)
	assert.Truef(t, f.autoscalingHeap.Keys["default/dpa-0"], "Expected dpa-0 to be in heap")
	assert.Truef(t, f.autoscalingHeap.Keys["default/dpa-1"], "Expected dpa-1 to be in heap")
	assert.Falsef(t, f.autoscalingHeap.Keys["default/dpa-2"], "Expected dpa-2 to not be in heap")
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
			currentNs := common.GetMyNamespace()
			id := fmt.Sprintf("%s/dpa-dca", currentNs)

			dpaSpec := datadoghq.DatadogPodAutoscalerSpec{
				TargetRef: tt.targetRef,
				// Local owner means .Spec source of truth is K8S
				Owner: datadoghq.DatadogPodAutoscalerLocalOwner,
			}

			dpa, dpaTyped := newFakePodAutoscaler(currentNs, "dpa-dca", 1, dpaSpec, datadoghq.DatadogPodAutoscalerStatus{})
			f.InformerObjects = append(f.InformerObjects, dpa)

			expectedDPAError := &datadoghq.DatadogPodAutoscaler{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DatadogPodAutoscaler",
					APIVersion: "datadoghq.com/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:       "dpa-dca",
					Namespace:  currentNs,
					Generation: 1,
					UID:        dpa.GetUID(),
				},
				Spec: datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "",
						Name:       "",
						APIVersion: "",
					},
					Owner: "",
				},
				Status: datadoghq.DatadogPodAutoscalerStatus{
					Conditions: []datadoghq.DatadogPodAutoscalerCondition{
						{
							Type:               datadoghq.DatadogPodAutoscalerErrorCondition,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: metav1.NewTime(testTime),
							Reason:             "Autoscaling target cannot be set to the cluster agent",
						},
						{
							Type:               datadoghq.DatadogPodAutoscalerActiveCondition,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: metav1.NewTime(testTime),
						},
						{
							Type:               datadoghq.DatadogPodAutoscalerHorizontalAbleToRecommendCondition,
							Status:             corev1.ConditionUnknown,
							LastTransitionTime: metav1.NewTime(testTime),
						},
						{
							Type:               datadoghq.DatadogPodAutoscalerVerticalAbleToRecommendCondition,
							Status:             corev1.ConditionUnknown,
							LastTransitionTime: metav1.NewTime(testTime),
						},
						{
							Type:               datadoghq.DatadogPodAutoscalerHorizontalScalingLimitedCondition,
							Status:             corev1.ConditionFalse,
							LastTransitionTime: metav1.NewTime(testTime),
						},
						{
							Type:               datadoghq.DatadogPodAutoscalerHorizontalAbleToScaleCondition,
							Status:             corev1.ConditionUnknown,
							LastTransitionTime: metav1.NewTime(testTime),
						},
						{
							Type:               datadoghq.DatadogPodAutoscalerVerticalAbleToApply,
							Status:             corev1.ConditionUnknown,
							LastTransitionTime: metav1.NewTime(testTime),
						},
					},
				},
			}
			expectedUnstructuredError, err := autoscaling.ToUnstructured(expectedDPAError)
			assert.NoError(t, err)
			f.RunControllerSync(true, id)

			f.Objects = append(f.Objects, dpaTyped)
			f.Actions = nil

			f.ExpectUpdateStatusAction(expectedUnstructuredError)
			f.RunControllerSync(true, id)
			assert.Len(t, f.store.GetAll(), 1)
			pai, found := f.store.Get(id)
			assert.Truef(t, found, "Expected to find DatadogPodAutoscaler in store")
			assert.Equal(t, errors.New("Autoscaling target cannot be set to the cluster agent"), pai.Error())
		})
	}
}
