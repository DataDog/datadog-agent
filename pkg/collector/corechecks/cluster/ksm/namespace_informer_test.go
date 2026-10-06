// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

type recordingDynamicStore struct {
	mu      sync.Mutex
	added   map[string]int
	deleted map[string]int
}

func newRecordingDynamicStore() *recordingDynamicStore {
	return &recordingDynamicStore{
		added:   make(map[string]int),
		deleted: make(map[string]int),
	}
}

func (s *recordingDynamicStore) Add(namespace string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.added[namespace]++
}

func (s *recordingDynamicStore) Del(namespace string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted[namespace]++
}

func (s *recordingDynamicStore) Snapshot() []cache.Store {
	return nil
}

func (s *recordingDynamicStore) addCount(namespace string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.added[namespace]
}

func (s *recordingDynamicStore) deleteCount(namespace string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deleted[namespace]
}

func TestSharedNamespaceInformer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	client := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "existing"}})
	informer := newSharedNamespaceInformer(ctx)
	firstStore := newRecordingDynamicStore()
	secondStore := newRecordingDynamicStore()

	firstRegistration, err := informer.Subscribe(client, firstStore, []string{corev1.NamespaceAll})
	require.NoError(t, err)
	sharedInformer := informer.informer
	secondRegistration, err := informer.Subscribe(client, secondStore, []string{corev1.NamespaceAll})
	require.NoError(t, err)
	require.Same(t, sharedInformer, informer.informer)

	require.Eventually(t, func() bool {
		return firstRegistration.HasSynced() && secondRegistration.HasSynced() &&
			firstStore.addCount("existing") == 1 && secondStore.addCount("existing") == 1
	}, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, informer.Unsubscribe(firstRegistration))
	_, err = client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "new"}}, metav1.CreateOptions{})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return secondStore.addCount("new") == 1
	}, 5*time.Second, 10*time.Millisecond)
	require.Zero(t, firstStore.addCount("new"))

	require.NoError(t, client.CoreV1().Namespaces().Delete(ctx, "new", metav1.DeleteOptions{}))
	require.Eventually(t, func() bool {
		return secondStore.deleteCount("new") == 1
	}, 5*time.Second, 10*time.Millisecond)
}

func TestNamespaceEventHandlerFiltersConfiguredNamespaces(t *testing.T) {
	stores := newRecordingDynamicStore()
	handler := namespaceEventHandler(stores, []string{"selected"})

	handler.OnAdd(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "selected"}}, false)
	handler.OnAdd(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ignored"}}, false)
	handler.OnDelete(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "selected"}})
	handler.OnDelete(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ignored"}})

	require.Equal(t, 1, stores.addCount("selected"))
	require.Zero(t, stores.addCount("ignored"))
	require.Equal(t, 1, stores.deleteCount("selected"))
	require.Zero(t, stores.deleteCount("ignored"))
}
