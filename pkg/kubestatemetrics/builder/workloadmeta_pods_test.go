// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package builder

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type MockKubeStore struct {
	mock.Mock
}

func (m *MockKubeStore) Add(obj interface{}) error {
	args := m.Called(obj)
	return args.Error(0)
}

func (m *MockKubeStore) Delete(obj interface{}) error {
	args := m.Called(obj)
	return args.Error(0)
}

func (m *MockKubeStore) Update(_ interface{}) error {
	// Unused in this test
	return nil
}

func (m *MockKubeStore) List() []interface{} {
	// Unused in this test
	return nil
}

func (m *MockKubeStore) ListKeys() []string {
	// Unused in this test
	return nil
}

func (m *MockKubeStore) Get(_ interface{}) (item interface{}, exists bool, err error) {
	// Unused in this test
	return nil, false, nil
}

func (m *MockKubeStore) GetByKey(_ string) (item interface{}, exists bool, err error) {
	// Unused in this test
	return nil, false, nil
}

func (m *MockKubeStore) Replace(_ []interface{}, _ string) error {
	// Unused in this test
	return nil
}

func (m *MockKubeStore) Resync() error {
	// Unused in this test
	return nil
}

func TestProcessEventBundle_SetEvent(t *testing.T) {
	tests := []struct {
		name                string
		reflectorNamespaces []string
		podNamespace        string
		podShouldBeAdded    bool
	}{
		{
			name:                "add pod in watched namespace",
			reflectorNamespaces: []string{"default"},
			podNamespace:        "default",
			podShouldBeAdded:    true,
		},
		{
			name:                "add pod in non-watched namespace",
			reflectorNamespaces: []string{"default"},
			podNamespace:        "some-namespace",
			podShouldBeAdded:    false,
		},
		{
			name:                "reflector watches all pods",
			reflectorNamespaces: []string{corev1.NamespaceAll},
			podNamespace:        "some-namespace",
			podShouldBeAdded:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wmetaMock := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				core.MockBundle(),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			reflector, err := newWorkloadmetaReflector(wmetaMock, test.reflectorNamespaces)
			require.NoError(t, err)

			// Define 2 to test that it works with multiple stores
			stores := []*MockKubeStore{
				new(MockKubeStore),
				new(MockKubeStore),
			}

			for _, store := range stores {
				store.On("Add", mock.Anything).Return(nil)
				err = reflector.addStore(store)
				require.NoError(t, err)
			}

			container := &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   "test-container-id",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "test-container-name",
					Namespace: test.podNamespace,
				},
			}

			pod := &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "12345",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "test-pod-name",
					Namespace: test.podNamespace,
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "test-container-id",
						Name: "test-container-name",
						Image: workloadmeta.ContainerImage{
							Name: "test-image",
						},
					},
				},
			}

			wmetaMock.Set(container)
			wmetaMock.Set(pod)

			wmetaEvents := workloadmeta.EventBundle{
				Events: []workloadmeta.Event{
					{
						Type:   workloadmeta.EventTypeSet,
						Entity: pod,
					},
				},
				Ch: make(chan struct{}),
			}

			reflector.processEventBundle(wmetaEvents)

			if !test.podShouldBeAdded {
				for _, store := range stores {
					store.AssertNotCalled(t, "Add", mock.Anything)
				}
				return
			}

			// Verify the stores were called
			for _, store := range stores {
				store.AssertCalled(t, "Add", mock.MatchedBy(func(pod *corev1.Pod) bool {
					return pod.Name == "test-pod-name" && pod.Namespace == test.podNamespace
				}))
			}
		})
	}
}

func TestProcessEventBundle_UnsetEvent(t *testing.T) {
	wmetaMock := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	reflector, err := newWorkloadmetaReflector(wmetaMock, []string{"default"})
	require.NoError(t, err)

	// Define 2 to test that it works with multiple stores
	stores := []*MockKubeStore{
		new(MockKubeStore),
		new(MockKubeStore),
	}

	for _, store := range stores {
		store.On("Delete", mock.Anything).Return(nil)
		err = reflector.addStore(store)
		require.NoError(t, err)
	}

	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "12345",
		},
	}

	wmetaEvents := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Type:   workloadmeta.EventTypeUnset,
				Entity: pod,
			},
		},
		Ch: make(chan struct{}),
	}

	reflector.processEventBundle(wmetaEvents)

	podToBeDeleted := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID: types.UID("12345"),
		},
	}

	for _, store := range stores {
		store.AssertCalled(t, "Delete", podToBeDeleted)
	}
}
