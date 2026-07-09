// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubeactionsimpl

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
)

// expected max resource output size and over-sized resource bytes
const (
	wantMaxResourceOutputSize int = 1.5 * 1024 * 1024
	overSizedResourceBytes    int = 2 * 1024 * 1024
)

// configMapGVR is the GroupVersionResource the executor builds for a
// "configmaps" kind with apiVersion "v1".
var configMapGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}

// newGetResourceClient creates a fake dynamic client seeded with the given
// unstructured objects. The configmaps list kind is registered so the fake
// client can resolve the GVR used by the executor.
func newGetResourceClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			configMapGVR: "ConfigMapList",
		},
		objects...,
	)
}

// newUnstructuredConfigMap builds an unstructured ConfigMap with the provided data.
func newUnstructuredConfigMap(namespace, name string, data map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"data": data,
		},
	}
}

func TestGetResourceExecute(t *testing.T) {
	tests := []struct {
		testName     string
		objects      []runtime.Object
		apiVersion   string
		kind         string
		namespace    string
		name         string
		wantStatus   string
		wantContains string
		// wantPayloadKey is the exact key the payload to expect in the ExecutionResult
		wantPayloadKey string
		// wantPayloadJSON is the exact (compacted) JSON to expect in the ExecutionResult
		wantPayloadJSON string
	}{
		{
			testName:        "nominal: existing resource is returned",
			objects:         []runtime.Object{newUnstructuredConfigMap("default", "my-config", map[string]interface{}{"key": "value"})},
			apiVersion:      "v1",
			kind:            "configmaps",
			namespace:       "default",
			name:            "my-config",
			wantStatus:      kubeactions.StatusSuccess,
			wantContains:    "get resource configmaps/default/my-config success",
			wantPayloadKey:  "configmaps/default/my-config",
			wantPayloadJSON: `{"apiVersion":"v1","data":{"key":"value"},"kind":"ConfigMap","metadata":{"name":"my-config","namespace":"default"}}`,
		},
		{
			testName:     "kind does not exist",
			objects:      nil,
			apiVersion:   "v1",
			kind:         "widgets",
			namespace:    "default",
			name:         "my-widget",
			wantStatus:   kubeactions.StatusFailed,
			wantContains: "failed to get resource widgets/default/my-widget",
		},
		{
			testName:     "apiVersion does not exist",
			objects:      []runtime.Object{newUnstructuredConfigMap("default", "my-config", map[string]interface{}{"key": "value"})},
			apiVersion:   "v2beta1",
			kind:         "configmaps",
			namespace:    "default",
			name:         "my-config",
			wantStatus:   kubeactions.StatusFailed,
			wantContains: "failed to get resource configmaps/default/my-config",
		},
		{
			testName: "resource too large",
			objects: []runtime.Object{newUnstructuredConfigMap("default", "big-config", map[string]interface{}{
				"payload": strings.Repeat("a", overSizedResourceBytes),
			})},
			apiVersion:   "v1",
			kind:         "configmaps",
			namespace:    "default",
			name:         "big-config",
			wantStatus:   kubeactions.StatusFailed,
			wantContains: "resource is too large",
		},
		{
			testName:     "missing apiVersion is rejected",
			objects:      nil,
			apiVersion:   "",
			kind:         "configmaps",
			namespace:    "default",
			name:         "my-config",
			wantStatus:   kubeactions.StatusFailed,
			wantContains: "apiVersion is required",
		},
		{
			testName:     "protected resource kind is rejected",
			objects:      nil,
			apiVersion:   "v1",
			kind:         "secrets",
			namespace:    "default",
			name:         "my-secret",
			wantStatus:   kubeactions.StatusFailed,
			wantContains: "not allowed for security reasons",
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			client := newGetResourceClient(tt.objects...)
			executor := NewGetResourceExecutor(client)

			in := kubeactions.GetResourceInputs{ResourceRef: kubeactions.ResourceRef{
				APIVersion: tt.apiVersion,
				Kind:       tt.kind,
				Namespace:  tt.namespace,
				Name:       tt.name,
			}}
			result := executor.Execute(t.Context(), in)

			assert.Equal(t, tt.wantStatus, result.Status)
			if tt.wantContains != "" {
				assert.Contains(t, result.Message, tt.wantContains)
			}

			if tt.wantPayloadJSON != "" {
				require.Len(t, result.Payloads, 1)
				require.Contains(t, result.Payloads, tt.wantPayloadKey)

				payload := result.Payloads[tt.wantPayloadKey]
				assert.LessOrEqual(t, len(payload), wantMaxResourceOutputSize)
				assert.JSONEq(t, tt.wantPayloadJSON, string(payload))
			} else {
				assert.Empty(t, result.Payloads)
			}
		})
	}
}
