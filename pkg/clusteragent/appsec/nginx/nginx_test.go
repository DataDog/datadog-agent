// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package nginx

import (
	"errors"
	"fmt"
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
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
)

func newTestNginxPattern(t *testing.T, objects ...runtime.Object) (*nginxInjectionPattern, *dynamicfake.FakeDynamicClient) {
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
			CommonAnnotations: map[string]string{},
		},
	}

	return &nginxInjectionPattern{
		client: client,
		logger: logger,
		config: config,
		eventRecorder: eventRecorder{
			recorder: record.NewFakeRecorder(100),
		},
	}, client
}

func newOriginalConfigMap(namespace, name, ddName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					watchedConfigMapLabel: "true",
				},
				"annotations": map[string]interface{}{
					ddConfigMapAnnotation: ddName,
				},
			},
		},
	}
}

func newDDConfigMap(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/part-of":                     "datadog",
					"app.kubernetes.io/component":                   "datadog-appsec-injector",
					"app.kubernetes.io/managed-by":                  "datadog-cluster-agent",
					appsecconfig.AppsecProcessorProxyTypeAnnotation: string(appsecconfig.ProxyTypeIngressNginx),
				},
			},
		},
	}
}

func TestDeleted_NonNginxIngressClass(t *testing.T) {
	ctx := t.Context()
	ddCM := newDDConfigMap("ingress-nginx", ddConfigMapName("ingress-nginx-controller"))
	pattern, client := newTestNginxPattern(t, ddCM)

	err := pattern.Deleted(ctx, newIngressClass("traefik", "traefik.io/ingress-controller"))
	require.NoError(t, err)

	_, err = client.Resource(configMapGVR).Namespace("ingress-nginx").Get(ctx, ddCM.GetName(), metav1.GetOptions{})
	require.NoError(t, err)
}

func TestDeleted_SkipsWhenOtherIngressClassesExist(t *testing.T) {
	ctx := t.Context()
	ddCM := newDDConfigMap("ingress-nginx", ddConfigMapName("ingress-nginx-controller"))
	pattern, client := newTestNginxPattern(t,
		ddCM,
		newIngressClass("nginx", ingressNginxControllerName),
		newIngressClass("nginx-internal", ingressNginxControllerName),
	)

	err := pattern.Deleted(ctx, newIngressClass("nginx", ingressNginxControllerName))
	require.NoError(t, err)

	_, err = client.Resource(configMapGVR).Namespace("ingress-nginx").Get(ctx, ddCM.GetName(), metav1.GetOptions{})
	require.NoError(t, err)
}

func TestDeleted_CleansUpDDConfigMaps(t *testing.T) {
	ctx := t.Context()
	firstOriginalName := "ingress-nginx-controller"
	secondOriginalName := "custom-nginx-controller"
	firstDDName := ddConfigMapName(firstOriginalName)
	secondDDName := ddConfigMapName(secondOriginalName)

	pattern, client := newTestNginxPattern(t,
		newOriginalConfigMap("ingress-nginx", firstOriginalName, firstDDName),
		newOriginalConfigMap("other-ns", secondOriginalName, secondDDName),
		newDDConfigMap("ingress-nginx", firstDDName),
		newDDConfigMap("other-ns", secondDDName),
	)

	err := pattern.Deleted(ctx, newIngressClass("nginx", ingressNginxControllerName))
	require.NoError(t, err)

	_, err = client.Resource(configMapGVR).Namespace("ingress-nginx").Get(ctx, firstDDName, metav1.GetOptions{})
	assert.Error(t, err)
	_, err = client.Resource(configMapGVR).Namespace("other-ns").Get(ctx, secondDDName, metav1.GetOptions{})
	assert.Error(t, err)

	firstOriginal, err := client.Resource(configMapGVR).Namespace("ingress-nginx").Get(ctx, firstOriginalName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotContains(t, firstOriginal.GetLabels(), watchedConfigMapLabel)
	assert.NotContains(t, firstOriginal.GetAnnotations(), ddConfigMapAnnotation)

	secondOriginal, err := client.Resource(configMapGVR).Namespace("other-ns").Get(ctx, secondOriginalName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotContains(t, secondOriginal.GetLabels(), watchedConfigMapLabel)
	assert.NotContains(t, secondOriginal.GetAnnotations(), ddConfigMapAnnotation)
}

func TestDeleted_PartialDeletionFailure(t *testing.T) {
	ctx := t.Context()
	firstOriginalName := "ingress-nginx-controller"
	secondOriginalName := "custom-nginx-controller"
	firstDDName := ddConfigMapName(firstOriginalName)
	secondDDName := ddConfigMapName(secondOriginalName)

	pattern, client := newTestNginxPattern(t,
		newOriginalConfigMap("ingress-nginx", firstOriginalName, firstDDName),
		newOriginalConfigMap("other-ns", secondOriginalName, secondDDName),
		newDDConfigMap("ingress-nginx", firstDDName),
		newDDConfigMap("other-ns", secondDDName),
	)

	deleteErr := errors.New("boom")
	client.PrependReactor("delete", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
		deleteAction, ok := action.(k8stesting.DeleteAction)
		if !ok {
			return false, nil, nil
		}
		if action.GetNamespace() == "other-ns" && deleteAction.GetName() == secondDDName {
			return true, nil, deleteErr
		}
		return false, nil, nil
	})

	err := pattern.Deleted(ctx, newIngressClass("nginx", ingressNginxControllerName))
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to delete 1 DD ConfigMap(s)")
	assert.ErrorContains(t, err, fmt.Sprintf("other-ns/%s: %v", secondDDName, deleteErr))

	_, err = client.Resource(configMapGVR).Namespace("ingress-nginx").Get(ctx, firstDDName, metav1.GetOptions{})
	assert.Error(t, err)
	_, err = client.Resource(configMapGVR).Namespace("other-ns").Get(ctx, secondDDName, metav1.GetOptions{})
	require.NoError(t, err)

	firstOriginal, err := client.Resource(configMapGVR).Namespace("ingress-nginx").Get(ctx, firstOriginalName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotContains(t, firstOriginal.GetLabels(), watchedConfigMapLabel)
	assert.NotContains(t, firstOriginal.GetAnnotations(), ddConfigMapAnnotation)

	secondOriginal, err := client.Resource(configMapGVR).Namespace("other-ns").Get(ctx, secondOriginalName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "true", secondOriginal.GetLabels()[watchedConfigMapLabel])
	assert.Equal(t, secondDDName, secondOriginal.GetAnnotations()[ddConfigMapAnnotation])
}

func TestDeleted_ListFailure(t *testing.T) {
	ctx := t.Context()
	pattern, client := newTestNginxPattern(t)

	listErr := errors.New("list failed")
	client.PrependReactor("list", "configmaps", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, listErr
	})

	err := pattern.Deleted(ctx, newIngressClass("nginx", ingressNginxControllerName))
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to list DD ConfigMaps for cleanup")
	assert.ErrorContains(t, err, listErr.Error())
}
