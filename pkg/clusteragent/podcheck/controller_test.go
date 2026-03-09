// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package podcheck

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

var testGVR = schema.GroupVersionResource{
	Group:    "datadoghq.com",
	Version:  "v1alpha1",
	Resource: "datadogpodchecks",
}

func newUnstructuredPodCheck(namespace, name string, labels map[string]string, checks []interface{}) *unstructured.Unstructured {
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
			"kind":       "DatadogPodCheck",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"selector": selector,
				"checks":   checks,
			},
		},
	}
}

func makeCheck(name string, adIDs []interface{}, instances []interface{}) map[string]interface{} {
	check := map[string]interface{}{
		"name":       name,
		"initConfig": map[string]interface{}{},
		"instances":  instances,
	}
	if adIDs != nil {
		check["adIdentifiers"] = adIDs
	}
	return check
}

func setupTestController(t *testing.T, configMapName, configMapNamespace string, existingCRs []*unstructured.Unstructured) (*PodCheckController, *k8sfake.Clientset) {
	t.Helper()

	scheme := runtime.NewScheme()

	objs := make([]runtime.Object, len(existingCRs))
	for i, cr := range existingCRs {
		objs[i] = cr
	}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			testGVR: "DatadogPodCheckList",
		},
		objs...,
	)

	informerFactory := dynamicinformer.NewDynamicSharedInformerFactory(dynamicClient, 0)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: configMapNamespace,
		},
		Data: map[string]string{},
	}
	kubeClient := k8sfake.NewSimpleClientset(cm)

	controller, err := NewPodCheckController(
		informerFactory,
		kubeClient,
		func() bool { return true },
		configMapName,
		configMapNamespace,
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	informerFactory.Start(ctx.Done())
	informerFactory.WaitForCacheSync(ctx.Done())

	return controller, kubeClient
}

func TestReconcile_SingleCR(t *testing.T) {
	cr := newUnstructuredPodCheck("web-team", "nginx-check",
		map[string]string{"app": "nginx"},
		[]interface{}{
			makeCheck("nginx", []interface{}{"nginx:latest"}, []interface{}{
				map[string]interface{}{"nginx_status_url": "http://%%host%%:81/status/"},
			}),
		})

	controller, kubeClient := setupTestController(t, "dda-podcheck-config", "datadog", []*unstructured.Unstructured{cr})

	err := controller.reconcile()
	require.NoError(t, err)

	cm, err := kubeClient.CoreV1().ConfigMaps("datadog").Get(context.TODO(), "dda-podcheck-config", metav1.GetOptions{})
	require.NoError(t, err)

	assert.Len(t, cm.Data, 1)
	assert.Contains(t, cm.Data, "web-team_nginx-check_nginx.yaml")

	yaml := cm.Data["web-team_nginx-check_nginx.yaml"]
	assert.Contains(t, yaml, "nginx:latest")
	assert.Contains(t, yaml, "nginx_status_url")
}

func TestReconcile_CRWithMultipleChecks(t *testing.T) {
	cr := newUnstructuredPodCheck("default", "multi",
		map[string]string{"app": "myapp"},
		[]interface{}{
			makeCheck("http_check", []interface{}{"myapp:v1"}, []interface{}{map[string]interface{}{"url": "http://%%host%%"}}),
			makeCheck("redisdb", []interface{}{"redis:7"}, []interface{}{map[string]interface{}{"host": "%%host%%"}}),
		})

	controller, kubeClient := setupTestController(t, "dda-podcheck-config", "datadog", []*unstructured.Unstructured{cr})

	err := controller.reconcile()
	require.NoError(t, err)

	cm, err := kubeClient.CoreV1().ConfigMaps("datadog").Get(context.TODO(), "dda-podcheck-config", metav1.GetOptions{})
	require.NoError(t, err)

	assert.Len(t, cm.Data, 2)
	assert.Contains(t, cm.Data, "default_multi_http_check.yaml")
	assert.Contains(t, cm.Data, "default_multi_redisdb.yaml")
}

func TestReconcile_MultipleCRs(t *testing.T) {
	cr1 := newUnstructuredPodCheck("web-team", "nginx-check",
		map[string]string{"app": "nginx"},
		[]interface{}{
			makeCheck("nginx", []interface{}{"nginx:latest"}, []interface{}{map[string]interface{}{"url": "http://%%host%%/status/"}}),
		})
	cr2 := newUnstructuredPodCheck("data-team", "redis-check",
		map[string]string{"app": "redis"},
		[]interface{}{
			makeCheck("redisdb", []interface{}{"redis:7"}, []interface{}{map[string]interface{}{"host": "%%host%%", "port": "6379"}}),
		})

	controller, kubeClient := setupTestController(t, "dda-podcheck-config", "datadog",
		[]*unstructured.Unstructured{cr1, cr2})

	err := controller.reconcile()
	require.NoError(t, err)

	cm, err := kubeClient.CoreV1().ConfigMaps("datadog").Get(context.TODO(), "dda-podcheck-config", metav1.GetOptions{})
	require.NoError(t, err)

	assert.Len(t, cm.Data, 2)
	assert.Contains(t, cm.Data, "web-team_nginx-check_nginx.yaml")
	assert.Contains(t, cm.Data, "data-team_redis-check_redisdb.yaml")
}

func TestReconcile_EmptyCRList(t *testing.T) {
	controller, kubeClient := setupTestController(t, "dda-podcheck-config", "datadog", nil)

	err := controller.reconcile()
	require.NoError(t, err)

	cm, err := kubeClient.CoreV1().ConfigMaps("datadog").Get(context.TODO(), "dda-podcheck-config", metav1.GetOptions{})
	require.NoError(t, err)

	assert.Empty(t, cm.Data)
}

func TestReconcile_NonLeaderSkips(t *testing.T) {
	cr := newUnstructuredPodCheck("web-team", "nginx-check",
		map[string]string{"app": "nginx"},
		[]interface{}{
			makeCheck("nginx", []interface{}{"nginx:latest"}, []interface{}{map[string]interface{}{"url": "http://%%host%%/status/"}}),
		})

	controller, kubeClient := setupTestController(t, "dda-podcheck-config", "datadog",
		[]*unstructured.Unstructured{cr})

	controller.isLeader = func() bool { return false }

	err := controller.reconcile()
	require.NoError(t, err)

	cm, err := kubeClient.CoreV1().ConfigMaps("datadog").Get(context.TODO(), "dda-podcheck-config", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Empty(t, cm.Data)
}

func TestReconcile_NoUpdateWhenUnchanged(t *testing.T) {
	cr := newUnstructuredPodCheck("web-team", "nginx-check",
		map[string]string{"app": "nginx"},
		[]interface{}{
			makeCheck("nginx", []interface{}{"nginx:latest"}, []interface{}{map[string]interface{}{"url": "http://%%host%%/status/"}}),
		})

	controller, kubeClient := setupTestController(t, "dda-podcheck-config", "datadog",
		[]*unstructured.Unstructured{cr})

	err := controller.reconcile()
	require.NoError(t, err)

	cm1, err := kubeClient.CoreV1().ConfigMaps("datadog").Get(context.TODO(), "dda-podcheck-config", metav1.GetOptions{})
	require.NoError(t, err)

	err = controller.reconcile()
	require.NoError(t, err)

	cm2, err := kubeClient.CoreV1().ConfigMaps("datadog").Get(context.TODO(), "dda-podcheck-config", metav1.GetOptions{})
	require.NoError(t, err)

	assert.Equal(t, cm1.ResourceVersion, cm2.ResourceVersion)
}
