// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package registry

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

// The tests below cover the informers that trim objects before caching them, the split of ConfigMaps between the
// ConfigMap and the Helm release informers, and the deletion watch of terminated pods.

const (
	lastAppliedConfigAnnotation = "kubectl.kubernetes.io/last-applied-configuration"
	secretValue                 = "c3VwZXItc2VjcmV0" // base64("super-secret")
)

func TestConfigMapMetadataInformer(t *testing.T) {
	configMap := &metav1.PartialObjectMetadata{
		// The fake metadata client derives the resource from the Kind.
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "configmap",
			Namespace: "namespace",
			Annotations: map[string]string{
				"annotation":                "my-annotation",
				lastAppliedConfigAnnotation: `{"apiVersion":"v1","data":{"key":"secret-value"},"kind":"ConfigMap"}`,
			},
			ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kubectl", Operation: metav1.ManagedFieldsOperationApply}},
		},
	}
	// Helm release ConfigMaps are listed by the Helm release informer instead.
	helmRelease := &metav1.PartialObjectMetadata{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{Name: "sh.helm.release.v1.my-app.v1", Namespace: "namespace", Labels: map[string]string{"owner": "helm"}},
	}

	informer := startInformer(t, []runtime.Object{configMap, helmRelease}, configMapMetadataInformer, configMapGVR)

	assert.Equal(t, []string{"namespace/configmap"}, informer.GetStore().ListKeys())
	cached := getCached[*metav1.PartialObjectMetadata](t, informer, "namespace/configmap")
	assert.Nil(t, cached.ManagedFields)
	assert.Equal(t, "-", cached.Annotations[lastAppliedConfigAnnotation])
	assert.Equal(t, "my-annotation", cached.Annotations["annotation"])
}

func TestHelmReleaseConfigMapInformer(t *testing.T) {
	helmRelease := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "sh.helm.release.v1.my-app.v2",
			Namespace:   "namespace",
			Labels:      map[string]string{"owner": "helm", "name": "my-app"},
			Annotations: map[string]string{lastAppliedConfigAnnotation: `{"apiVersion":"v1","data":{"key":"value"},"kind":"ConfigMap"}`},
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "helm", Operation: metav1.ManagedFieldsOperationUpdate},
			},
		},
		Data:       map[string]string{"release": "H4sIAAAAAAAA/encoded-release"},
		BinaryData: map[string][]byte{"blob": []byte("binary")},
	}
	otherConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "not-helm", Namespace: "namespace"},
		Data:       map[string]string{"key": "value"},
	}

	informer := startInformer(t, []runtime.Object{helmRelease, otherConfigMap}, helmReleaseConfigMapInformer, schema.GroupVersionResource{})

	// Only Helm releases reach the cache, with their data, trimmed like regular ConfigMaps.
	assert.Equal(t, []string{"namespace/sh.helm.release.v1.my-app.v2"}, informer.GetStore().ListKeys())
	cached := getCached[*corev1.ConfigMap](t, informer, "namespace/sh.helm.release.v1.my-app.v2")
	assert.Equal(t, helmRelease.Data, cached.Data)
	assert.Equal(t, helmRelease.BinaryData, cached.BinaryData)
	assert.Nil(t, cached.ManagedFields)
	assert.Equal(t, "-", cached.Annotations[lastAppliedConfigAnnotation])
}

func TestSecretMetadataInformer(t *testing.T) {
	informer := startInformer(t, []runtime.Object{newTestSecret()}, secretMetadataInformer, secretGVR)

	// The transform deletes sensitive annotations before objects are stored in the cache.
	cached := getCached[*metav1.PartialObjectMetadata](t, informer, "namespace/secret")
	assert.Nil(t, cached.ManagedFields)
	assert.NotContains(t, cached.Annotations, lastAppliedConfigAnnotation)
	assert.Equal(t, "my-annotation", cached.Annotations["annotation"])
	assert.Equal(t, "my-app", cached.Labels["app"])
}

// TestSecretMetadataInformerFailsClosed checks that the Secret informer returns an error when the cache transform
// cannot be registered, which happens when the shared informer was already started.
func TestSecretMetadataInformerFailsClosed(t *testing.T) {
	runCfg, start := NewTestRunConfig(t, []runtime.Object{newTestSecret()})
	started := runCfg.OrchestratorInformerFactory.SecretMetadataInformerFactory.ForResource(secretGVR).Informer()
	start()
	require.True(t, cache.WaitForCacheSync(runCfg.StopCh, started.HasSynced))

	informer, err := secretMetadataInformer(runCfg, secretGVR, nil)
	assert.Error(t, err)
	assert.Nil(t, informer)
}

