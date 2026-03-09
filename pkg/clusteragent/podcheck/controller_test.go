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

func newUnstructuredPodCheck(namespace, name, image, checkName string, instances []map[string]interface{}) *unstructured.Unstructured {
	instancesList := make([]interface{}, len(instances))
	for i, inst := range instances {
		instancesList[i] = inst
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
				"containerImage": image,
				"check": map[string]interface{}{
					"name":       checkName,
					"initConfig": map[string]interface{}{},
					"instances":  instancesList,
				},
			},
		},
	}
}

func setupTestController(t *testing.T, configMapName, configMapNamespace string, existingCRs []*unstructured.Unstructured) (*PodCheckController, *k8sfake.Clientset) {
	t.Helper()

	scheme := runtime.NewScheme()

	// Create dynamic client with existing CRs
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

	// Create informer factory
	informerFactory := dynamicinformer.NewDynamicSharedInformerFactory(dynamicClient, 0)

	// Create kube client with empty ConfigMap
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
		func() bool { return true }, // always leader
		configMapName,
		configMapNamespace,
	)
	require.NoError(t, err)

	// Start informers and wait for sync
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	informerFactory.Start(ctx.Done())
	informerFactory.WaitForCacheSync(ctx.Done())

	return controller, kubeClient
}

func TestReconcile_SingleCR(t *testing.T) {
	cr := newUnstructuredPodCheck("web-team", "nginx-check", "nginx:latest", "nginx",
		[]map[string]interface{}{
			{"nginx_status_url": "http://%%host%%:81/status/"},
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

func TestReconcile_MultipleCRs(t *testing.T) {
	cr1 := newUnstructuredPodCheck("web-team", "nginx-check", "nginx:latest", "nginx",
		[]map[string]interface{}{{"url": "http://%%host%%/status/"}})
	cr2 := newUnstructuredPodCheck("data-team", "redis-check", "redis:7", "redisdb",
		[]map[string]interface{}{{"host": "%%host%%", "port": "6379"}})

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
	cr := newUnstructuredPodCheck("web-team", "nginx-check", "nginx:latest", "nginx",
		[]map[string]interface{}{{"url": "http://%%host%%/status/"}})

	controller, kubeClient := setupTestController(t, "dda-podcheck-config", "datadog",
		[]*unstructured.Unstructured{cr})

	// Override to non-leader
	controller.isLeader = func() bool { return false }

	err := controller.reconcile()
	require.NoError(t, err)

	// ConfigMap should remain empty since we're not leader
	cm, err := kubeClient.CoreV1().ConfigMaps("datadog").Get(context.TODO(), "dda-podcheck-config", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Empty(t, cm.Data)
}

func TestReconcile_NoUpdateWhenUnchanged(t *testing.T) {
	cr := newUnstructuredPodCheck("web-team", "nginx-check", "nginx:latest", "nginx",
		[]map[string]interface{}{{"url": "http://%%host%%/status/"}})

	controller, kubeClient := setupTestController(t, "dda-podcheck-config", "datadog",
		[]*unstructured.Unstructured{cr})

	// First reconcile
	err := controller.reconcile()
	require.NoError(t, err)

	// Get the ConfigMap state after first reconcile
	cm1, err := kubeClient.CoreV1().ConfigMaps("datadog").Get(context.TODO(), "dda-podcheck-config", metav1.GetOptions{})
	require.NoError(t, err)

	// Second reconcile - should be a no-op
	err = controller.reconcile()
	require.NoError(t, err)

	cm2, err := kubeClient.CoreV1().ConfigMaps("datadog").Get(context.TODO(), "dda-podcheck-config", metav1.GetOptions{})
	require.NoError(t, err)

	// ResourceVersion shouldn't change since data is the same
	assert.Equal(t, cm1.ResourceVersion, cm2.ResourceVersion)
}
