// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package nginx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

func newTestConfigMapReconciler(t *testing.T, objects ...runtime.Object) (*configMapReconciler, *dynamicfake.FakeDynamicClient) {
	t.Helper()

	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			configMapGVR:    "ConfigMapList",
			ingressClassGVR: "IngressClassList",
		},
		objects...,
	)

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Nginx: appsecconfig.Nginx{
				ModuleMountPath: "/modules_mount",
			},
		},
		Injection: appsecconfig.Injection{
			CommonLabels: map[string]string{
				"app.kubernetes.io/part-of":    "datadog",
				"app.kubernetes.io/component":  "datadog-appsec-injector",
				"app.kubernetes.io/managed-by": "datadog-cluster-agent",
			},
			CommonAnnotations: map[string]string{
				"annotation": "value",
			},
		},
	}

	return &configMapReconciler{
		client: client,
		logger: logger,
		config: config,
		eventRecorder: eventRecorder{
			recorder: record.NewFakeRecorder(100),
		},
	}, client
}

func TestReconcilerReconcile_Success(t *testing.T) {
	ctx := t.Context()
	originalName := "ingress-nginx-controller"
	ddName := ddConfigMapName(originalName)
	originalCM := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      originalName,
				"namespace": "ingress-nginx",
				"uid":       "original-uid",
				"labels": map[string]interface{}{
					watchedConfigMapLabel: "true",
				},
				"annotations": map[string]interface{}{
					ddConfigMapAnnotation: ddName,
				},
			},
			"data": map[string]interface{}{
				mainSnippetKey: "worker_connections 4096;",
				httpSnippetKey: "keepalive_timeout 75;",
			},
		},
	}
	existingDDCM := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":            ddName,
				"namespace":       "ingress-nginx",
				"resourceVersion": "1",
			},
			"data": map[string]interface{}{
				mainSnippetKey: "stale-main-snippet",
			},
		},
	}

	r, client := newTestConfigMapReconciler(t, originalCM, existingDDCM)
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedItemBasedRateLimiter[reconcileItem]())
	defer queue.ShutDown()

	item := reconcileItem{namespace: "ingress-nginx", originalName: originalName}
	queue.Add(item)
	received, quit := queue.Get()
	require.False(t, quit)
	require.Equal(t, item, received)

	r.reconcile(ctx, queue, received)
	assert.Equal(t, 0, queue.NumRequeues(item))

	ddCM, err := client.Resource(configMapGVR).Namespace("ingress-nginx").Get(ctx, ddName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "value", ddCM.GetAnnotations()["annotation"])
	assert.Equal(t, "datadog", ddCM.GetLabels()["app.kubernetes.io/part-of"])
	assert.Equal(t, string(appsecconfig.ProxyTypeIngressNginx), ddCM.GetLabels()[appsecconfig.AppsecProcessorProxyTypeAnnotation])
	data, found, err := unstructured.NestedStringMap(ddCM.UnstructuredContent(), "data")
	require.NoError(t, err)
	require.True(t, found)
	assert.Contains(t, data[mainSnippetKey], "load_module /modules_mount/ngx_http_datadog_module.so;")
	assert.Contains(t, data[mainSnippetKey], "worker_connections 4096;")
	assert.Contains(t, data[httpSnippetKey], "datadog_appsec_enabled on;")
	assert.Contains(t, data[httpSnippetKey], "keepalive_timeout 75;")
}

func TestReconcilerReconcile_MissingAnnotation(t *testing.T) {
	ctx := t.Context()
	originalName := "ingress-nginx-controller"
	originalCM := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      originalName,
				"namespace": "ingress-nginx",
				"labels": map[string]interface{}{
					watchedConfigMapLabel: "true",
				},
			},
		},
	}

	r, client := newTestConfigMapReconciler(t, originalCM)
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedItemBasedRateLimiter[reconcileItem]())
	defer queue.ShutDown()

	item := reconcileItem{namespace: "ingress-nginx", originalName: originalName}
	queue.Add(item)
	received, quit := queue.Get()
	require.False(t, quit)

	r.reconcile(ctx, queue, received)
	assert.Equal(t, 0, queue.NumRequeues(item))

	_, err := client.Resource(configMapGVR).Namespace("ingress-nginx").Get(ctx, ddConfigMapName(originalName), metav1.GetOptions{})
	assert.Error(t, err)
}

func TestReconcilerReconcile_AnnotationMismatch(t *testing.T) {
	ctx := t.Context()
	originalName := "ingress-nginx-controller"
	mismatchedDDName := ddConfigMapName("different-original")
	originalCM := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      originalName,
				"namespace": "ingress-nginx",
				"labels": map[string]interface{}{
					watchedConfigMapLabel: "true",
				},
				"annotations": map[string]interface{}{
					ddConfigMapAnnotation: mismatchedDDName,
				},
			},
		},
	}
	existingDDCM := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":            mismatchedDDName,
				"namespace":       "ingress-nginx",
				"resourceVersion": "1",
			},
			"data": map[string]interface{}{
				mainSnippetKey: "unchanged",
			},
		},
	}

	r, client := newTestConfigMapReconciler(t, originalCM, existingDDCM)
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedItemBasedRateLimiter[reconcileItem]())
	defer queue.ShutDown()

	item := reconcileItem{namespace: "ingress-nginx", originalName: originalName}
	queue.Add(item)
	received, quit := queue.Get()
	require.False(t, quit)

	r.reconcile(ctx, queue, received)
	assert.Equal(t, 0, queue.NumRequeues(item))

	ddCM, err := client.Resource(configMapGVR).Namespace("ingress-nginx").Get(ctx, mismatchedDDName, metav1.GetOptions{})
	require.NoError(t, err)
	data, found, err := unstructured.NestedStringMap(ddCM.UnstructuredContent(), "data")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "unchanged", data[mainSnippetKey])
}
