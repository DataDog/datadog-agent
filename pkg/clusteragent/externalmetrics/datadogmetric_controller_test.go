// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package externalmetrics

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
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

	c, err := NewDatadogMetricController(0, f.client, informer, &fakeLeaderElector{leader}, &f.store)
	if err != nil {
		return nil, nil
	}
	c.synced = alwaysReady

	for _, metric := range f.datadogMetricLister {
		informer.Datadoghq().V1alpha1().DatadogMetrics().Informer().GetIndexer().Add(metric)
	}

	return c, informer
}

func (f *fixture) runControllerSync(leader bool, datadogMetricId string, expectedError error) {
	controller, informer := f.newController(leader)
	stopCh := make(chan struct{})
	defer close(stopCh)
	informer.Start(stopCh)

	err := controller.syncDatadogMetric(datadogMetricId)
	if expectedError == nil && err != nil {
		f.t.Errorf("Unexpected error syncing foo: %v", err)
	} else if expectedError != nil && err != expectedError {
		f.t.Errorf("Expected error syncing foo, got nil or different error. Exepected: %v, Got: %v", expectedError, err)
	}

	actions := filterInformerActions(f.client.Actions())
	for i, action := range actions {
		if len(f.actions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(actions)-len(f.actions), actions[i:])
			break
		}

		expectedAction := f.actions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.actions) > len(actions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.actions)-len(actions), f.actions[len(actions):])
	}
}

func (f *fixture) expectUpdateDatadogMetricStatusAction(datadogMetric *datadoghq.DatadogMetric) {
	action := core.NewUpdateSubresourceAction(schema.GroupVersionResource{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogmetrics"}, "status", datadogMetric.Namespace, datadogMetric)
	f.actions = append(f.actions, action)
}

// checkAction verifies that expected and actual actions are equal and both have
// same attached resources
func checkAction(expected, actual core.Action, t *testing.T) {
	if !(expected.Matches(actual.GetVerb(), actual.GetResource().Resource) && actual.GetSubresource() == expected.GetSubresource()) {
		t.Errorf("Expected\n\t%#v\ngot\n\t%#v", expected, actual)
		return
	}

	if reflect.TypeOf(actual) != reflect.TypeOf(expected) {
		t.Errorf("Action has wrong type. Expected: %t. Got: %t", expected, actual)
		return
	}

	switch a := actual.(type) {
	case core.CreateActionImpl:
		e, _ := expected.(core.CreateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.UpdateActionImpl:
		e, _ := expected.(core.UpdateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.PatchActionImpl:
		e, _ := expected.(core.PatchActionImpl)
		expPatch := e.GetPatch()
		patch := a.GetPatch()

		if !reflect.DeepEqual(expPatch, patch) {
			t.Errorf("Action %s %s has wrong patch\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expPatch, patch))
		}
	default:
		t.Errorf("Uncaptured Action %s %s, you should explicitly add a case to capture it",
			actual.GetVerb(), actual.GetResource().Resource)
	}
}

// filterInformerActions filters list and watch actions for testing resources.
// Since list and watch don't change resource state we can filter it to lower
// nose level in our tests.
func filterInformerActions(actions []core.Action) []core.Action {
	ret := []core.Action{}
	for _, action := range actions {
		if len(action.GetNamespace()) == 0 &&
			(action.Matches("list", "DatadogMetric") ||
				action.Matches("watch", "DatadogMetric")) {
			continue
		}
		ret = append(ret, action)
	}

	return ret
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
		Id:         "default/dd-metric-0",
		Query:      "metric query0",
		Valid:      false,
		Value:      0,
		UpdateTime: testTime,
		Error:      nil,
	}, f.store.Get("default/dd-metric-0"))

	// Now that we validated that `UpdateTime` is after `testTime`, we read `UpdateTime` to allow comparison later
	updateTimeKube := metav1.NewTime(f.store.Get("default/dd-metric-0").UpdateTime)
	outputMetric := newFakeDatadogMetric("", "dd-metric-0", "", datadoghq.DatadogMetricStatus{
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
		Id:         "default/dd-metric-0",
		Query:      "metric query0",
		Valid:      true,
		Value:      10.0,
		UpdateTime: updateTime,
		Error:      nil,
	})

	outputMetric := newFakeDatadogMetric("", "dd-metric-0", "", datadoghq.DatadogMetricStatus{
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
		Id:         "default/dd-metric-0",
		Query:      "metric query0",
		Valid:      true,
		Value:      10.0,
		UpdateTime: updateTime,
		Error:      fmt.Errorf("Error from backend while fetching metric"),
	})

	outputMetric := newFakeDatadogMetric("", "dd-metric-0", "", datadoghq.DatadogMetricStatus{
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
		Id:         "default/dd-metric-0",
		Query:      "metric query0",
		Valid:      true,
		Value:      10.0,
		UpdateTime: updateTime,
		Error:      fmt.Errorf("Error from backend while fetching metric"),
	})

	f.runControllerSync(true, "default/dd-metric-0", nil)
}

// Scenario: Test that followers only follows (no action, always update store) even if local content is newer
func TestFollower(t *testing.T) {
	f := newFixture(t)

	prevUpdateTime := time.Now().Add(-10 * time.Second)
	prevUpdateTimeKube := metav1.NewTime(prevUpdateTime)
	metric := newFakeDatadogMetric("default", "dd-metric-0", "metric query0", datadoghq.DatadogMetricStatus{
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

	updateTime := time.Now()
	f.datadogMetricLister = append(f.datadogMetricLister, metric)
	f.objects = append(f.objects, metric)
	// We have new updates locally (maybe leader changed or something. Followers should still overwrite local cache)
	f.store.Set("default/dd-metric-0", model.DatadogMetricInternal{
		Id:         "default/dd-metric-0",
		Query:      "metric query0",
		Valid:      true,
		Value:      20.0,
		UpdateTime: updateTime,
		Error:      fmt.Errorf("Error from backend while fetching metric"),
	})

	f.runControllerSync(false, "default/dd-metric-0", nil)

	// Check internal store content
	assert.Equal(t, &model.DatadogMetricInternal{
		Id:         "default/dd-metric-0",
		Query:      "metric query0",
		Valid:      true,
		Value:      10.0,
		UpdateTime: prevUpdateTime.UTC(),
		Error:      nil,
	}, f.store.Get("default/dd-metric-0"))
}
