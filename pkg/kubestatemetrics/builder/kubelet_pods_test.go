// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package builder

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

type MockPodWatcher struct {
	mock.Mock
}

func (m *MockPodWatcher) PullChanges(ctx context.Context) ([]*kubelet.Pod, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*kubelet.Pod), args.Error(1)
}

func (m *MockPodWatcher) Expire() ([]string, error) {
	args := m.Called()
	return args.Get(0).([]string), args.Error(1)
}

type MockStore struct {
	mock.Mock
}

func (m *MockStore) Add(obj interface{}) error {
	args := m.Called(obj)
	return args.Error(0)
}

func (m *MockStore) Delete(obj interface{}) error {
	args := m.Called(obj)
	return args.Error(0)
}

func (m *MockStore) Update(_ interface{}) error {
	// Unused in this test
	return nil
}

func (m *MockStore) List() []interface{} {
	// Unused in this test
	return nil
}

func (m *MockStore) ListKeys() []string {
	// Unused in this test
	return nil
}

func (m *MockStore) Get(_ interface{}) (item interface{}, exists bool, err error) {
	// Unused in this test
	return nil, false, nil
}

func (m *MockStore) GetByKey(_ string) (item interface{}, exists bool, err error) {
	// Unused in this test
	return nil, false, nil
}

func (m *MockStore) Replace(_ []interface{}, _ string) error {
	// Unused in this test
	return nil
}

func (m *MockStore) Resync() error {
	// Unused in this test
	return nil
}

func TestUpdateStores_AddPods(t *testing.T) {
	stores := []*MockStore{
		new(MockStore),
		new(MockStore),
	}

	watcher := new(MockPodWatcher)

	kubeletPod := &kubelet.Pod{
		Metadata: kubelet.PodMetadata{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "12345",
		},
	}

	kubernetesPod := kubelet.ConvertKubeletPodToK8sPod(kubeletPod)

	watcher.On("PullChanges", mock.Anything).Return([]*kubelet.Pod{kubeletPod}, nil)
	watcher.On("Expire").Return([]string{}, nil)

	for _, store := range stores {
		store.On("Add", kubernetesPod).Return(nil)
	}

	reflector := kubeletReflector{
		namespaces: []string{"default"},
		podWatcher: watcher,
	}
	for _, store := range stores {
		err := reflector.addStore(store)
		assert.NoError(t, err)
	}

	err := reflector.updateStores(context.TODO())
	assert.NoError(t, err)

	for _, store := range stores {
		store.AssertCalled(t, "Add", kubernetesPod)
	}
}

func TestUpdateStores_FilterPodsByNamespace(t *testing.T) {
	stores := []*MockStore{
		new(MockStore),
		new(MockStore),
	}

	for _, store := range stores {
		store.On("Add", mock.Anything).Return(nil)
	}

	watcher := new(MockPodWatcher)

	reflector := kubeletReflector{
		namespaces: []string{"default"},
		podWatcher: watcher,
	}
	for _, store := range stores {
		err := reflector.addStore(store)
		assert.NoError(t, err)
	}

	kubeletPod := &kubelet.Pod{
		Metadata: kubelet.PodMetadata{
			Name:      "test-pod",
			Namespace: "other-namespace",
			UID:       "12345",
		},
	}

	watcher.On("PullChanges", mock.Anything).Return([]*kubelet.Pod{kubeletPod}, nil)
	watcher.On("Expire").Return([]string{}, nil)

	err := reflector.updateStores(context.TODO())
	assert.NoError(t, err)

	// Add() shouldn't be called because the pod is in a different namespace
	for _, store := range stores {
		store.AssertNotCalled(t, "Add", mock.Anything)
	}
}

func TestUpdateStores_WatchAllNamespaces(t *testing.T) {
	stores := []*MockStore{
		new(MockStore),
		new(MockStore),
	}

	for _, store := range stores {
		store.On("Add", mock.Anything).Return(nil)
	}

	watcher := new(MockPodWatcher)

	reflector := kubeletReflector{
		watchAllNamespaces: true,
		podWatcher:         watcher,
	}
	for _, store := range stores {
		err := reflector.addStore(store)
		assert.NoError(t, err)
	}

	kubeletPod := &kubelet.Pod{
		Metadata: kubelet.PodMetadata{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "12345",
		},
	}

	kubernetesPod := kubelet.ConvertKubeletPodToK8sPod(kubeletPod)

	watcher.On("PullChanges", mock.Anything).Return([]*kubelet.Pod{kubeletPod}, nil)
	watcher.On("Expire").Return([]string{}, nil)

	err := reflector.updateStores(context.TODO())
	assert.NoError(t, err)

	for _, store := range stores {
		store.AssertCalled(t, "Add", kubernetesPod)
	}
}

func TestUpdateStores_HandleExpiredPods(t *testing.T) {
	stores := []*MockStore{
		new(MockStore),
		new(MockStore),
	}

	watcher := new(MockPodWatcher)

	podUID := "kubernetes_pod://pod-12345"
	kubernetesPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID: types.UID("pod-12345"),
		},
	}

	watcher.On("PullChanges", mock.Anything).Return([]*kubelet.Pod{}, nil)
	watcher.On("Expire").Return([]string{podUID}, nil)

	for _, store := range stores {
		store.On("Delete", &kubernetesPod).Return(nil)
	}

	reflector := kubeletReflector{
		namespaces: []string{"default"},
		podWatcher: watcher,
	}
	for _, store := range stores {
		err := reflector.addStore(store)
		assert.NoError(t, err)
	}

	err := reflector.updateStores(context.TODO())
	assert.NoError(t, err)

	for _, store := range stores {
		store.AssertCalled(t, "Delete", &kubernetesPod)
	}
}

func TestUpdateStores_HandleExpiredContainers(t *testing.T) {
	stores := []*MockStore{
		new(MockStore),
		new(MockStore),
	}

	watcher := new(MockPodWatcher)

	watcher.On("PullChanges", mock.Anything).Return([]*kubelet.Pod{}, nil)
	watcher.On("Expire").Return([]string{"container-12345"}, nil)

	reflector := kubeletReflector{
		namespaces: []string{"default"},
		podWatcher: watcher,
	}
	for _, store := range stores {
		err := reflector.addStore(store)
		assert.NoError(t, err)
	}

	err := reflector.updateStores(context.TODO())
	assert.NoError(t, err)

	// Delete() shouldn't be called because the expired entity is not a pod
	for _, store := range stores {
		store.AssertNotCalled(t, "Delete", mock.Anything)
	}
}
