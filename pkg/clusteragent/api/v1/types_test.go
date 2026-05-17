// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestNamespacesPodsStringsSet_Set(t *testing.T) {
	namespacesPodsSet := NewNamespacesPodsStringsSet()

	namespacesPodsSet.Set("default", "pod1", "svc1")
	namespacesPodsSet.Set("default", "pod2", "svc1")
	namespacesPodsSet.Set("default", "pod3", "svc2")

	require.Equal(t, 3, len(namespacesPodsSet["default"]))
	assert.Equal(t, sets.New("svc1"), namespacesPodsSet["default"]["pod1"])
	assert.Equal(t, sets.New("svc1"), namespacesPodsSet["default"]["pod2"])
	assert.Equal(t, sets.New("svc2"), namespacesPodsSet["default"]["pod3"])
}

func TestNamespacesPodsStringsSet_Delete(t *testing.T) {
	tests := []struct {
		name         string
		initialSet   NamespacesPodsStringsSet
		namespace    string
		services     []string
		wantModified bool
		wantResult   NamespacesPodsStringsSet
	}{
		{
			name:         "non-existing namespace returns false",
			initialSet:   NamespacesPodsStringsSet{},
			namespace:    "ns1",
			services:     []string{"svc1"},
			wantModified: false,
			wantResult:   NamespacesPodsStringsSet{},
		},
		{
			name: "non-existing service returns false",
			initialSet: NamespacesPodsStringsSet{
				"ns1": map[string]sets.Set[string]{"pod1": sets.New("svc1")},
			},
			namespace:    "ns1",
			services:     []string{"svc-other"},
			wantModified: false,
			wantResult: NamespacesPodsStringsSet{
				"ns1": map[string]sets.Set[string]{"pod1": sets.New("svc1")},
			},
		},
		{
			name: "deleting existing service returns true",
			initialSet: NamespacesPodsStringsSet{
				"ns1": map[string]sets.Set[string]{"pod1": sets.New("svc1", "svc2")},
			},
			namespace:    "ns1",
			services:     []string{"svc1"},
			wantModified: true,
			wantResult: NamespacesPodsStringsSet{
				"ns1": map[string]sets.Set[string]{"pod1": sets.New("svc2")},
			},
		},
		{
			name: "delete service from multiple pods",
			initialSet: NamespacesPodsStringsSet{
				"ns1": map[string]sets.Set[string]{
					"pod1": sets.New("svc1"),
					"pod2": sets.New("svc1"),
					"pod3": sets.New("svc2"),
				},
			},
			namespace:    "ns1",
			services:     []string{"svc1"},
			wantModified: true,
			wantResult: NamespacesPodsStringsSet{
				"ns1": map[string]sets.Set[string]{"pod3": sets.New("svc2")},
			},
		},
		{
			name: "deleting last service from a namespace also deletes the namespace",
			initialSet: NamespacesPodsStringsSet{
				"ns1": map[string]sets.Set[string]{"pod1": sets.New("svc1")},
			},
			namespace:    "ns1",
			services:     []string{"svc1"},
			wantModified: true,
			wantResult:   NamespacesPodsStringsSet{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.initialSet.Delete(test.namespace, test.services...)
			assert.Equal(t, test.wantModified, got)
			assert.Equal(t, test.wantResult, test.initialSet)
		})
	}
}
