// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package externalmetrics

import (
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	core "k8s.io/client-go/testing"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics/model"
	datadoghq "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	dd_fake_clientset "github.com/DataDog/datadog-operator/pkg/generated/clientset/versioned/fake"
	dd_informers "github.com/DataDog/datadog-operator/pkg/generated/informers/externalversions"

	"github.com/stretchr/testify/assert"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

// Test fixture
type fixture struct {
	t *testing.T

	client *dd_fake_clientset.Clientset
	// Objects to put in the store.
	datadogMetricLister []*datadoghq.DatadogMetric
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

func (f *fixture) newController(leader bool) (*DatadogMetricController, dd_informers.SharedInformerFactory) {
	f.client = dd_fake_clientset.NewSimpleClientset(f.objects...)
	informer := dd_informers.NewSharedInformerFactory(f.client, noResyncPeriodFunc())

	c, err := NewDatadogMetricController(f.client, informer, getIsLeaderFunction(leader), &f.store)
	if err != nil {
		return nil, nil
	}
	c.synced = alwaysReady

	for _, metric := range f.datadogMetricLister {
		informer.Datadoghq().V1alpha1().DatadogMetrics().Informer().GetIndexer().Add(metric)
	}

	return c, informer
}

func (f *fixture) runControllerSync(leader bool, datadogMetricID string, expectedError error) {
	controller, informer := f.newController(leader)
	stopCh := make(chan struct{})
	defer close(stopCh)
	informer.Start(stopCh)

	err := controller.processDatadogMetric(datadogMetricID)
	assert.Equal(f.t, expectedError, err)
	// if expectedError == nil && err != nil {
	// 	f.t.Errorf("Unexpected error syncing foo: %v", err)
	// } else if expectedError != nil && err != expectedError {
	// 	f.t.Errorf("Expected error syncing foo, got nil or different error. Exepected: %v, Got: %v", expectedError, err)
	// }

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

func (f *fixture) expectCreateDatadogMetricAction(datadogMetric *datadoghq.DatadogMetric) {
	action := core.NewCreateAction(schema.GroupVersionResource{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogmetrics"}, datadogMetric.Namespace, datadogMetric)
	f.actions = append(f.actions, action)
}

func (f *fixture) expectDeleteDatadogMetricAction(ns, name string) {
	action := core.NewDeleteAction(schema.GroupVersionResource{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogmetrics"}, ns, name)
	f.actions = append(f.actions, action)
}

func (f *fixture) expectUpdateDatadogMetricStatusAction(datadogMetric *datadoghq.DatadogMetric) {
	action := core.NewUpdateSubresourceAction(schema.GroupVersionResource{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogmetrics"}, "status", datadogMetric.Namespace, datadogMetric)
	f.actions = append(f.actions, action)
}

func newFakeDatadogMetric(ns, name, query string, status datadoghq.DatadogMetricStatus) *datadoghq.DatadogMetric {
	return &datadoghq.DatadogMetric{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: datadoghq.DatadogMetricSpec{
			Query: query,
		},
		Status: status,
	}
}

// Scenario: user creates a new DatadogMetric `dd-metric-0`
// We check that a leader controller stores it locally. Test update if no changes happened before resync
func TestLeaderHandlingNewMetric(t *testing.T) {
	f := newFixture(t)
	metric := newFakeDatadogMetric("default", "dd-metric-0", "metric query0", datadoghq.DatadogMetricStatus{})
	testTime := time.Now().UTC()

	f.datadogMetricLister = append(f.datadogMetricLister, metric)
	f.objects = append(f.objects, metric)

	f.runControllerSync(true, "default/dd-metric-0", nil)

	// Check internal store content
	compareDatadogMetricInternal(t, &model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Query:      "metric query0",
		Valid:      false,
		Value:      0,
		UpdateTime: testTime,
		Error:      nil,
	}, f.store.Get("default/dd-metric-0"))

	// Now that we validated that `UpdateTime` is after `testTime`, we read `UpdateTime` to allow comparison later
	updateTimeKube := metav1.NewTime(f.store.Get("default/dd-metric-0").UpdateTime)
	outputMetric := newFakeDatadogMetric("default", "dd-metric-0", "", datadoghq.DatadogMetricStatus{
		Value: 0.0,
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

// Scenario: A DatadogMetric has been updated by background process (first time)
// We check that we synchronize that back to K8S
func TestLeaderUpdateFromStoreInitialUpdate(t *testing.T) {
	f := newFixture(t)

	updateTime := time.Now()
	updateTimeKube := metav1.NewTime(updateTime)
	metric := newFakeDatadogMetric("default", "dd-metric-0", "metric query0", datadoghq.DatadogMetricStatus{})
	f.datadogMetricLister = append(f.datadogMetricLister, metric)
	f.objects = append(f.objects, metric)
	f.store.Set("default/dd-metric-0", model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Query:      "metric query0",
		Valid:      true,
		Value:      10.0,
		UpdateTime: updateTime,
		Error:      nil,
	}, "utest")

	outputMetric := newFakeDatadogMetric("default", "dd-metric-0", "", datadoghq.DatadogMetricStatus{
		Value: 10.0,
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
	metric := newFakeDatadogMetric("default", "dd-metric-0", "metric query0", datadoghq.DatadogMetricStatus{
		Value: 10.0,
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
	f.objects = append(f.objects, metric)
	f.store.Set("default/dd-metric-0", model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Query:      "metric query0",
		Valid:      true,
		Value:      10.0,
		UpdateTime: updateTime,
		Error:      fmt.Errorf("Error from backend while fetching metric"),
	}, "utest")

	outputMetric := newFakeDatadogMetric("default", "dd-metric-0", "", datadoghq.DatadogMetricStatus{
		Value: 10.0,
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
	metric := newFakeDatadogMetric("default", "dd-metric-0", "metric query0", datadoghq.DatadogMetricStatus{
		Value: 10.0,
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
	f.objects = append(f.objects, metric)
	f.store.Set("default/dd-metric-0", model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Query:      "metric query0",
		Valid:      true,
		Value:      10.0,
		UpdateTime: updateTime,
		Error:      fmt.Errorf("Error from backend while fetching metric"),
	}, "utest")

	f.runControllerSync(true, "default/dd-metric-0", nil)
}

func TestCreateDatadogMetric(t *testing.T) {
	f := newFixture(t)

	updateTime := time.Now()
	updateTimeKube := metav1.NewTime(updateTime)
	f.store.Set("default/dd-metric-0", model.DatadogMetricInternal{
		ID:                 "default/dd-metric-0",
		Query:              "metric query0",
		Valid:              true,
		Deleted:            false,
		Autogen:            true,
		ExternalMetricName: "name1",
		Value:              20.0,
		UpdateTime:         updateTime,
		Error:              nil,
	}, "utest")
	f.store.Set("default/dd-metric-1", model.DatadogMetricInternal{
		ID:         "default/dd-metric-1",
		Query:      "metric query1",
		Valid:      true,
		Deleted:    false,
		Autogen:    false,
		Value:      20.0,
		UpdateTime: updateTime,
		Error:      nil,
	}, "utest")
	f.store.Set("default/dd-metric-1", model.DatadogMetricInternal{
		ID:         "default/dd-metric-1",
		Query:      "metric query1",
		Valid:      true,
		Deleted:    false,
		Autogen:    false,
		Value:      20.0,
		UpdateTime: updateTime,
		Error:      nil,
	}, "utest")
	f.store.Set("default/dd-metric-2", model.DatadogMetricInternal{
		ID:         "default/dd-metric-2",
		Query:      "metric query2",
		Valid:      true,
		Deleted:    false,
		Autogen:    true,
		Value:      20.0,
		UpdateTime: updateTime,
		Error:      nil,
	}, "utest")

	// Test successful creation
	expectedDatadogMetric := newFakeDatadogMetric("default", "dd-metric-0", "metric query0", datadoghq.DatadogMetricStatus{
		Value: 20.0,
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
	expectedDatadogMetric.Spec.ExternalMetricName = "name1"
	f.expectCreateDatadogMetricAction(expectedDatadogMetric)
	f.runControllerSync(true, "default/dd-metric-0", nil)

	// Test creating non autogen DatadogMetric
	f.actions = nil
	f.runControllerSync(true, "default/dd-metric-1", fmt.Errorf("Attempt to create DatadogMetric that was not auto-generated - not creating, DatadogMetric: %v", f.store.Get("default/dd-metric-1")))

	// Test create autogen without ExternalMetricName
	f.runControllerSync(true, "default/dd-metric-2", fmt.Errorf("Unable to create autogen DatadogMetric default/dd-metric-2 without ExternalMetricName"))
}

// Scenario: Test DatadogMetric is deleted if something from store is flagged as deleted and object exists in K8S
func TestLeaderDeleteExisting(t *testing.T) {
	f := newFixture(t)

	prevUpdateTime := time.Now().Add(-10 * time.Second)
	prevUpdateTimeKube := metav1.NewTime(prevUpdateTime)
	metric0 := newFakeDatadogMetric("default", "dd-metric-0", "metric query0", datadoghq.DatadogMetricStatus{
		Value: 20.0,
		Conditions: []datadoghq.DatadogMetricCondition{
			{
				Type:               datadoghq.DatadogMetricConditionTypeValid,
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
	metric1 := newFakeDatadogMetric("default", "dd-metric-1", "metric query1", datadoghq.DatadogMetricStatus{
		Value: 20.0,
		Conditions: []datadoghq.DatadogMetricCondition{
			{
				Type:               datadoghq.DatadogMetricConditionTypeValid,
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
	f.objects = append(f.objects, metric0, metric1)
	f.store.Set("default/dd-metric-0", model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Query:      "metric query0",
		Valid:      true,
		Deleted:    true,
		Autogen:    true,
		Value:      20.0,
		UpdateTime: prevUpdateTime.UTC(),
		Error:      nil,
	}, "utest")
	f.store.Set("default/dd-metric-1", model.DatadogMetricInternal{
		ID:         "default/dd-metric-1",
		Query:      "metric query1",
		Valid:      true,
		Deleted:    true,
		Autogen:    false,
		Value:      20.0,
		UpdateTime: prevUpdateTime.UTC(),
		Error:      nil,
	}, "utest")

	f.expectDeleteDatadogMetricAction("default", "dd-metric-0")
	f.runControllerSync(true, "default/dd-metric-0", nil)

	// Check internal store content has not changed
	assert.Equal(t, &model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Query:      "metric query0",
		Valid:      true,
		Deleted:    true,
		Autogen:    true,
		Value:      20.0,
		UpdateTime: prevUpdateTime.UTC(),
		Error:      nil,
	}, f.store.Get("default/dd-metric-0"))

	// Test that we get an error trying to delete a not Autogen DatadogMetric
	f.actions = nil
	f.runControllerSync(true, "default/dd-metric-1", fmt.Errorf("Attempt to delete DatadogMetric that was not auto-generated - not deleting, DatadogMetric: %v", f.store.Get("default/dd-metric-1")))
}

// Scenario: Object has already been deleted, controller should clean up internal store
func TestLeaderDeleteCleanup(t *testing.T) {
	f := newFixture(t)

	updateTime := time.Now()
	f.store.Set("default/dd-metric-0", model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Query:      "metric query0",
		Valid:      true,
		Deleted:    true,
		Value:      20.0,
		UpdateTime: updateTime,
		Error:      nil,
	}, "utest")

	f.runControllerSync(true, "default/dd-metric-0", nil)

	// Check internal store content has not changed
	assert.Equal(t, 0, f.store.Count())
}

// Scenario: Test that followers only follows (no action, always update store) even if local content is newer
func TestFollower(t *testing.T) {
	f := newFixture(t)

	prevUpdateTime := time.Now().Add(-10 * time.Second)
	prevUpdateTimeKube := metav1.NewTime(prevUpdateTime)
	metric0 := newFakeDatadogMetric("default", "dd-metric-0", "metric query0", datadoghq.DatadogMetricStatus{
		Value: 10.0,
		Conditions: []datadoghq.DatadogMetricCondition{
			{
				Type:               datadoghq.DatadogMetricConditionTypeValid,
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
	metric1 := newFakeDatadogMetric("default", "autogen-1", "metric query1", datadoghq.DatadogMetricStatus{
		Value: 10.0,
		Conditions: []datadoghq.DatadogMetricCondition{
			{
				Type:               datadoghq.DatadogMetricConditionTypeValid,
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
	metric1.Spec.ExternalMetricName = "dd-metric-1"

	updateTime := time.Now()
	f.datadogMetricLister = append(f.datadogMetricLister, metric0, metric1)
	f.objects = append(f.objects, metric0, metric1)
	// We have new updates locally (maybe leader changed or something. Followers should still overwrite local cache)
	f.store.Set("default/dd-metric-0", model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Query:      "metric query0",
		Valid:      true,
		Value:      20.0,
		UpdateTime: updateTime,
		Error:      fmt.Errorf("Error from backend while fetching metric"),
	}, "utest")

	f.runControllerSync(false, "default/dd-metric-0", nil)

	// Check internal store content
	assert.Equal(t, 1, f.store.Count())
	assert.Equal(t, &model.DatadogMetricInternal{
		ID:         "default/dd-metric-0",
		Query:      "metric query0",
		Valid:      true,
		Value:      10.0,
		UpdateTime: prevUpdateTime.UTC(),
		Error:      nil,
	}, f.store.Get("default/dd-metric-0"))

	f.runControllerSync(false, "default/autogen-1", nil)
	assert.Equal(t, 2, f.store.Count())
	assert.Equal(t, &model.DatadogMetricInternal{
		ID:                 "default/autogen-1",
		Query:              "metric query1",
		Valid:              true,
		Autogen:            true,
		ExternalMetricName: "dd-metric-1",
		Value:              10.0,
		UpdateTime:         prevUpdateTime.UTC(),
		Error:              nil,
	}, f.store.Get("default/autogen-1"))
}
