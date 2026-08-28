// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package gke

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*dynamicfake.FakeDynamicClient)
		expected  bool
		wantErr   bool
	}{
		{
			name: "CRD present",
			setupMock: func(client *dynamicfake.FakeDynamicClient) {
				client.PrependReactor("get", "customresourcedefinitions", func(action k8stesting.Action) (bool, runtime.Object, error) {
					crd := &unstructured.Unstructured{}
					crd.SetName(action.(k8stesting.GetAction).GetName())
					return true, crd, nil
				})
			},
			expected: true,
			wantErr:  false,
		},
		{
			name: "CRD absent",
			setupMock: func(client *dynamicfake.FakeDynamicClient) {
				client.PrependReactor("get", "customresourcedefinitions", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewNotFound(
						schema.GroupResource{Group: "apiextensions.k8s.io", Resource: "customresourcedefinitions"},
						action.(k8stesting.GetAction).GetName(),
					)
				})
			},
			expected: false,
			wantErr:  false,
		},
		{
			name: "API error (Internal 500)",
			setupMock: func(client *dynamicfake.FakeDynamicClient) {
				client.PrependReactor("get", "customresourcedefinitions", func(_ k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewInternalError(errors.New("internal server error"))
				})
			},
			expected: false,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			client := dynamicfake.NewSimpleDynamicClient(scheme)
			tt.setupMock(client)

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
