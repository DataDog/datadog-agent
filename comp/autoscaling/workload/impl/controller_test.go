// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package impl

import (
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

	"github.com/DataDog/datadog-agent/comp/autoscaling/workload/impl/model"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
)

type fixture struct {
	*autoscaling.ControllerFixture

	clock    *clock.FakeClock
	recorder *record.FakeRecorder
	store    *store
}

func newFixture(t *testing.T, testTime time.Time) *fixture {
	store := autoscaling.NewStore[model.PodAutoscalerInternal]()
	clock := clock.NewFakeClock(testTime)
	recorder := record.NewFakeRecorder(100)
	return &fixture{
		ControllerFixture: autoscaling.NewFixture(
			t, podAutoscalerGVR,
			func(fakeClient *fake.FakeDynamicClient, informer dynamicinformer.DynamicSharedInformerFactory, isLeader func() bool) (*autoscaling.Controller, error) {
				c, err := newController(recorder, nil, nil, fakeClient, informer, isLeader, store, nil)
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
		clock:    clock,
		recorder: recorder,
		store:    store,
	}
}

func newFakePodAutoscaler(ns, name string, gen int64, spec datadoghq.DatadogPodAutoscalerSpec, status datadoghq.DatadogPodAutoscalerStatus) (obj *unstructured.Unstructured, dpa *datadoghq.DatadogPodAutoscaler) {
	dpa = &datadoghq.DatadogPodAutoscaler{
		TypeMeta: podAutoscalerMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  ns,
			Generation: gen,
			UID:        uuid.NewUUID(),
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

	// Read newly created DPA
	dpa, dpaTyped := newFakePodAutoscaler("default", "dpa-0", 1, dpaSpec, datadoghq.DatadogPodAutoscalerStatus{})

	f.InformerObjects = append(f.InformerObjects, dpa)
	f.Objects = append(f.Objects, dpaTyped)

	f.RunControllerSync(true, "default/dpa-0")

	// Check internal store content
	expectedDPAInternal := model.PodAutoscalerInternal{
		Namespace:  "default",
		Name:       "dpa-0",
		Generation: 1,
		Spec:       &dpaSpec,
	}
	dpaInternal, found := f.store.Get("default/dpa-0")
	assert.True(t, found)
	assert.Equal(t, expectedDPAInternal, dpaInternal)

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
	assert.Equal(t, expectedDPAInternal, dpaInternal)
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

	dpaInternal := model.PodAutoscalerInternal{
		Namespace: "default",
		Name:      "dpa-0",
		Spec:      &dpaSpec,
	}
	f.store.Set("default/dpa-0", dpaInternal, controllerID)

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
	f.store.Set("default/dpa-0", dpaInternal, controllerID)
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
