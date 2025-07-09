// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

//nolint:revive // TODO(CAPP) Fix revive linter
package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	pkgorchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
)

func TestToTypedSlice(t *testing.T) {
	tests := []struct {
		name           string
		setupResource  func() interface{}
		setupCollector func(t *testing.T) collectors.K8sCollector
		expectedType   func(result interface{}) bool
	}{
		{
			name: "replicaSet collector",
			setupResource: func() interface{} {
				rs := &appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rs",
					},
				}
				return toInterface(rs)
			},
			setupCollector: func(t *testing.T) collectors.K8sCollector {
				return k8s.NewReplicaSetCollector(utils.GetMetadataAsTags(mockconfig.New(t)))
			},
			expectedType: func(result interface{}) bool {
				_, ok := result.([]*appsv1.ReplicaSet)
				return ok
			},
		},
		{
			name: "crd collector",
			setupResource: func() interface{} {
				es := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "customResourceDefinition",
					},
				}
				return toInterface(es)
			},
			setupCollector: func(t *testing.T) collectors.K8sCollector {
				collector, err := k8s.NewCRCollector("customResourceDefinition", "apiextensions.k8s.io/v1")
				require.NoError(t, err)
				return collector
			},
			expectedType: func(result interface{}) bool {
				_, ok := result.([]runtime.Object)
				return ok
			},
		},
		{
			name: "cr collector",
			setupResource: func() interface{} {
				es := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apiextensions.k8s.io/v1",
						"kind":       "datadogcustomresource",
					},
				}
				return toInterface(es)
			},
			setupCollector: func(t *testing.T) collectors.K8sCollector {
				collector, err := k8s.NewCRCollector("datadogcustomresource", "apiextensions.k8s.io/v1")
				require.NoError(t, err)
				return collector
			},
			expectedType: func(result interface{}) bool {
				_, ok := result.([]runtime.Object)
				return ok
			},
		},
		{
			name: "generic collector",
			setupResource: func() interface{} {
				es := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "EndpointSlice",
					},
				}
				return toInterface(es)
			},
			setupCollector: func(t *testing.T) collectors.K8sCollector {
				collector, err := k8s.GenericResource{
					Name:         "endpointSlice",
					GroupVersion: "discovery.k8s.io/v1",
					NodeType:     pkgorchestratormodel.K8sEndpointSlice,
				}.NewGenericCollector()
				require.NoError(t, err)
				return collector
			},
			expectedType: func(result interface{}) bool {
				_, ok := result.([]runtime.Object)
				return ok
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := tt.setupResource()
			collector := tt.setupCollector(t)
			list := []interface{}{resource}

			typedList := toTypedSlice(collector, list)
			require.True(t, tt.expectedType(typedList))
		})
	}
}

func toInterface(i interface{}) interface{} {
	return i
}

func TestGetResource(t *testing.T) {
	tests := []struct {
		name     string
		resource interface{}
		wantOk   bool
	}{
		{
			name:     "nil resource",
			resource: nil,
			wantOk:   false,
		},
		{
			name: "DeletedFinalStateUnknown with nil object",
			resource: cache.DeletedFinalStateUnknown{
				Obj: nil,
			},
			wantOk: false,
		},
		{
			name: "DeletedFinalStateUnknown with ReplicaSet",
			resource: cache.DeletedFinalStateUnknown{
				Obj: &appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rs",
					},
				},
			},
			wantOk: true,
		},
		{
			name: "direct ReplicaSet",
			resource: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "rs",
				},
			},
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getResource(tt.resource)
			require.Equal(t, tt.wantOk, err == nil)
		})
	}
}
