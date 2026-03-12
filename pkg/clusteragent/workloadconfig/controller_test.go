// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workloadconfig

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

var testGVR = schema.GroupVersionResource{
	Group:    "datadoghq.com",
	Version:  "v1alpha1",
	Resource: "datadoginstrumentations",
}

// fakeHandler records calls from the controller for testing.
type fakeHandler struct {
	name      string
	callCount int
	lastCRs   []*datadoghq.DatadogInstrumentation
	err       error
}

func (f *fakeHandler) Name() string { return f.name }
func (f *fakeHandler) Reconcile(crs []*datadoghq.DatadogInstrumentation) error {
	f.callCount++
	f.lastCRs = crs
	return f.err
}

func newUnstructuredWorkloadConfig(namespace, name string, labels map[string]string, checks []interface{}) *unstructured.Unstructured {
	selector := map[string]interface{}{}
	if labels != nil {
		matchLabels := make(map[string]interface{}, len(labels))
		for k, v := range labels {
			matchLabels[k] = v
		}
		selector["matchLabels"] = matchLabels
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "datadoghq.com/v1alpha1",
			"kind":       "DatadogInstrumentation",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"selector": selector,
				"config": map[string]interface{}{
					"checks": checks,
				},
			},
		},
	}
}

func makeCheck(integration string, containerImages []interface{}, instances []interface{}) map[string]interface{} {
	check := map[string]interface{}{
		"integration": integration,
		"initConfig":  map[string]interface{}{},
		"instances":   instances,
	}
	if containerImages != nil {
		check["containerImage"] = containerImages
	}
	return check
}

func setupTestController(t *testing.T, existingCRs []*unstructured.Unstructured, handlers ...ConfigSectionHandler) *InstrumentationController {
	t.Helper()

	scheme := runtime.NewScheme()

	objs := make([]runtime.Object, len(existingCRs))
	for i, cr := range existingCRs {
		objs[i] = cr
	}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			testGVR: "DatadogInstrumentationList",
		},
		objs...,
	)

	informerFactory := dynamicinformer.NewDynamicSharedInformerFactory(dynamicClient, 0)

	controller, err := NewInstrumentationCRDController(
		informerFactory,
		func() bool { return true },
		make(chan struct{}, 1),
		handlers,
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	informerFactory.Start(ctx.Done())
	informerFactory.WaitForCacheSync(ctx.Done())

	return controller
}

func TestReconcile_SingleCR(t *testing.T) {
	cr := newUnstructuredWorkloadConfig("web-team", "nginx-check",
		map[string]string{"app": "nginx"},
		[]interface{}{
			makeCheck("nginx", []interface{}{"nginx:latest"}, []interface{}{
				map[string]interface{}{"nginx_status_url": "http://%%host%%:81/status/"},
			}),
		})

	handler := &fakeHandler{name: "test"}
	controller := setupTestController(t, []*unstructured.Unstructured{cr}, handler)

	err := controller.reconcile()
	require.NoError(t, err)

	assert.Equal(t, 1, handler.callCount)
	require.Len(t, handler.lastCRs, 1)
	assert.Equal(t, "nginx-check", handler.lastCRs[0].Name)
}

func TestReconcile_MultipleCRs(t *testing.T) {
	cr1 := newUnstructuredWorkloadConfig("web-team", "nginx-check",
		map[string]string{"app": "nginx"},
		[]interface{}{
			makeCheck("nginx", []interface{}{"nginx:latest"}, []interface{}{map[string]interface{}{"url": "http://%%host%%/status/"}}),
		})
	cr2 := newUnstructuredWorkloadConfig("data-team", "redis-check",
		map[string]string{"app": "redis"},
		[]interface{}{
			makeCheck("redisdb", []interface{}{"redis:7"}, []interface{}{map[string]interface{}{"host": "%%host%%", "port": "6379"}}),
		})

	handler := &fakeHandler{name: "test"}
	controller := setupTestController(t, []*unstructured.Unstructured{cr1, cr2}, handler)

	err := controller.reconcile()
	require.NoError(t, err)

	assert.Equal(t, 1, handler.callCount)
	assert.Len(t, handler.lastCRs, 2)
}

func TestReconcile_EmptyCRList(t *testing.T) {
	handler := &fakeHandler{name: "test"}
	controller := setupTestController(t, nil, handler)

	err := controller.reconcile()
	require.NoError(t, err)

	assert.Equal(t, 1, handler.callCount)
	assert.Empty(t, handler.lastCRs)
}

func TestReconcile_NonLeaderSkips(t *testing.T) {
	cr := newUnstructuredWorkloadConfig("web-team", "nginx-check",
		map[string]string{"app": "nginx"},
		[]interface{}{
			makeCheck("nginx", []interface{}{"nginx:latest"}, []interface{}{map[string]interface{}{"url": "http://%%host%%/status/"}}),
		})

	handler := &fakeHandler{name: "test"}
	controller := setupTestController(t, []*unstructured.Unstructured{cr}, handler)

	controller.isLeader = func() bool { return false }

	err := controller.reconcile()
	require.NoError(t, err)

	assert.Equal(t, 0, handler.callCount)
}

func TestReconcile_MultipleHandlers(t *testing.T) {
	cr := newUnstructuredWorkloadConfig("default", "check",
		map[string]string{"app": "test"},
		[]interface{}{
			makeCheck("http_check", nil, []interface{}{map[string]interface{}{"url": "http://localhost"}}),
		})

	handler1 := &fakeHandler{name: "handler1"}
	handler2 := &fakeHandler{name: "handler2"}
	controller := setupTestController(t, []*unstructured.Unstructured{cr}, handler1, handler2)

	err := controller.reconcile()
	require.NoError(t, err)

	assert.Equal(t, 1, handler1.callCount)
	assert.Equal(t, 1, handler2.callCount)
	assert.Len(t, handler1.lastCRs, 1)
	assert.Len(t, handler2.lastCRs, 1)
}

func TestReconcile_HandlerErrorReturned(t *testing.T) {
	cr := newUnstructuredWorkloadConfig("default", "check",
		map[string]string{"app": "test"},
		[]interface{}{
			makeCheck("http_check", nil, []interface{}{map[string]interface{}{"url": "http://localhost"}}),
		})

	handler1 := &fakeHandler{name: "handler1", err: assert.AnError}
	handler2 := &fakeHandler{name: "handler2"}
	controller := setupTestController(t, []*unstructured.Unstructured{cr}, handler1, handler2)

	err := controller.reconcile()
	assert.ErrorIs(t, err, assert.AnError)

	// Both handlers should still be called
	assert.Equal(t, 1, handler1.callCount)
	assert.Equal(t, 1, handler2.callCount)
}
