// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package externalmetrics

import (
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/dynamic/fake"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	core "k8s.io/client-go/testing"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	"github.com/stretchr/testify/assert"
)

var (
	scheme             = kscheme.Scheme
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

func init() {
	datadoghq.AddToScheme(scheme)
}

// Test fixture
type fixture struct {
	t *testing.T

	client *fake.FakeDynamicClient
	// Objects to put in the store.
	datadogMetricLister []*unstructured.Unstructured
	// Actions expected to happen on the client.
	actions []core.Action
	// Objects from here preloaded into Fake client.
	objects []runtime.Object
	// Local store
	store DatadogMetricsInternalStore
}

func newFixture(t *testing.T) *fixture {
	return &fixture{
		t:       t,
		objects: []runtime.Object{},
		store:   NewDatadogMetricsInternalStore(),
	}
}

func (f *fixture) newController(leader bool) (*DatadogMetricController, dynamicinformer.DynamicSharedInformerFactory) {
	f.client = fake.NewSimpleDynamicClient(scheme, f.objects...)
	informer := dynamicinformer.NewDynamicSharedInformerFactory(f.client, noResyncPeriodFunc())

	c, err := NewDatadogMetricController(f.client, informer, getIsLeaderFunction(leader), &f.store)
	if err != nil {
		return nil, nil
	}
	c.synced = alwaysReady

	for _, metric := range f.datadogMetricLister {
		informer.ForResource(gvrDDM).Informer().GetIndexer().Add(metric)
	}

	return c, informer
}

func (f *fixture) runControllerSync(leader bool, datadogMetricID string, expectedError error) {
	f.t.Helper()
	controller, informer := f.newController(leader)
	stopCh := make(chan struct{})
	defer close(stopCh)
	informer.Start(stopCh)

	err := controller.processDatadogMetric(datadogMetricID)
	assert.Equal(f.t, expectedError, err)

	actions := filterInformerActions(f.client.Actions(), "datadogmetrics")
	for i, action := range actions {
		if len(f.actions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(actions)-len(f.actions), actions[i:])
			break
		}

		expectedAction := f.actions[i]
		checkAction(f.t, expectedAction, action)
	}

	if len(f.actions) > len(actions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.actions)-len(actions), f.actions[len(actions):])
	}
}

func (f *fixture) expectCreateDatadogMetricAction(datadogMetric *unstructured.Unstructured) {
	action := core.NewCreateAction(gvrDDM, datadogMetric.GetNamespace(), datadogMetric)
	f.actions = append(f.actions, action)
}

func (f *fixture) expectDeleteDatadogMetricAction(ns, name string) {
	action := core.NewDeleteAction(gvrDDM, ns, name)
	f.actions = append(f.actions, action)
}

func (f *fixture) expectUpdateDatadogMetricStatusAction(datadogMetric *unstructured.Unstructured) {
	action := core.NewUpdateSubresourceAction(gvrDDM, "status", datadogMetric.GetNamespace(), datadogMetric)
	f.actions = append(f.actions, action)
}

func newFakeDatadogMetric(ns, name, query string, status datadoghq.DatadogMetricStatus) (obj *unstructured.Unstructured, ddm *datadoghq.DatadogMetric) {
	obj = &unstructured.Unstructured{}
	ddm = &datadoghq.DatadogMetric{
		TypeMeta: metaDDM,
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: datadoghq.DatadogMetricSpec{
			Query: query,
		},
		Status: status,
	}

	if err := UnstructuredFromDDM(ddm, obj); err != nil {
		panic("Failed to construct unstructured DDM")
	}

	return
}

