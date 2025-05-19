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
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

func TestToTypedSlice(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "rs",
		},
	}
	list := []interface{}{toInterface(rs)}
	typedList := toTypedSlice(k8s.NewReplicaSetCollector(utils.GetMetadataAsTags(mockconfig.New(t))), list)
	_, ok := typedList.([]*appsv1.ReplicaSet)
	require.True(t, ok)
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
