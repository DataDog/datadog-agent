// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tags

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestMetaToTags(t *testing.T) {
	redisPodTags := []string{
		"kube_namespace:default",
		"kube_resource_name:redis",
		"kube_resource_kind:pod",
		"pod_name:redis",
	}

	tests := []struct {
		name     string
		loadFunc func()
		kind     string
		objMeta  metav1.Object
		want     []string
		wantErr  bool
		wantFunc func(t *testing.T)
	}{
		{
			name:     "cache miss / pod",
			loadFunc: func() {},
			kind:     "Pod",
			objMeta: &metav1.ObjectMeta{
				UID:       types.UID("94f8f228-5207-4c68-b63d-87a2f8ad25f1"),
				Name:      "redis",
				Namespace: "default",
			},
			want:    redisPodTags,
			wantErr: false,
		},
		{
			name:     "cache hit / pod",
			loadFunc: func() { cache.Cache.Set("metaTags-94f8f228-5207-4c68-b63d-87a2f8ad25f1", redisPodTags, 1*time.Hour) },
			kind:     "",
			objMeta:  &metav1.ObjectMeta{UID: types.UID("94f8f228-5207-4c68-b63d-87a2f8ad25f1")},
			want:     redisPodTags,
			wantErr:  false,
		},
		{
			name:     "empty UID",
			loadFunc: func() {},
			kind:     "",
			objMeta:  &metav1.ObjectMeta{UID: types.UID("")},
			want:     []string{},
			wantErr:  true,
		},
		{
			name:     "with ownerref",
			loadFunc: func() {},
			kind:     "Pod",
			objMeta: &metav1.ObjectMeta{
				UID:       types.UID("b894f6c1-8ca8-4467-873a-cbe1e26f4b5f"),
				Name:      "datadog-bsp4p",
				Namespace: "monitoring",
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind: "DaemonSet",
						Name: "datadog",
					},
				},
			},
			want: []string{
				"kube_namespace:monitoring",
				"kube_resource_name:datadog-bsp4p",
				"kube_resource_kind:pod",
				"pod_name:datadog-bsp4p",
				"kube_ownerref_name:datadog",
				"kube_ownerref_kind:daemonset",
				"kube_daemon_set:datadog",
			},
			wantErr: false,
		},
		{
			name:     "cluster role",
			loadFunc: func() {},
			kind:     "ClusterRole",
			objMeta: &metav1.ObjectMeta{
				UID:  types.UID("e12bf43c-2015-44e1-b95b-b9f05fcd1c56"),
				Name: "datadog-agent",
			},
			want:    []string{"kube_resource_name:datadog-agent", "kube_resource_kind:clusterrole"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache.Cache.Flush()
			tt.loadFunc()
			got, err := MetaToTags(tt.kind, tt.objMeta)
			assert.Equal(t, tt.wantErr, err != nil)
			assert.ElementsMatch(t, tt.want, got)
			if err == nil {
				_, found := cache.Cache.Get(tagsCacheKeyPrefix + string(tt.objMeta.GetUID()))
				assert.True(t, found)
			}
		})
	}
}
