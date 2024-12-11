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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	kscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

var scheme = kscheme.Scheme

func Test_injectTagsFromLabels(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		pod         *corev1.Pod
		wantPodFunc func() corev1.Pod
		found       bool
		injected    bool
	}{
		{
			name:   "nominal case",
			labels: map[string]string{"tags.datadoghq.com/env": "dev", "tags.datadoghq.com/service": "dd-agent", "tags.datadoghq.com/version": "7"},
			pod:    common.FakePod("foo-pod"),
			wantPodFunc: func() corev1.Pod {
				pod := common.FakePod("foo-pod")
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, common.FakeEnvWithValue("DD_ENV", "dev"))
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, common.FakeEnvWithValue("DD_SERVICE", "dd-agent"))
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, common.FakeEnvWithValue("DD_VERSION", "7"))
				return *pod
			},
			found:    true,
			injected: true,
		},
		{
			name:   "no labels",
			labels: map[string]string{},
			pod:    common.FakePod("foo-pod"),
			wantPodFunc: func() corev1.Pod {
				pod := common.FakePod("foo-pod")
				return *pod
			},
			found:    false,
			injected: false,
		},
		{
			name:   "env only",
			labels: map[string]string{"tags.datadoghq.com/env": "dev"},
			pod:    common.FakePod("foo-pod"),
			wantPodFunc: func() corev1.Pod {
				pod := common.FakePod("foo-pod")
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, common.FakeEnvWithValue("DD_ENV", "dev"))
				return *pod
			},
			found:    true,
			injected: true,
		},
		{
			name:   "label found but not injected",
			labels: map[string]string{"tags.datadoghq.com/env": "dev"},
			pod:    common.FakePodWithEnv("foo-pod", "DD_ENV"),
			wantPodFunc: func() corev1.Pod {
				pod := common.FakePodWithEnv("foo-pod", "DD_ENV")
				return *pod
			},
			found:    true,
			injected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, injected := injectTagsFromLabels(tt.labels, tt.pod)
			assert.Equal(t, tt.found, found)
			assert.Equal(t, tt.injected, injected)
			assert.Len(t, tt.pod.Spec.Containers, 1)
			assert.Len(t, tt.wantPodFunc().Spec.Containers, 1)
			assert.ElementsMatch(t, tt.wantPodFunc().Spec.Containers[0].Env, tt.pod.Spec.Containers[0].Env)
		})
	}
}

func Test_injectTags(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		pod         *corev1.Pod
		wantPodFunc func() corev1.Pod
	}{
		{
			name: "tag labels and injection on",
			pod: common.WithLabels(
				common.FakePod("foo-pod"),
				map[string]string{
					"admission.datadoghq.com/enabled": "true",
					"tags.datadoghq.com/env":          "dev",
					"tags.datadoghq.com/service":      "dd-agent",
					"tags.datadoghq.com/version":      "7",
				},
			),
			wantPodFunc: func() corev1.Pod {
				pod := common.WithLabels(common.FakePod("foo-pod"), map[string]string{"admission.datadoghq.com/enabled": "true"})
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, common.FakeEnvWithValue("DD_ENV", "dev"))
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, common.FakeEnvWithValue("DD_SERVICE", "dd-agent"))
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, common.FakeEnvWithValue("DD_VERSION", "7"))
				return *pod
			},
		},
		{
			name: "no labels and injection on",
			pod:  common.WithLabels(common.FakePod("foo-pod"), map[string]string{"admission.datadoghq.com/enabled": "true"}),
			wantPodFunc: func() corev1.Pod {
				pod := common.WithLabels(common.FakePod("foo-pod"), map[string]string{"admission.datadoghq.com/enabled": "true"})
				return *pod
			},
		},
		{
			name: "env only and injection on",
			pod: common.WithLabels(
				common.FakePod("foo-pod"),
				map[string]string{"admission.datadoghq.com/enabled": "true", "tags.datadoghq.com/env": "dev"},
			),
			wantPodFunc: func() corev1.Pod {
				pod := common.WithLabels(common.FakePod("foo-pod"), map[string]string{"admission.datadoghq.com/enabled": "true"})
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, common.FakeEnvWithValue("DD_ENV", "dev"))
				return *pod
			},
		},
		{
			name: "tag label found but not injected, injection on",
			pod: common.WithLabels(
				common.FakePodWithEnv("foo-pod", "DD_ENV"),
				map[string]string{"admission.datadoghq.com/enabled": "true", "tags.datadoghq.com/env": "dev"},
			),
			wantPodFunc: func() corev1.Pod {
				pod := common.WithLabels(common.FakePodWithEnv("foo-pod", "DD_ENV"), map[string]string{"admission.datadoghq.com/enabled": "true"})
				return *pod
			},
		},
		{
			name: "tag label found but not injected, injection label not set",
			pod: common.WithLabels(
				common.FakePodWithEnv("foo-pod", "DD_ENV"),
				map[string]string{"tags.datadoghq.com/env": "dev"},
			),
			wantPodFunc: func() corev1.Pod {
				pod := common.FakePodWithEnv("foo-pod", "DD_ENV")
				return *pod
			},
		},
	}
	wmeta := fxutil.Test[workloadmeta.Component](t, core.MockBundle(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))
	datadogConfig := fxutil.Test[config.Component](t, core.MockBundle())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, _ := autoinstrumentation.NewInjectionFilter(datadogConfig)
			webhook := NewWebhook(wmeta, datadogConfig, filter)
			_, err := webhook.injectTags(tt.pod, "ns", nil)
			assert.NoError(t, err)
			assert.Len(t, tt.pod.Spec.Containers, 1)
			assert.Len(t, tt.wantPodFunc().Spec.Containers, 1)
			assert.ElementsMatch(t, tt.wantPodFunc().Spec.Containers[0].Env, tt.pod.Spec.Containers[0].Env)
		})
	}
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

func TestGetAndCacheOwner(t *testing.T) {
	ownerInfo := dummyInfo()
	kubeObj := newUnstructuredWithSpec(map[string]interface{}{"foo": "bar"})
	owner := newOwner(kubeObj)
	wmeta := fxutil.Test[workloadmeta.Component](t, core.MockBundle(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))
	config := fxutil.Test[config.Component](t, core.MockBundle())
	filter, _ := autoinstrumentation.NewInjectionFilter(config)
	webhook := NewWebhook(wmeta, config, filter)

	// Cache hit
	cache.Cache.Set(ownerInfo.buildID(testNamespace), owner, webhook.ownerCacheTTL)
	dc := fake.NewSimpleDynamicClient(scheme)
	obj, err := webhook.getAndCacheOwner(ownerInfo, testNamespace, dc)
	assert.NoError(t, err)
	assert.NotNil(t, obj)
	assert.Equal(t, owner, obj)
	assert.Len(t, dc.Actions(), 0)
	cache.Cache.Flush()

	// Cache miss
	dc = fake.NewSimpleDynamicClient(scheme, kubeObj)
	obj, err = webhook.getAndCacheOwner(ownerInfo, testNamespace, dc)
	assert.NoError(t, err)
	assert.NotNil(t, obj)
	assert.Equal(t, owner, obj)
	assert.Len(t, dc.Actions(), 1)
	cachedObj, found := cache.Cache.Get(ownerInfo.buildID(testNamespace))
	assert.True(t, found)
	assert.NotNil(t, cachedObj)
}

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