// Scenario: user creates a new DatadogMetric `dd-metric-0`
// We check that a leader controller stores it locally. Test update if no changes happened before resync
func TestLeaderHandlingNewMetric(t *testing.T) {
	f := newFixture(t)
	metric, metricTyped := newFakeDatadogMetric("default", "dd-metric-0", "metric query0", datadoghq.DatadogMetricStatus{})
	testTime := time.Now().UTC()

	f.datadogMetricLister = append(f.datadogMetricLister, metric)
	f.objects = append(f.objects, metricTyped)

	f.runControllerSync(true, "default/dd-metric-0", nil)

	// Check internal store content
	ddm := model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Valid:      false,
		Value:      0,
		UpdateTime: testTime,
		Error:      nil,
	}
	ddm.SetQueries("metric query0")
	compareDatadogMetricInternal(t, &ddm, f.store.Get("default/dd-metric-0"))

	// Now that we validated that `UpdateTime` is after `testTime`, we read `UpdateTime` to allow comparison later
	updateTimeKube := metav1.NewTime(f.store.Get("default/dd-metric-0").UpdateTime)
	outputMetric, _ := newFakeDatadogMetric("default", "dd-metric-0", "", datadoghq.DatadogMetricStatus{
		Value: "0",
		Conditions: []datadoghq.DatadogMetricCondition{
			{
				Type:               datadoghq.DatadogMetricConditionTypeActive,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: updateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeValid,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: updateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
			{
				Type:   datadoghq.DatadogMetricConditionTypeUpdated,
				Status: corev1.ConditionTrue,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeError,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: updateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
		},
	})
	f.expectUpdateDatadogMetricStatusAction(outputMetric)
	f.runControllerSync(true, "default/dd-metric-0", nil)
}

// Scenario: A DatadogMetric has been updated by background process (first time)
// We check that we synchronize that back to K8S
func TestLeaderUpdateFromStoreInitialUpdate(t *testing.T) {
	f := newFixture(t)

	updateTime := time.Now()
	updateTimeKube := metav1.NewTime(updateTime)
	metric, metricTyped := newFakeDatadogMetric("default", "dd-metric-0", "metric query0", datadoghq.DatadogMetricStatus{})
	f.datadogMetricLister = append(f.datadogMetricLister, metric)
	f.objects = append(f.objects, metricTyped)
	ddm := model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Valid:      true,
		Value:      2332548489456.557505560,
		UpdateTime: updateTime,
		DataTime:   updateTime,
		Error:      nil,
	}
	ddm.SetQueries("metric query0")
	f.store.Set("default/dd-metric-0", ddm, "utest")

	outputMetric, _ := newFakeDatadogMetric("default", "dd-metric-0", "", datadoghq.DatadogMetricStatus{
		Value: "2332548489456.5576",
		Conditions: []datadoghq.DatadogMetricCondition{
			{
				Type:               datadoghq.DatadogMetricConditionTypeActive,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: updateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeValid,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: updateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeUpdated,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: updateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeError,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: updateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
		},
	})
	f.expectUpdateDatadogMetricStatusAction(outputMetric)
	f.runControllerSync(true, "default/dd-metric-0", nil)
}

// Scenario: A DatadogMetric has been updated by background process (after initial update) and metric we were not able to get data
// We check that we synchronize that back to K8S
func TestLeaderUpdateFromStoreAfterInitial(t *testing.T) {
	f := newFixture(t)

	prevUpdateTimeKube := metav1.NewTime(time.Now().Add(-10 * time.Second))
	metric, metricTyped := newFakeDatadogMetric("default", "dd-metric-0", "metric query0", datadoghq.DatadogMetricStatus{
		Value: "10",
		Conditions: []datadoghq.DatadogMetricCondition{
			{
				Type:               datadoghq.DatadogMetricConditionTypeActive,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeValid,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeUpdated,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeError,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
		},
	})

	updateTime := time.Now()
	updateTimeKube := metav1.NewTime(updateTime)
	f.datadogMetricLister = append(f.datadogMetricLister, metric)
	f.objects = append(f.objects, metricTyped)
	ddm := model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Valid:      true,
		Value:      10.0,
		UpdateTime: updateTime,
		DataTime:   updateTime,
		Error:      fmt.Errorf("Error from backend while fetching metric"),
	}
	ddm.SetQueries("metric query0")
	f.store.Set("default/dd-metric-0", ddm, "utest")

	outputMetric, _ := newFakeDatadogMetric("default", "dd-metric-0", "", datadoghq.DatadogMetricStatus{
		Value: "10",
		Conditions: []datadoghq.DatadogMetricCondition{
			{
				Type:               datadoghq.DatadogMetricConditionTypeActive,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeValid,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeUpdated,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeError,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: updateTimeKube,
				LastUpdateTime:     updateTimeKube,
				Reason:             model.DatadogMetricErrorConditionReason,
				Message:            "Error from backend while fetching metric",
			},
		},
	})
	f.expectUpdateDatadogMetricStatusAction(outputMetric)
	f.runControllerSync(true, "default/dd-metric-0", nil)
}

// Scenario: A resync happened without any update from background thread
// We check that no action is taken by leader
func TestLeaderNoUpdate(t *testing.T) {
	f := newFixture(t)

	updateTime := time.Now()
	updateTimeKube := metav1.NewTime(updateTime)
	metric, metricTyped := newFakeDatadogMetric("default", "dd-metric-0", "metric query0", datadoghq.DatadogMetricStatus{
		Value: "10",
		Conditions: []datadoghq.DatadogMetricCondition{
			{
				Type:               datadoghq.DatadogMetricConditionTypeActive,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: updateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeValid,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: updateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeUpdated,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: updateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeError,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: updateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
		},
	})

	f.datadogMetricLister = append(f.datadogMetricLister, metric)
	f.objects = append(f.objects, metricTyped)
	ddm := model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Valid:      true,
		Value:      10.0,
		UpdateTime: updateTime,
		DataTime:   updateTime,
		Error:      fmt.Errorf("Error from backend while fetching metric"),
	}
	ddm.SetQueries("metric query0")
	f.store.Set("default/dd-metric-0", ddm, "utest")

	f.runControllerSync(true, "default/dd-metric-0", nil)
}

func TestCreateDatadogMetric(t *testing.T) {
	f := newFixture(t)

	updateTime := time.Now()
	updateTimeKube := metav1.NewTime(updateTime)
	ddm := model.DatadogMetricInternal{
		ID:                 "default/dd-metric-0",
		Valid:              true,
		Deleted:            false,
		Autogen:            true,
		ExternalMetricName: "name1",
		Value:              20.0,
		UpdateTime:         updateTime,
		DataTime:           updateTime,
		Error:              nil,
	}
	ddm.SetQueries("metric query0")
	f.store.Set("default/dd-metric-0", ddm, "utest")

	ddm = model.DatadogMetricInternal{
		ID:         "default/dd-metric-1",
		Valid:      true,
		Deleted:    false,
		Autogen:    false,
		Value:      20.0,
		UpdateTime: updateTime,
		DataTime:   updateTime,
		Error:      nil,
	}
	ddm.SetQueries("metric query1")
	f.store.Set("default/dd-metric-1", ddm, "utest")

	ddm = model.DatadogMetricInternal{
		ID:         "default/dd-metric-1",
		Valid:      true,
		Deleted:    true,
		Autogen:    true,
		Value:      20.0,
		UpdateTime: updateTime,
		DataTime:   updateTime,
		Error:      nil,
	}
	ddm.SetQueries("metric query1")
	f.store.Set("default/dd-metric-1", ddm, "utest")

	ddm = model.DatadogMetricInternal{
		ID:         "default/dd-metric-2",
		Valid:      true,
		Deleted:    false,
		Autogen:    true,
		Value:      20.0,
		UpdateTime: updateTime,
		DataTime:   updateTime,
		Error:      nil,
	}
	ddm.SetQueries("metric query2")
	f.store.Set("default/dd-metric-2", ddm, "utest")

	// Test successful creation
	expectedDatadogMetric, _ := newFakeDatadogMetric("default", "dd-metric-0", "metric query0", datadoghq.DatadogMetricStatus{
		Value: "20",
		Conditions: []datadoghq.DatadogMetricCondition{
			{
				Type:               datadoghq.DatadogMetricConditionTypeActive,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: updateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeValid,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: updateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeUpdated,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: updateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeError,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: updateTimeKube,
				LastUpdateTime:     updateTimeKube,
			},
		},
	})
	unstructured.SetNestedField(expectedDatadogMetric.Object, "name1", "spec", "externalMetricName")
	f.expectCreateDatadogMetricAction(expectedDatadogMetric)
	f.runControllerSync(true, "default/dd-metric-0", nil)

	// Test that autogen missing in Kubernetes with `Deleted` flag is not created
	f.actions = nil
	f.runControllerSync(true, "default/dd-metric-1", nil)
	assert.Empty(t, f.actions)
	assert.Nil(t, f.store.Get("default/dd-metric-1"))

	// Test create autogen without ExternalMetricName
	f.actions = nil
	f.runControllerSync(true, "default/dd-metric-2", fmt.Errorf("Unable to create autogen DatadogMetric default/dd-metric-2 without ExternalMetricName"))
	assert.Empty(t, f.actions)
}

// Scenario: Test DatadogMetric is deleted if something from store is flagged as deleted and object exists in K8S
func TestLeaderDeleteExisting(t *testing.T) {
	f := newFixture(t)

	prevUpdateTime := time.Now().Add(-10 * time.Second)
	prevUpdateTimeKube := metav1.NewTime(prevUpdateTime)
	metric0, metric0Typed := newFakeDatadogMetric("default", "dd-metric-0", "metric query0", datadoghq.DatadogMetricStatus{
		Value: "20",
		Conditions: []datadoghq.DatadogMetricCondition{
			{
				Type:               datadoghq.DatadogMetricConditionTypeActive,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeValid,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeUpdated,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeError,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
		},
	})
	metric1, metric1Typed := newFakeDatadogMetric("default", "dd-metric-1", "metric query1", datadoghq.DatadogMetricStatus{
		Value: "20",
		Conditions: []datadoghq.DatadogMetricCondition{
			{
				Type:               datadoghq.DatadogMetricConditionTypeActive,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeValid,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeUpdated,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeError,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
		},
	})

	f.datadogMetricLister = append(f.datadogMetricLister, metric0, metric1)
	f.objects = append(f.objects, metric0Typed, metric1Typed)

	ddm := model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Valid:      true,
		Deleted:    true,
		Autogen:    true,
		Value:      20.0,
		UpdateTime: prevUpdateTime.UTC(),
		DataTime:   prevUpdateTime.UTC(),
		Error:      nil,
	}
	ddm.SetQueries("metric query0")
	f.store.Set("default/dd-metric-0", ddm, "utest")

	ddm = model.DatadogMetricInternal{
		ID:         "default/dd-metric-1",
		Valid:      true,
		Deleted:    true,
		Autogen:    false,
		Value:      20.0,
		UpdateTime: prevUpdateTime.UTC(),
		DataTime:   prevUpdateTime.UTC(),
		Error:      nil,
	}
	ddm.SetQueries("metric query1")
	f.store.Set("default/dd-metric-1", ddm, "utest")

	f.expectDeleteDatadogMetricAction("default", "dd-metric-0")
	f.runControllerSync(true, "default/dd-metric-0", nil)

	// Check internal store content has not changed
	ddm = model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Valid:      true,
		Deleted:    true,
		Autogen:    true,
		Value:      20.0,
		UpdateTime: prevUpdateTime.UTC(),
		DataTime:   prevUpdateTime.UTC(),
		Error:      nil,
	}
	ddm.SetQueries("metric query0")
	assert.Equal(t, &ddm, f.store.Get("default/dd-metric-0"))

	// Test that `Deleted` attribute is not considered on non-Autogen DDM
	f.actions = nil
	f.runControllerSync(true, "default/dd-metric-1", nil)
	assert.Empty(t, f.actions)
}

// Scenario: Object has already been deleted, controller should clean up internal store
func TestLeaderDeleteCleanup(t *testing.T) {
	f := newFixture(t)

	updateTime := time.Now()
	ddm := model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Valid:      true,
		Deleted:    true,
		Value:      20.0,
		UpdateTime: updateTime,
		DataTime:   updateTime,
		Error:      nil,
	}
	ddm.SetQueries("metric query0")
	f.store.Set("default/dd-metric-0", ddm, "utest")

	f.runControllerSync(true, "default/dd-metric-0", nil)

	// Check internal store content has not changed
	assert.Equal(t, 0, f.store.Count())
}

