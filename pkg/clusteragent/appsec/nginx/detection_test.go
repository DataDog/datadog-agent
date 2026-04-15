// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package nginx

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func newIngressClass(name, controllerName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "IngressClass",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"controller": controllerName,
			},
		},
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		objects  []*unstructured.Unstructured
		expected bool
		wantErr  bool
	}{
		{
			name:     "ingress-nginx IngressClass found",
			objects:  []*unstructured.Unstructured{newIngressClass("nginx", "k8s.io/ingress-nginx")},
			expected: true,
		},
		{
			name:     "no IngressClass found",
			objects:  nil,
			expected: false,
		},
		{
			name:     "wrong controller name",
			objects:  []*unstructured.Unstructured{newIngressClass("traefik", "traefik.io/ingress-controller")},
			expected: false,
		},
		{
			name: "multiple IngressClasses, one is nginx",
			objects: []*unstructured.Unstructured{
				newIngressClass("traefik", "traefik.io/ingress-controller"),
				newIngressClass("nginx", "k8s.io/ingress-nginx"),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			var objs []runtime.Object
			for _, o := range tt.objects {
				objs = append(objs, o)
			}
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
				map[schema.GroupVersionResource]string{
					ingressClassGVR: "IngressClassList",
				},
				objs...,
			)

			found, err := Detect(context.Background(), client)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.expected, found)
		})
	}
}

func TestDetect_APIError(t *testing.T) {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			ingressClassGVR: "IngressClassList",
		},
	)
	client.PrependReactor("list", "ingressclasses", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("api error")
	})

	found, err := Detect(context.Background(), client)
	assert.Error(t, err)
	assert.False(t, found)
}
