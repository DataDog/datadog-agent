// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package nginx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestDDConfigMapName(t *testing.T) {
	assert.Equal(t, "datadog-appsec-ingress-nginx-controller", ddConfigMapName("ingress-nginx-controller"))
	assert.Equal(t, "datadog-appsec-my-config", ddConfigMapName("my-config"))
}

func TestBuildSnippet(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		dd       string
		expected string
	}{
		{
			name:     "empty existing snippet",
			existing: "",
			dd:       "load_module /modules_mount/ngx_http_datadog_module.so;",
			expected: "# datadog-appsec-begin\nload_module /modules_mount/ngx_http_datadog_module.so;\n# datadog-appsec-end",
		},
		{
			name:     "existing snippet preserved",
			existing: "worker_connections 1024;",
			dd:       "load_module /modules_mount/ngx_http_datadog_module.so;",
			expected: "# datadog-appsec-begin\nload_module /modules_mount/ngx_http_datadog_module.so;\n# datadog-appsec-end\nworker_connections 1024;",
		},
		{
			name:     "idempotent - markers already present",
			existing: "# datadog-appsec-begin\nold_directive;\n# datadog-appsec-end\nworker_connections 1024;",
			dd:       "new_directive;",
			expected: "# datadog-appsec-begin\nnew_directive;\n# datadog-appsec-end\nworker_connections 1024;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildSnippet(tt.existing, tt.dd)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStripDDSnippet(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no markers",
			input:    "some config;",
			expected: "some config;",
		},
		{
			name:     "markers at start",
			input:    "# datadog-appsec-begin\nsome directive;\n# datadog-appsec-end\nuser config;",
			expected: "user config;",
		},
		{
			name:     "markers only",
			input:    "# datadog-appsec-begin\nsome directive;\n# datadog-appsec-end",
			expected: "",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, stripDDSnippet(tt.input))
		})
	}
}

func TestBuildSnippetRoundTrip(t *testing.T) {
	original := "user_config_line_1;\nuser_config_line_2;"
	dd := "load_module /modules_mount/ngx_http_datadog_module.so;"

	built := buildSnippet(original, dd)
	stripped := stripDDSnippet(built)
	assert.Equal(t, original, stripped)
}

func TestCreateOrUpdateDDConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()

	t.Run("creates DD ConfigMap from empty original", func(t *testing.T) {
		// Original ConfigMap exists but is empty (default for ingress-nginx)
		originalCM := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "ingress-nginx-controller",
					"namespace": "ingress-nginx",
				},
			},
		}

		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				configMapGVR: "ConfigMapList",
			},
			originalCM,
		)

		labels := map[string]string{"app": "datadog"}
		annotations := map[string]string{}

		err := createOrUpdateDDConfigMap(context.Background(), client, "ingress-nginx", "ingress-nginx-controller", "/modules_mount", labels, annotations)
		require.NoError(t, err)

		// Verify DD ConfigMap was created
		ddCM, err := client.Resource(configMapGVR).Namespace("ingress-nginx").Get(context.Background(), "datadog-appsec-ingress-nginx-controller", getOpts())
		require.NoError(t, err)

		data, _, _ := unstructured.NestedStringMap(ddCM.UnstructuredContent(), "data")
		assert.Contains(t, data[mainSnippetKey], "load_module /modules_mount/ngx_http_datadog_module.so;")
		assert.Contains(t, data[mainSnippetKey], "thread_pool waf_thread_pool")
		assert.Contains(t, data[mainSnippetKey], "env DD_AGENT_HOST;",
			"main-snippet must preserve DD_AGENT_HOST env var for nginx worker processes")
		assert.Contains(t, data[httpSnippetKey], "datadog_appsec_enabled on;")
		assert.Contains(t, data[httpSnippetKey], "datadog_waf_thread_pool_name waf_thread_pool;")
	})

	t.Run("creates DD ConfigMap with original data preserved", func(t *testing.T) {
		originalCM := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "ingress-nginx-controller",
					"namespace": "ingress-nginx",
				},
				"data": map[string]interface{}{
					"proxy-body-size":    "10m",
					"main-snippet":       "worker_connections 4096;",
					"custom-http-errors": "404,503",
				},
			},
		}

		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				configMapGVR: "ConfigMapList",
			},
			originalCM,
		)

		err := createOrUpdateDDConfigMap(context.Background(), client, "ingress-nginx", "ingress-nginx-controller", "/modules_mount", map[string]string{}, map[string]string{})
		require.NoError(t, err)

		ddCM, err := client.Resource(configMapGVR).Namespace("ingress-nginx").Get(context.Background(), "datadog-appsec-ingress-nginx-controller", getOpts())
		require.NoError(t, err)

		data, _, _ := unstructured.NestedStringMap(ddCM.UnstructuredContent(), "data")
		// Original keys preserved
		assert.Equal(t, "10m", data["proxy-body-size"])
		assert.Equal(t, "404,503", data["custom-http-errors"])
		// main-snippet has DD directives prepended to original
		assert.Contains(t, data[mainSnippetKey], "load_module")
		assert.Contains(t, data[mainSnippetKey], "worker_connections 4096;")
	})

	t.Run("handles missing original ConfigMap", func(t *testing.T) {
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				configMapGVR: "ConfigMapList",
			},
		)

		err := createOrUpdateDDConfigMap(context.Background(), client, "ingress-nginx", "ingress-nginx-controller", "/modules_mount", map[string]string{}, map[string]string{})
		require.NoError(t, err)

		ddCM, err := client.Resource(configMapGVR).Namespace("ingress-nginx").Get(context.Background(), "datadog-appsec-ingress-nginx-controller", getOpts())
		require.NoError(t, err)

		data, _, _ := unstructured.NestedStringMap(ddCM.UnstructuredContent(), "data")
		assert.Contains(t, data[mainSnippetKey], "load_module")
	})
}

func getOpts() metav1.GetOptions {
	return metav1.GetOptions{}
}
