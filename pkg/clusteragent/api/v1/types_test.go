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

func TestNewNamespacesPodsStringsSet(t *testing.T) {
	m := NewNamespacesPodsStringsSet()
	if m == nil {
		t.Error("NewNamespacesPodsStringsSet() returned nil")
	}
	if len(m) != 0 {
		t.Errorf("NewNamespacesPodsStringsSet() returned non-empty map: %v", m)
	}
}

func TestNamespacesPodsStringsSet_Get(t *testing.T) {
	tests := []struct {
		name      string
		m         NamespacesPodsStringsSet
		namespace string
		podName   string
		wantFound bool
	}{
		{
			name:      "empty map",
			m:         NewNamespacesPodsStringsSet(),
			namespace: "ns",
			podName:   "pod",
			wantFound: false,
		},
		{
			name: "namespace not found",
			m: NamespacesPodsStringsSet{
				"other-ns": map[string]sets.Set[string]{"pod": sets.New("svc")},
			},
			namespace: "ns",
			podName:   "pod",
			wantFound: false,
		},
		{
			name: "pod not found",
			m: NamespacesPodsStringsSet{
				"ns": map[string]sets.Set[string]{"other-pod": sets.New("svc")},
			},
			namespace: "ns",
			podName:   "pod",
			wantFound: false,
		},
		{
			name: "found",
			m: NamespacesPodsStringsSet{
				"ns": map[string]sets.Set[string]{"pod": sets.New("svc1", "svc2")},
			},
			namespace: "ns",
			podName:   "pod",
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := tt.m.Get(tt.namespace, tt.podName)
			if found != tt.wantFound {
				t.Errorf("Get() found = %v, want %v", found, tt.wantFound)
			}
			if tt.wantFound && got == nil {
				t.Error("Get() returned nil when expected values")
			}
		})
	}
}

func TestNamespacesPodsStringsSet_Set(t *testing.T) {
	tests := []struct {
		name      string
		m         NamespacesPodsStringsSet
		namespace string
		podName   string
		strings   []string
	}{
		{
			name:      "set on empty map",
			m:         NewNamespacesPodsStringsSet(),
			namespace: "ns",
			podName:   "pod",
			strings:   []string{"svc1", "svc2"},
		},
		{
			name: "set new namespace",
			m: NamespacesPodsStringsSet{
				"existing-ns": map[string]sets.Set[string]{"pod": sets.New("svc")},
			},
			namespace: "new-ns",
			podName:   "pod",
			strings:   []string{"svc1"},
		},
		{
			name: "set new pod in existing namespace",
			m: NamespacesPodsStringsSet{
				"ns": map[string]sets.Set[string]{"existing-pod": sets.New("svc")},
			},
			namespace: "ns",
			podName:   "new-pod",
			strings:   []string{"svc1"},
		},
		{
			name: "add to existing pod",
			m: NamespacesPodsStringsSet{
				"ns": map[string]sets.Set[string]{"pod": sets.New("svc-existing")},
			},
			namespace: "ns",
			podName:   "pod",
			strings:   []string{"svc-new"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.m.Set(tt.namespace, tt.podName, tt.strings...)

			got, found := tt.m.Get(tt.namespace, tt.podName)
			if !found {
				t.Errorf("Set() did not add entry")
				return
			}

			for _, s := range tt.strings {
				contained := false
				for _, g := range got {
					if g == s {
						contained = true
						break
					}
				}
				if !contained {
					t.Errorf("Set() did not add string %s", s)
				}
			}
		})
	}
}

func TestNamespacesPodsStringsSet_Delete(t *testing.T) {
	tests := []struct {
		name             string
		m                NamespacesPodsStringsSet
		namespace        string
		stringsToDelete  []string
		wantEmptyNs      bool
		expectedPodCount int
	}{
		{
			name:             "delete from non-existent namespace",
			m:                NewNamespacesPodsStringsSet(),
			namespace:        "ns",
			stringsToDelete:  []string{"svc"},
			wantEmptyNs:      true,
			expectedPodCount: 0,
		},
		{
			name: "delete all services from pod",
			m: NamespacesPodsStringsSet{
				"ns": map[string]sets.Set[string]{"pod": sets.New("svc1")},
			},
			namespace:        "ns",
			stringsToDelete:  []string{"svc1"},
			wantEmptyNs:      true,
			expectedPodCount: 0,
		},
		{
			name: "delete some services from pod",
			m: NamespacesPodsStringsSet{
				"ns": map[string]sets.Set[string]{"pod": sets.New("svc1", "svc2", "svc3")},
			},
			namespace:        "ns",
			stringsToDelete:  []string{"svc1"},
			wantEmptyNs:      false,
			expectedPodCount: 1,
		},
		{
			name: "delete service from multiple pods",
			m: NamespacesPodsStringsSet{
				"ns": map[string]sets.Set[string]{
					"pod1": sets.New("svc1", "svc2"),
					"pod2": sets.New("svc1"),
				},
			},
			namespace:        "ns",
			stringsToDelete:  []string{"svc1"},
			wantEmptyNs:      false,
			expectedPodCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.m.Delete(tt.namespace, tt.stringsToDelete...)

			if tt.wantEmptyNs {
				if _, ok := tt.m[tt.namespace]; ok {
					t.Errorf("Delete() did not remove empty namespace")
				}
			} else {
				if len(tt.m[tt.namespace]) != tt.expectedPodCount {
					t.Errorf("Delete() pod count = %d, want %d", len(tt.m[tt.namespace]), tt.expectedPodCount)
				}
			}
		})
	}
}

func TestNewMetadataResponseBundle(t *testing.T) {
	bundle := NewMetadataResponseBundle()
	if bundle == nil {
		t.Error("NewMetadataResponseBundle() returned nil")
	}
	if bundle.Services == nil {
		t.Error("NewMetadataResponseBundle() Services is nil")
	}
}

func TestNewMetadataResponse(t *testing.T) {
	resp := NewMetadataResponse()
	if resp == nil {
		t.Error("NewMetadataResponse() returned nil")
	}
	if resp.Nodes == nil {
		t.Error("NewMetadataResponse() Nodes is nil")
	}
}

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
