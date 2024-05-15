// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package workload

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestHandleSetEvent(t *testing.T) {
	pw := newPodWatcher(nil).(*podWatcher)
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
	}
	event := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: pod,
	}

	pw.handleEvent(event)

	expectedOwner := NamespacedPodOwner{
		Namespace: "default",
		Kind:      kubernetes.DeploymentKind,
		Name:      "deploymentName",
	}
	pods := pw.GetPodsForOwner(expectedOwner)
	require.Len(t, pods, 1)
	assert.Equal(t, pod, pods[0])
}

func TestHandleUnsetEvent(t *testing.T) {
	pw := newPodWatcher(nil).(*podWatcher)
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
	}
	setEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: pod,
	}
	unsetEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeUnset,
		Entity: pod,
	}

	pw.handleEvent(setEvent)
	pw.handleEvent(unsetEvent)

	pods := pw.GetPodsForOwner(NamespacedPodOwner{
		Namespace: "default",
		Kind:      kubernetes.DeploymentKind,
		Name:      "deploymentName",
	})
	assert.Nil(t, pods)
	assert.NotNil(t, pw.podsPerPodOwner)
}

func TestPodWatcherStartStop(t *testing.T) {
	wlm := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		logimpl.MockModule(),
		config.MockModule(),
		fx.Supply(context.Background()),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
	))
	pw := newPodWatcher(wlm)
	ctx, cancel := context.WithCancel(context.Background())
	go pw.Start(ctx)
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
}

func TestGetNamespacedPodOwner(t *testing.T) {
	for _, tt := range []struct {
		name     string
		ns       string
		owner    *workloadmeta.KubernetesPodOwner
		expected NamespacedPodOwner
	}{
		{
			name: "pod owned by deployment",
			ns:   "default",
			owner: &workloadmeta.KubernetesPodOwner{
				Kind: kubernetes.ReplicaSetKind,
				Name: "datadog-agent-linux-cluster-agent-f64dd88",
			},
			expected: NamespacedPodOwner{
				Namespace: "default",
				Kind:      kubernetes.DeploymentKind,
				Name:      "datadog-agent-linux-cluster-agent",
			},
		},
		{
			name: "pod owned by daemonset",
			ns:   "default",
			owner: &workloadmeta.KubernetesPodOwner{
				Kind: kubernetes.DaemonSetKind,
				Name: "datadog-agent-f64dd88",
			},
			expected: NamespacedPodOwner{
				Namespace: "default",
				Kind:      kubernetes.DaemonSetKind,
				Name:      "datadog-agent-f64dd88",
			},
		},
		{
			name: "pod owned by replica set",
			ns:   "default",
			owner: &workloadmeta.KubernetesPodOwner{
				Kind: kubernetes.ReplicaSetKind,
				Name: "datadog-agent-linux-cluster-agent",
			},
			expected: NamespacedPodOwner{
				Namespace: "default",
				Kind:      kubernetes.ReplicaSetKind,
				Name:      "datadog-agent-linux-cluster-agent",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := getNamespacedPodOwner(tt.ns, tt.owner)
			assert.Equal(t, tt.expected, res)
		})
	}
}