// Scenario: Another event comes after Kubernetes object and internal store has been cleaned up
func TestLeaderDuplicatedDelete(t *testing.T) {
	f := newFixture(t)

	f.runControllerSync(true, "default/dd-metric-0", nil)

	// Check internal store content has not changed
	assert.Equal(t, 0, f.store.Count())
}

// Scenario: Test that followers only follows (no action, always update store) even if local content is newer
func TestFollower(t *testing.T) {
	f := newFixture(t)

	prevUpdateTime := time.Now().Add(-10 * time.Second)
	prevUpdateTimeKube := metav1.NewTime(prevUpdateTime)
	metric0, metric0Typed := newFakeDatadogMetric("default", "dd-metric-0", "metric query0", datadoghq.DatadogMetricStatus{
		Value: "10",
		Conditions: []datadoghq.DatadogMetricCondition{
			{
				Type:               datadoghq.DatadogMetricConditionTypeActive,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeValid,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeUpdated,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeError,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
		},
	})
	metric1, metric1Typed := newFakeDatadogMetric("default", "autogen-1", "metric query1", datadoghq.DatadogMetricStatus{
		Value: "10",
		Conditions: []datadoghq.DatadogMetricCondition{
			{
				Type:               datadoghq.DatadogMetricConditionTypeActive,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeValid,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeUpdated,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
			{
				Type:               datadoghq.DatadogMetricConditionTypeError,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: prevUpdateTimeKube,
				LastUpdateTime:     prevUpdateTimeKube,
			},
		},
	})
	unstructured.SetNestedField(metric1.Object, "dd-metric-1", "spec", "externalMetricName")
	metric1Typed.Spec.ExternalMetricName = "dd-metric-1"

	updateTime := time.Now()
	f.datadogMetricLister = append(f.datadogMetricLister, metric0, metric1)
	f.objects = append(f.objects, metric0Typed, metric1Typed)
	// We have new updates locally (maybe leader changed or something. Followers should still overwrite local cache)
	ddm := model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Valid:      true,
		Active:     true,
		Value:      20.0,
		UpdateTime: kubernetes.TimeWithoutWall(updateTime),
		DataTime:   kubernetes.TimeWithoutWall(updateTime),
		Error:      fmt.Errorf("Error from backend while fetching metric"),
	}
	ddm.SetQueries("metric query0")
	f.store.Set("default/dd-metric-0", ddm, "utest")

	f.runControllerSync(false, "default/dd-metric-0", nil)

	// Check internal store content
	assert.Equal(t, 1, f.store.Count())
	ddm = model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Valid:      true,
		Active:     true,
		Value:      10.0,
		UpdateTime: kubernetes.TimeWithoutWall(prevUpdateTime.UTC()),
		DataTime:   kubernetes.TimeWithoutWall(prevUpdateTime.UTC()),
		Error:      nil,
	}
	ddm.SetQueries("metric query0")
	assert.Equal(t, &ddm, f.store.Get("default/dd-metric-0"))

	f.runControllerSync(false, "default/autogen-1", nil)
	assert.Equal(t, 2, f.store.Count())

	ddm = model.DatadogMetricInternal{
		ID:                 "default/autogen-1",
		Valid:              true,
		Active:             true,
		Autogen:            true,
		ExternalMetricName: "dd-metric-1",
		Value:              10.0,
		UpdateTime:         kubernetes.TimeWithoutWall(prevUpdateTime.UTC()),
		DataTime:           kubernetes.TimeWithoutWall(prevUpdateTime.UTC()),
		Error:              nil,
	}
	ddm.SetQueries("metric query1")
	assert.Equal(t, &ddm, f.store.Get("default/autogen-1"))
}
