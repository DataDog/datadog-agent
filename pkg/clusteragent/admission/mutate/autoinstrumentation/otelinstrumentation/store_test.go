// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package otelinstrumentation

import (
	"context"
	"testing"
	"time"

	otelv1alpha1 "github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

const (
	// The store polls its cache, so tests wait on propagation instead of synchronizing.
	eventuallyTimeout = 5 * time.Second
	eventuallyTick    = 10 * time.Millisecond
)

func newUnstructuredInstrumentation(namespace, name, endpoint string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": otelv1alpha1.GroupVersion.String(),
		"kind":       "Instrumentation",
		"metadata": map[string]interface{}{
			"namespace": namespace,
			"name":      name,
		},
		"spec": map[string]interface{}{
			"exporter": map[string]interface{}{
				"endpoint": endpoint,
			},
		},
	}}
}

func newFakeClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{InstrumentationGVR: "InstrumentationList"},
		objects...,
	)
}

// startStore runs a Store until the test ends and waits for its initial cache sync.
func startStore(t *testing.T, client dynamic.Interface) *Store {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	store := NewStore(client)
	go store.Run(ctx)

	require.Eventually(t, store.HasSynced, eventuallyTimeout, eventuallyTick,
		"store never finished its initial cache sync")
	return store
}

func TestStoreStaysInertWhenCRDIsAbsent(t *testing.T) {
	client := newFakeClient()
	client.PrependReactor("list", "instrumentations", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(InstrumentationGVR.GroupResource(), "")
	})

	// The CRD check runs once immediately, so cancelling the context is enough to make
	// Run give up without waiting out the backoff.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	store := NewStore(client)
	go func() {
		store.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(eventuallyTimeout):
		t.Fatal("Run did not return after the context was cancelled")
	}

	assert.False(t, store.HasSynced())
	cr, ok := store.Get("default", "my-instrumentation")
	assert.False(t, ok, "lookup must miss when the CRD is absent")
	assert.Nil(t, cr)
}

func TestStoreStaysInertWhenCRDAccessIsForbidden(t *testing.T) {
	client := newFakeClient()
	client.PrependReactor("list", "instrumentations", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(InstrumentationGVR.GroupResource(), "", nil)
	})

	// A forbidden CRD check is permanent, so Run returns without needing a deadline.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	store := NewStore(client)
	go func() {
		store.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(eventuallyTimeout):
		t.Fatal("Run did not give up on a non-retryable CRD check")
	}

	assert.False(t, store.HasSynced())
	_, ok := store.Get("default", "my-instrumentation")
	assert.False(t, ok)
}

func TestStoreMissesBeforeCacheSync(t *testing.T) {
	store := NewStore(newFakeClient())
	// A custom resource present in the cache is still invisible until the initial sync
	// completes, so that a partially populated cache cannot produce a wrong answer.
	store.crs[storeKey("default", "my-instrumentation")] = &otelv1alpha1.Instrumentation{}

	_, ok := store.Get("default", "my-instrumentation")
	assert.False(t, ok, "lookup must miss before the initial cache sync")
}

func TestStoreGet(t *testing.T) {
	client := newFakeClient(
		newUnstructuredInstrumentation("default", "my-instrumentation", "http://collector:4317"),
		newUnstructuredInstrumentation("other", "my-instrumentation", "http://other:4317"),
	)
	store := startStore(t, client)

	tests := []struct {
		name         string
		namespace    string
		crName       string
		wantFound    bool
		wantEndpoint string
	}{
		{
			name:         "hit",
			namespace:    "default",
			crName:       "my-instrumentation",
			wantFound:    true,
			wantEndpoint: "http://collector:4317",
		},
		{
			name:         "same name in another namespace is a different custom resource",
			namespace:    "other",
			crName:       "my-instrumentation",
			wantFound:    true,
			wantEndpoint: "http://other:4317",
		},
		{
			name:      "miss on unknown name",
			namespace: "default",
			crName:    "nope",
		},
		{
			name:      "miss on unknown namespace",
			namespace: "nope",
			crName:    "my-instrumentation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr, ok := store.Get(tt.namespace, tt.crName)
			require.Equal(t, tt.wantFound, ok)
			if !tt.wantFound {
				assert.Nil(t, cr)
				return
			}
			assert.Equal(t, tt.crName, cr.Name)
			assert.Equal(t, tt.namespace, cr.Namespace)
			assert.Equal(t, tt.wantEndpoint, cr.Spec.Exporter.Endpoint)
		})
	}
}

func TestStoreListNamespace(t *testing.T) {
	client := newFakeClient(
		newUnstructuredInstrumentation("default", "b-second", "http://b:4317"),
		newUnstructuredInstrumentation("default", "a-first", "http://a:4317"),
		newUnstructuredInstrumentation("other", "elsewhere", "http://other:4317"),
	)
	store := startStore(t, client)

	t.Run("returns every custom resource in the namespace, sorted by name", func(t *testing.T) {
		crs, serving := store.ListNamespace("default")

		require.True(t, serving)
		require.Len(t, crs, 2)
		// Sorted so that the exactly-one rule and its diagnostics are deterministic
		// despite the underlying map.
		assert.Equal(t, "a-first", crs[0].Name)
		assert.Equal(t, "b-second", crs[1].Name)
	})

	t.Run("does not leak across namespaces", func(t *testing.T) {
		crs, serving := store.ListNamespace("other")

		require.True(t, serving)
		require.Len(t, crs, 1)
		assert.Equal(t, "elsewhere", crs[0].Name)
	})

	t.Run("an empty namespace is serving with zero results", func(t *testing.T) {
		crs, serving := store.ListNamespace("empty-ns")

		assert.True(t, serving, "an empty namespace is not the same as an unavailable store")
		assert.Empty(t, crs)
	})
}

