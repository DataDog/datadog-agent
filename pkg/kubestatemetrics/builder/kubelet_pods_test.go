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

func TestUpdateStore_AddPodToStore(t *testing.T) {
	store := new(MockStore)
	podWatcher := new(MockPodWatcher)

	kubeletPod := &kubelet.Pod{
		Metadata: kubelet.PodMetadata{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "12345",
		},
	}

	kubernetesPod := kubelet.ConvertKubeletPodToK8sPod(kubeletPod)

	podWatcher.On("PullChanges", mock.Anything).Return([]*kubelet.Pod{kubeletPod}, nil)
	podWatcher.On("Expire").Return([]string{}, nil)
	store.On("Add", kubernetesPod).Return(nil)

	err := updateStore(context.TODO(), store, podWatcher, "default")
	assert.NoError(t, err)

	store.AssertCalled(t, "Add", kubernetesPod)
}

func TestUpdateStore_FilterPodsByNamespace(t *testing.T) {
	store := new(MockStore)
	podWatcher := new(MockPodWatcher)

	kubeletPod := &kubelet.Pod{
		Metadata: kubelet.PodMetadata{
			Name:      "test-pod",
			Namespace: "other-namespace",
			UID:       "12345",
		},
	}

	store.On("Add", mock.Anything).Return(nil)
	podWatcher.On("PullChanges", mock.Anything).Return([]*kubelet.Pod{kubeletPod}, nil)
	podWatcher.On("Expire").Return([]string{}, nil)

	err := updateStore(context.TODO(), store, podWatcher, "default")
	assert.NoError(t, err)

	// Add() shouldn't be called because the pod is in a different namespace
	store.AssertNotCalled(t, "Add", mock.Anything)
}

func TestUpdateStore_HandleExpiredPods(t *testing.T) {
	store := new(MockStore)
	podWatcher := new(MockPodWatcher)
	podUID := "kubernetes_pod://pod-12345"
	kubernetesPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID: types.UID("pod-12345"),
		},
	}

	podWatcher.On("PullChanges", mock.Anything).Return([]*kubelet.Pod{}, nil)
	podWatcher.On("Expire").Return([]string{podUID}, nil)
	store.On("Delete", &kubernetesPod).Return(nil)

	err := updateStore(context.TODO(), store, podWatcher, "default")
	assert.NoError(t, err)

	store.AssertCalled(t, "Delete", &kubernetesPod)
}

func TestUpdateStore_HandleExpiredContainers(t *testing.T) {
	store := new(MockStore)
	podWatcher := new(MockPodWatcher)

	podWatcher.On("PullChanges", mock.Anything).Return([]*kubelet.Pod{}, nil)
	podWatcher.On("Expire").Return([]string{"container-12345"}, nil)

	err := updateStore(context.TODO(), store, podWatcher, "default")
	assert.NoError(t, err)

	// Delete() shouldn't be called because the expired entity is not a pod
	store.AssertNotCalled(t, "Delete", mock.Anything)
}