func TestTrimSecretMetadata(t *testing.T) {
	secret := newTestSecret()
	secret.Annotations["consul.hashicorp.com/original-pod"] = "pod-spec"
	secret.Labels[lastAppliedConfigAnnotation] = "label-value"

	obj, err := trimSecretMetadata(secret)
	require.NoError(t, err)

	trimmed, ok := obj.(*metav1.PartialObjectMetadata)
	require.True(t, ok)
	assert.Nil(t, trimmed.ManagedFields)
	assert.Equal(t, map[string]string{"annotation": "my-annotation"}, trimmed.Annotations)
	assert.Equal(t, map[string]string{"app": "my-app"}, trimmed.Labels)
}

// TestPodDeletionInformer checks that the informer of terminated pods reports each deleted pod of its watch, and
// ignores the other events.
func TestPodDeletionInformer(t *testing.T) {
	fakeWatcher := watch.NewFakeWithOptions(watch.FakeOptions{ChannelSize: 10})
	client := fake.NewClientset()
	client.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &corev1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: "1"}}, nil
	})
	client.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, fakeWatcher, nil
	})
	runCfg, _ := NewTestRunConfig(t, nil)
	runCfg.APIClient.Cl = client

	deleted := make(chan *corev1.Pod, 10)
	informer, err := podDeletionWatcher(runCfg, schema.GroupVersionResource{}, func(obj any) { deleted <- obj.(*corev1.Pod) })
	require.NoError(t, err)
	assert.Nil(t, informer)

	pod := func(name string) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID(name), ResourceVersion: "2"}}
	}
	fakeWatcher.Add(pod("added"))
	fakeWatcher.Modify(pod("modified"))
	fakeWatcher.Delete(pod("deleted"))

	select {
	case got := <-deleted:
		assert.Equal(t, "deleted", got.Name)
	case <-time.After(5 * time.Second):
		t.Fatal("the deleted pod was not reported")
	}
	assert.Never(t, func() bool { return len(deleted) > 0 }, 500*time.Millisecond, 50*time.Millisecond)
}

// TestPodDeletionInformerIsShared checks that the collectors of both bundles share one pod watch and both receive its
// deletions.
func TestPodDeletionInformerIsShared(t *testing.T) {
	fakeWatcher := watch.NewFakeWithOptions(watch.FakeOptions{ChannelSize: 10})
	client := fake.NewClientset()
	client.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &corev1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: "1"}}, nil
	})
	var watches atomic.Int32
	client.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		watches.Add(1)
		return true, fakeWatcher, nil
	})
	runCfg, _ := NewTestRunConfig(t, nil)
	runCfg.APIClient.Cl = client

	metadata := make(chan *corev1.Pod, 10)
	manifests := make(chan *corev1.Pod, 10)
	_, err := podDeletionWatcher(runCfg, schema.GroupVersionResource{}, func(obj any) { metadata <- obj.(*corev1.Pod) })
	require.NoError(t, err)
	_, err = podDeletionWatcher(runCfg, schema.GroupVersionResource{}, func(obj any) { manifests <- obj.(*corev1.Pod) })
	require.NoError(t, err)

	fakeWatcher.Delete(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "deleted", Namespace: "default", ResourceVersion: "2"}})
	for _, deleted := range []chan *corev1.Pod{metadata, manifests} {
		select {
		case got := <-deleted:
			assert.Equal(t, "deleted", got.Name)
		case <-time.After(5 * time.Second):
			t.Fatal("the deleted pod was not reported to every bundle")
		}
	}
	assert.EqualValues(t, 1, watches.Load())
}

// startInformer creates an informer of the registry on fake clients that serve the objects, and waits for its cache.
func startInformer(t *testing.T, objects []runtime.Object, newInformer InformerFunc, gvr schema.GroupVersionResource) cache.SharedIndexInformer {
	runCfg, start := NewTestRunConfig(t, objects)
	informer, err := newInformer(runCfg, gvr, nil)
	require.NoError(t, err)
	start()
	require.True(t, cache.WaitForCacheSync(runCfg.StopCh, informer.HasSynced))
	return informer
}

func getCached[T any](t *testing.T, informer cache.SharedIndexInformer, key string) T {
	obj, exists, err := informer.GetStore().GetByKey(key)
	require.NoError(t, err)
	require.True(t, exists, "%s is not in the cache", key)
	return obj.(T)
}

func newTestSecret() *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		// The fake metadata client derives the resource from the Kind.
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "namespace",
			Labels:    map[string]string{"app": "my-app"},
			Annotations: map[string]string{
				"annotation":                "my-annotation",
				lastAppliedConfigAnnotation: `{"apiVersion":"v1","data":{"password":"` + secretValue + `"},"kind":"Secret"}`,
			},
			ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kubectl", Operation: metav1.ManagedFieldsOperationApply}},
		},
	}
}