func TestStoreListNamespaceBeforeCacheSync(t *testing.T) {
	store := NewStore(newFakeClient())
	store.crs[storeKey("default", "my-instrumentation")] = &otelv1alpha1.Instrumentation{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "my-instrumentation"},
	}

	// Reporting not-serving rather than an empty list is what lets the resolver tell
	// "the store is not ready" apart from "this namespace has no custom resource".
	crs, serving := store.ListNamespace("default")

	assert.False(t, serving)
	assert.Nil(t, crs)
}

func TestStoreListNamespaceReflectsUpdates(t *testing.T) {
	client := newFakeClient()
	store := startStore(t, client)
	resource := client.Resource(InstrumentationGVR).Namespace("default")

	countIn := func(namespace string) int {
		crs, _ := store.ListNamespace(namespace)
		return len(crs)
	}

	require.Equal(t, 0, countIn("default"))

	_, err := resource.Create(t.Context(),
		newUnstructuredInstrumentation("default", "first", "http://a:4317"), metav1.CreateOptions{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return countIn("default") == 1 },
		eventuallyTimeout, eventuallyTick, "created custom resource never appeared in the namespace listing")

	_, err = resource.Create(t.Context(),
		newUnstructuredInstrumentation("default", "second", "http://b:4317"), metav1.CreateOptions{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return countIn("default") == 2 },
		eventuallyTimeout, eventuallyTick, "second custom resource never appeared in the namespace listing")

	require.NoError(t, resource.Delete(t.Context(), "first", metav1.DeleteOptions{}))
	require.Eventually(t, func() bool { return countIn("default") == 1 },
		eventuallyTimeout, eventuallyTick, "deleted custom resource never left the namespace listing")
}

func TestStoreTracksInformerEvents(t *testing.T) {
	client := newFakeClient()
	store := startStore(t, client)

	resource := client.Resource(InstrumentationGVR).Namespace("default")
	endpointOf := func() (string, bool) {
		cr, ok := store.Get("default", "my-instrumentation")
		if !ok {
			return "", false
		}
		return cr.Spec.Exporter.Endpoint, true
	}

	_, ok := endpointOf()
	require.False(t, ok, "store must start empty")

	created, err := resource.Create(t.Context(),
		newUnstructuredInstrumentation("default", "my-instrumentation", "http://collector:4317"),
		metav1.CreateOptions{})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		endpoint, ok := endpointOf()
		return ok && endpoint == "http://collector:4317"
	}, eventuallyTimeout, eventuallyTick, "add event never reached the store")

	require.NoError(t, unstructured.SetNestedField(created.Object, "http://updated:4317", "spec", "exporter", "endpoint"))
	_, err = resource.Update(t.Context(), created, metav1.UpdateOptions{})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		endpoint, ok := endpointOf()
		return ok && endpoint == "http://updated:4317"
	}, eventuallyTimeout, eventuallyTick, "update event never reached the store")

	require.NoError(t, resource.Delete(t.Context(), "my-instrumentation", metav1.DeleteOptions{}))

	require.Eventually(t, func() bool {
		_, ok := endpointOf()
		return !ok
	}, eventuallyTimeout, eventuallyTick, "delete event never reached the store")
}

func TestStoreRemoveHandlesTombstone(t *testing.T) {
	store := NewStore(newFakeClient())
	store.setSynced()

	obj := newUnstructuredInstrumentation("default", "my-instrumentation", "http://collector:4317")
	store.upsert(obj)
	_, ok := store.Get("default", "my-instrumentation")
	require.True(t, ok)

	store.remove(cache.DeletedFinalStateUnknown{Key: "default/my-instrumentation", Obj: obj})

	_, ok = store.Get("default", "my-instrumentation")
	assert.False(t, ok, "a tombstoned delete must evict the custom resource")
}

func TestStoreIgnoresUnconvertibleObjects(t *testing.T) {
	store := NewStore(newFakeClient())
	store.setSynced()

	store.upsert("not-a-kubernetes-object")
	store.remove(cache.DeletedFinalStateUnknown{Key: "default/nope", Obj: 42})

	assert.Empty(t, store.crs)
}

func TestInstrumentationFromObject(t *testing.T) {
	typed := &otelv1alpha1.Instrumentation{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "my-instrumentation"},
		Spec: otelv1alpha1.InstrumentationSpec{
			Exporter: otelv1alpha1.Exporter{Endpoint: "http://collector:4317"},
		},
	}

	t.Run("typed object is deep copied", func(t *testing.T) {
		cr, err := InstrumentationFromObject(typed)
		require.NoError(t, err)
		assert.Equal(t, "http://collector:4317", cr.Spec.Exporter.Endpoint)

		cr.Spec.Exporter.Endpoint = "mutated"
		assert.Equal(t, "http://collector:4317", typed.Spec.Exporter.Endpoint)
	})

	t.Run("unstructured object is converted", func(t *testing.T) {
		cr, err := InstrumentationFromObject(
			newUnstructuredInstrumentation("default", "my-instrumentation", "http://collector:4317"))
		require.NoError(t, err)
		assert.Equal(t, "my-instrumentation", cr.Name)
		assert.Equal(t, "http://collector:4317", cr.Spec.Exporter.Endpoint)
	})

	t.Run("unexpected type is rejected", func(t *testing.T) {
		_, err := InstrumentationFromObject(struct{}{})
		assert.Error(t, err)
	})
}
