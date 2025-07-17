// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagsfromlabels

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestGetAndCacheOwner(t *testing.T) {
	ownerInfo := dummyInfo()
	kubeObj := newUnstructuredWithSpec(map[string]interface{}{"foo": "bar"})
	owner := newOwner(kubeObj)
	config := fxutil.Test[config.Component](t, core.MockBundle())
	ownerCacheTTL := ownerCacheTTL(config)

	// Cache hit
	cache.Cache.Set(ownerInfo.buildID(testNamespace), owner, ownerCacheTTL)
	dc := fake.NewSimpleDynamicClient(scheme)
	obj, err := getAndCacheOwner(ownerInfo, testNamespace, dc, ownerCacheTTL)
	assert.NoError(t, err)
	assert.NotNil(t, obj)
	assert.Equal(t, owner, obj)
	assert.Len(t, dc.Actions(), 0)
	cache.Cache.Flush()

	// Cache miss
	dc = fake.NewSimpleDynamicClient(scheme, kubeObj)
	obj, err = getAndCacheOwner(ownerInfo, testNamespace, dc, ownerCacheTTL)
	assert.NoError(t, err)
	assert.NotNil(t, obj)
	assert.Equal(t, owner, obj)
	assert.Len(t, dc.Actions(), 1)
	cachedObj, found := cache.Cache.Get(ownerInfo.buildID(testNamespace))
	assert.True(t, found)
	assert.NotNil(t, cachedObj)
}

func Test_getOwnerInfo(t *testing.T) {
	tests := []struct {
		name    string
		owner   metav1.OwnerReference
		want    *ownerInfo
		wantErr bool
	}{
		{
			name: "replicaset",
			owner: metav1.OwnerReference{
				APIVersion:         "apps/v1",
				BlockOwnerDeletion: pointer.Ptr(true),
				Controller:         pointer.Ptr(true),
				Kind:               "ReplicaSet",
				Name:               "my-app-547c56f566",
				UID:                "2dfa7d22-245f-4769-8854-bc3b056cd224",
			},
			want: &ownerInfo{
				name: "my-app-547c56f566",
				gvr: schema.GroupVersionResource{
					Group:    "apps",
					Version:  "v1",
					Resource: "replicasets",
				},
			},
			wantErr: false,
		},
		{
			name: "job",
			owner: metav1.OwnerReference{
				APIVersion:         "batch/v1",
				BlockOwnerDeletion: pointer.Ptr(true),
				Controller:         pointer.Ptr(true),
				Kind:               "Job",
				Name:               "my-job",
				UID:                "89e8148c-8601-4c69-b8a6-3fbb176547d0",
			},
			want: &ownerInfo{
				name: "my-job",
				gvr: schema.GroupVersionResource{
					Group:    "batch",
					Version:  "v1",
					Resource: "jobs",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid APIVersion",
			owner: metav1.OwnerReference{
				APIVersion:         "batch/v1/",
				BlockOwnerDeletion: pointer.Ptr(true),
				Controller:         pointer.Ptr(true),
				Kind:               "Job",
				Name:               "my-job",
				UID:                "89e8148c-8601-4c69-b8a6-3fbb176547d0",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getOwnerInfo(tt.owner)
			if (err != nil) != tt.wantErr {
				t.Errorf("getOwnerInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getOwnerInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}

const (
	testGroup      = "testgroup"
	testVersion    = "testversion"
	testResource   = "testkinds"
	testNamespace  = "testns"
	testName       = "testname"
	testKind       = "TestKind"
	testAPIVersion = "testgroup/testversion"
)

func dummyInfo() *ownerInfo {
	return &ownerInfo{
		name: testName,
		gvr: schema.GroupVersionResource{
			Group:    testGroup,
			Resource: testResource,
			Version:  testVersion,
		},
	}
}

func newOwner(obj *unstructured.Unstructured) *owner {
	return &owner{
		name:            obj.GetName(),
		namespace:       obj.GetNamespace(),
		kind:            obj.GetKind(),
		labels:          obj.GetLabels(),
		ownerReferences: obj.GetOwnerReferences(),
	}
}

func newUnstructured(apiVersion, kind, namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"namespace": namespace,
				"name":      name,
			},
		},
	}
}

func newUnstructuredWithSpec(spec map[string]interface{}) *unstructured.Unstructured {
	u := newUnstructured(testAPIVersion, testKind, testNamespace, testName)
	u.Object["spec"] = spec
	return u
}
