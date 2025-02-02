// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package deprecatedresources is an admission controller webhook use to detect the usage of deprecated APIGroup and APIVersions.
package deprecatedresources

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func Test_objectInfoMapType_IsDeprecated(t *testing.T) {
	deprecationInfo := map[schema.GroupVersionKind]deprecationInfoType{
		{
			Group: "autoscaling", Version: "v2beta2", Kind: "HorizontalPodAutoscaler"}: {
			isDeprecated:           true,
			deprecationVersion:     semVersionType{Major: 1, Minor: 23},
			removalVersion:         semVersionType{Major: 1, Minor: 26},
			recommendedReplacement: schema.GroupVersionKind{Kind: "HorizontalPodAutoscaler", Version: "v2", Group: "autoscaling"},
		},
	}
	tests := []struct {
		name  string
		infos map[schema.GroupVersionKind]deprecationInfoType
		gvk   schema.GroupVersionKind
		want  deprecationInfoType
	}{
		{
			name:  "deprecated HPA v2beta2",
			infos: deprecationInfo,
			gvk:   schema.GroupVersionKind{Group: "autoscaling", Version: "v2beta2", Kind: "HorizontalPodAutoscaler"},
			want: deprecationInfoType{
				isDeprecated:           true,
				deprecationVersion:     semVersionType{Major: 1, Minor: 23},
				removalVersion:         semVersionType{Major: 1, Minor: 26},
				recommendedReplacement: schema.GroupVersionKind{Kind: "HorizontalPodAutoscaler", Version: "v2", Group: "autoscaling"},
			},
		},
		{
			name:  "non deprecated HPA v2",
			infos: deprecationInfo,
			gvk:   schema.GroupVersionKind{Group: "autoscaling", Version: "v1", Kind: "HorizontalPodAutoscaler"},
			want: deprecationInfoType{
				isDeprecated: false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := newObjectInfoMap(tt.infos)
			if got := o.IsDeprecated(tt.gvk); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("objectInfoMapType.IsDeprecated() = %v, want %v", got, tt.want)
			}
		})
	}
}
