// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package builder

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
	tests := []struct {
		name                string
		reflectorNamespaces []string
		addedPodNamespace   string
		podShouldBeAdded    bool
	}{
		{
			name:                "add pod in watched namespace",
			reflectorNamespaces: []string{"default"},
			addedPodNamespace:   "default",
			podShouldBeAdded:    true,
		},
		{
			name:                "add pod in non-watched namespace",
			reflectorNamespaces: []string{"default"},
			addedPodNamespace:   "other-namespace",
			podShouldBeAdded:    false,
		},
		{
			name:                "reflector watches all pods",
			reflectorNamespaces: []string{corev1.NamespaceAll},
			addedPodNamespace:   "default",
			podShouldBeAdded:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stores := []*MockStore{
				new(MockStore),
				new(MockStore),
			}
			for _, store := range stores {
				store.On("Add", mock.Anything).Return(nil)
			}

			watcher := new(MockPodWatcher)

			kubeletPod := &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Namespace: test.addedPodNamespace,
					Name:      "test-pod",
					UID:       "12345",
				},
			}

			kubernetesPod := kubelet.ConvertKubeletPodToK8sPod(kubeletPod)

			watcher.On("PullChanges", mock.Anything).Return([]*kubelet.Pod{kubeletPod}, nil)
			watcher.On("Expire").Return([]string{}, nil)

			reflector := kubeletReflector{
				namespaces:         test.reflectorNamespaces,
				watchAllNamespaces: slices.Contains(test.reflectorNamespaces, corev1.NamespaceAll),
				podWatcher:         watcher,
			}

			for _, store := range stores {
				err := reflector.addStore(store)
				require.NoError(t, err)
			}

			err := reflector.updateStores(context.TODO())
			require.NoError(t, err)

			if test.podShouldBeAdded {
				for _, store := range stores {
					store.AssertCalled(t, "Add", kubernetesPod)
				}
			} else {
				for _, store := range stores {
					store.AssertNotCalled(t, "Add", mock.Anything)
				}
			}
		})
	}
}

func TestUpdateStores_HandleExpired(t *testing.T) {
	tests := []struct {
		name                   string
		expiredUID             string
		expectedPodToBeDeleted *corev1.Pod
	}{
		{
			name:       "expired pod",
			expiredUID: "kubernetes_pod://pod-12345",
			expectedPodToBeDeleted: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("pod-12345"),
				},
			},
		},
		{
			name:                   "expired container",
			expiredUID:             "container-12345",
			expectedPodToBeDeleted: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stores := []*MockStore{
				new(MockStore),
				new(MockStore),
			}
			for _, store := range stores {
				store.On("Delete", mock.Anything).Return(nil)
			}

			watcher := new(MockPodWatcher)
			watcher.On("PullChanges", mock.Anything).Return([]*kubelet.Pod{}, nil)
			watcher.On("Expire").Return([]string{test.expiredUID}, nil)

			reflector := kubeletReflector{
				namespaces: []string{"default"},
				podWatcher: watcher,
			}
			for _, store := range stores {
				err := reflector.addStore(store)
				require.NoError(t, err)
			}

			err := reflector.updateStores(context.TODO())
			require.NoError(t, err)

			for _, store := range stores {
				if test.expectedPodToBeDeleted != nil {
					store.AssertCalled(t, "Delete", test.expectedPodToBeDeleted)
				} else {
					store.AssertNotCalled(t, "Delete", mock.Anything)
				}
			}
		})
	}
}
