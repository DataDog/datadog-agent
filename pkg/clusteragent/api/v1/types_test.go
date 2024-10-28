// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
)

func TestNamespacesPodsStringsSet_Copy(t *testing.T) {
	tests := []struct {
		name string
		m    NamespacesPodsStringsSet
		old  *NamespacesPodsStringsSet
		want NamespacesPodsStringsSet
	}{
		{
			name: "nil input map",
			m: NamespacesPodsStringsSet{
				"foo": map[string]sets.Set[string]{"bar": sets.New("buz")},
			},
			old: nil,
			want: NamespacesPodsStringsSet{
				"foo": map[string]sets.Set[string]{"bar": sets.New("buz")},
			},
		},
		{
			name: "base case",
			m:    NamespacesPodsStringsSet{},
			old: &NamespacesPodsStringsSet{
				"foo": map[string]sets.Set[string]{"bar": sets.New("buz")},
			},
			want: NamespacesPodsStringsSet{
				"foo": map[string]sets.Set[string]{"bar": sets.New("buz")},
			},
		},
		{
			name: "merge case",
			m: NamespacesPodsStringsSet{
				"fuu": map[string]sets.Set[string]{"bur": sets.New("buz")},
			},
			old: &NamespacesPodsStringsSet{
				"foo": map[string]sets.Set[string]{"bar": sets.New("buz")},
			},
			want: NamespacesPodsStringsSet{
				"foo": map[string]sets.Set[string]{"bar": sets.New("buz")},
				"fuu": map[string]sets.Set[string]{"bur": sets.New("buz")},
			},
		},
		{
			name: "merge service case",
			m: NamespacesPodsStringsSet{
				"foo": map[string]sets.Set[string]{"bur": sets.New("boz")},
			},
			old: &NamespacesPodsStringsSet{
				"foo": map[string]sets.Set[string]{"bar": sets.New("buz")},
			},
			want: NamespacesPodsStringsSet{
				"foo": map[string]sets.Set[string]{"bar": sets.New("buz"), "bur": sets.New("boz")},
			},
		},
		{
			name: "union case",
			m: NamespacesPodsStringsSet{
				"foo": map[string]sets.Set[string]{"bur": sets.New("boz")},
			},
			old: &NamespacesPodsStringsSet{
				"foo": map[string]sets.Set[string]{"bur": sets.New("buz")},
			},
			want: NamespacesPodsStringsSet{
				"foo": map[string]sets.Set[string]{"bur": sets.New("buz", "boz")},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.DeepCopy(tt.old); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NamespacesPodsStringsSet.Copy() = %v, want %v", got, tt.want)
			}
		})
	}
}
