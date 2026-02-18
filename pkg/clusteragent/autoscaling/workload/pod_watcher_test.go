// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestHandleSetEvent(t *testing.T) {
	pw := NewPodWatcher(nil, nil)
	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "p1",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "pod1",
			Namespace: "default",
		},
		Owners: []workloadmeta.KubernetesPodOwner{{Kind: kubernetes.ReplicaSetKind, Name: "deploymentName-766dbb7846"}},
		Ready:  true,
	}
	event := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: pod,
	}

	pw.HandleEvent(event)

	expectedOwner := NamespacedPodOwner{
		Namespace: "default",
		Kind:      kubernetes.DeploymentKind,
		Name:      "deploymentName",
	}
	pods := pw.GetPodsForOwner(expectedOwner)
	require.Len(t, pods, 1)
	assert.Equal(t, pod, pods[0])

	numReadyPods := pw.GetReadyPodsForOwner(expectedOwner)
	assert.Equal(t, int32(1), numReadyPods)
}

func TestHandleUnsetEvent(t *testing.T) {
	pw := NewPodWatcher(nil, nil)
	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "p1",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "pod1",
			Namespace: "default",
		},
		Owners: []workloadmeta.KubernetesPodOwner{{Kind: kubernetes.ReplicaSetKind, Name: "deploymentName-766dbb7846"}},
		Ready:  true,
	}
	setEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: pod,
	}
	unsetEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeUnset,
		Entity: pod,
	}

	expectedOwner := NamespacedPodOwner{
		Namespace: "default",
		Kind:      kubernetes.DeploymentKind,
		Name:      "deploymentName",
	}

	pw.HandleEvent(setEvent)

	numReadyPods := pw.GetReadyPodsForOwner(expectedOwner)
	assert.Equal(t, int32(1), numReadyPods)

	pw.HandleEvent(unsetEvent)

	pods := pw.GetPodsForOwner(expectedOwner)
	assert.Nil(t, pods)
	assert.NotNil(t, pw.podsPerPodOwner)

	numReadyPods = pw.GetReadyPodsForOwner(expectedOwner)
	assert.Equal(t, int32(0), numReadyPods)
}

func TestPodWatcherStartStop(t *testing.T) {
	wlm := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	pw := NewPodWatcher(wlm, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go pw.Run(ctx)
	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "p1",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "pod1",
			Namespace: "default",
		},
		Owners: []workloadmeta.KubernetesPodOwner{{Kind: kubernetes.ReplicaSetKind, Name: "deploymentName-766dbb7846"}},
		Ready:  false,
	}

	wlm.Set(pod)

	expectedOwner := NamespacedPodOwner{
		Namespace: "default",
		Kind:      kubernetes.DeploymentKind,
		Name:      "deploymentName",
	}

	assert.Eventuallyf(t, func() bool {
		pods := pw.GetPodsForOwner(expectedOwner)
		return pods != nil
	}, 5*time.Second, 200*time.Millisecond, "expected pod to be added to the pod watcher")
	newPods := pw.GetPodsForOwner(expectedOwner)
	require.Len(t, newPods, 1)
	assert.Equal(t, pod, newPods[0])
	cancel()

	numReadyPods := pw.GetReadyPodsForOwner(expectedOwner)
	assert.Equal(t, int32(0), numReadyPods)
	_, ok := pw.readyPodsPerPodOwner[expectedOwner]
	assert.False(t, ok)
}

func TestResolveNamespacedPodOwner(t *testing.T) {
	for _, tt := range []struct {
		name     string
		pod      *workloadmeta.KubernetesPod
		expected NamespacedPodOwner
		err      error
	}{
		{
			name: "pod owned by deployment",
			pod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Namespace: "default",
				},
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.ReplicaSetKind,
						Name: "datadog-agent-linux-cluster-agent-f64dd88",
					},
				},
			},
			expected: NamespacedPodOwner{
				Namespace: "default",
				Kind:      kubernetes.DeploymentKind,
				Name:      "datadog-agent-linux-cluster-agent",
			},
		},
		{
			name: "pod owned by daemonset",
			pod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Namespace: "default",
				},
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.DaemonSetKind,
						Name: "datadog-agent-f64dd88",
					},
				},
			},
			expected: NamespacedPodOwner{
				Namespace: "default",
				Kind:      kubernetes.DaemonSetKind,
				Name:      "datadog-agent-f64dd88",
			},
		},
		{
			name: "pod owned by replica set",
			pod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Namespace: "default",
				},
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.ReplicaSetKind,
						Name: "datadog-agent-linux-cluster-agent",
					},
				},
			},
			expected: NamespacedPodOwner{
				Namespace: "default",
				Kind:      kubernetes.ReplicaSetKind,
				Name:      "datadog-agent-linux-cluster-agent",
			},
		},
		{
			name: "pod owned by deployment directly",
			pod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Namespace: "default",
				},
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.DeploymentKind,
						Name: "datadog-agent-linux-cluster-agent",
					},
				},
			},
			expected: NamespacedPodOwner{
				Namespace: "default",
				Kind:      kubernetes.ReplicaSetKind,
				Name:      "datadog-agent-linux-cluster-agent",
			},
			err: errDeploymentNotValidOwner,
		},
		{
			name: "pod owned by replicaset managed by rollout",
			pod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Namespace: "default",
					Labels: map[string]string{
						kubernetes.ArgoRolloutLabelKey: "9b8dc4bd6",
					},
				},
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.ReplicaSetKind,
						Name: "my-rollout-9b8dc4bd6",
					},
				},
			},
			expected: NamespacedPodOwner{
				Namespace: "default",
				Kind:      kubernetes.RolloutKind,
				Name:      "my-rollout",
			},
		},
		{
			name: "pod owned by replicaset with rollout label but invalid name format",
			pod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Namespace: "default",
					Labels: map[string]string{
						kubernetes.ArgoRolloutLabelKey: "invalid",
					},
				},
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.ReplicaSetKind,
						Name: "invalid-name",
					},
				},
			},
			expected: NamespacedPodOwner{
				Namespace: "default",
				Kind:      kubernetes.ReplicaSetKind,
				Name:      "invalid-name",
			},
		},
		{
			name: "pod owned by statefulset",
			pod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Namespace: "default",
				},
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.StatefulSetKind,
						Name: "my-statefulset",
					},
				},
			},
			expected: NamespacedPodOwner{
				Namespace: "default",
				Kind:      kubernetes.StatefulSetKind,
				Name:      "my-statefulset",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res, err := resolveNamespacedPodOwner(tt.pod)
			if tt.err != nil {
				assert.Equal(t, tt.err, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, res)
			}
		})
	}
}
