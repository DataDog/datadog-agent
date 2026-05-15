// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/contlcycle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// ownerStore is a configurable stub for container owner lookups.
type ownerStore struct {
	workloadmeta.Component
	owners map[string]*workloadmeta.EntityID
}

func (s *ownerStore) GetContainer(id string) (*workloadmeta.Container, error) {
	if owner, ok := s.owners[id]; ok {
		return &workloadmeta.Container{Owner: owner}, nil
	}
	return &workloadmeta.Container{}, nil
}

func storeWithOwner(containerID string, owner *workloadmeta.EntityID) *ownerStore {
	return &ownerStore{owners: map[string]*workloadmeta.EntityID{containerID: owner}}
}

func emptyStore() *ownerStore { return &ownerStore{} }

// TestContainerTerminationHandlerCanHandle tests the CanHandle method for the ContainerTerminationHandler.
//
// Test partitions:
// - event type: unset | set
// - entity kind: container | non-container
func TestContainerTerminationHandlerCanHandle(t *testing.T) {
	h := NewContainerTerminationHandler(emptyStore())
	cont := &workloadmeta.Container{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer}}
	pod := &workloadmeta.KubernetesPod{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod}}

	tests := []struct {
		subdomain string
		ev        workloadmeta.Event
		want      bool
	}{
		{"event type unset + entity kind container", workloadmeta.Event{Type: workloadmeta.EventTypeUnset, Entity: cont}, true},
		{"event type set + entity kind non-container", workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: pod}, false},
	}
	for _, tt := range tests {
		t.Run(tt.subdomain, func(t *testing.T) {
			assert.Equal(t, tt.want, h.CanHandle(tt.ev))
		})
	}
}

// TestContainerTerminationHandlerHandle tests the Handle method for the ContainerTerminationHandler.
//
// Test partitions:
// - entity type: *Container (correct) | other (wrong)
// - exit state: none | has timestamp and exit code
// - owner kind: none | pod | task | unknown (ignored)
func TestContainerTerminationHandlerHandle(t *testing.T) {
	now := time.Now()
	exitCode := int64(1)

	t.Run("entity type wrong", func(t *testing.T) {
		h := NewContainerTerminationHandler(emptyStore())
		ev := workloadmeta.Event{
			Type:   workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.KubernetesPod{EntityID: workloadmeta.EntityID{ID: "ben-bitdiddle"}},
		}
		_, err := h.Handle(ev)
		assert.Error(t, err)
	})

	t.Run("exit state none + owner kind none", func(t *testing.T) {
		h := NewContainerTerminationHandler(emptyStore())
		ev := workloadmeta.Event{
			Type:   workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.Container{EntityID: workloadmeta.EntityID{ID: "alyssa-hacker", Kind: workloadmeta.KindContainer}},
		}
		les, err := h.Handle(ev)
		require.NoError(t, err)
		require.Len(t, les, 1)
		ctr := les[0].ProtoEvent.GetContainer()
		assert.Equal(t, "alyssa-hacker", ctr.GetContainerID())
		assert.Nil(t, ctr.OptionalExitCode)
		assert.Nil(t, ctr.OptionalExitTimestamp)
		assert.Nil(t, ctr.Owner)
	})

	t.Run("exit state has timestamp and exit code + owner kind pod", func(t *testing.T) {
		h := NewContainerTerminationHandler(storeWithOwner("lem-tweakit", &workloadmeta.EntityID{
			ID: "pod-uid", Kind: workloadmeta.KindKubernetesPod,
		}))
		ev := workloadmeta.Event{
			Type: workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{ID: "lem-tweakit", Kind: workloadmeta.KindContainer},
				State:    workloadmeta.ContainerState{FinishedAt: now, ExitCode: &exitCode},
			},
		}
		les, err := h.Handle(ev)
		require.NoError(t, err)
		ctr := les[0].ProtoEvent.GetContainer()
		assert.Equal(t, now.Unix(), ctr.GetExitTimestamp())
		assert.Equal(t, int32(1), ctr.GetExitCode())
		require.NotNil(t, ctr.Owner)
		assert.Equal(t, model.ObjectKind_Pod, ctr.Owner.OwnerType)
		assert.Equal(t, "pod-uid", ctr.Owner.OwnerUID)
	})

	t.Run("owner kind task", func(t *testing.T) {
		h := NewContainerTerminationHandler(storeWithOwner("louis-reasoner", &workloadmeta.EntityID{
			ID: "task-arn", Kind: workloadmeta.KindECSTask,
		}))
		ev := workloadmeta.Event{
			Type:   workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.Container{EntityID: workloadmeta.EntityID{ID: "louis-reasoner", Kind: workloadmeta.KindContainer}},
		}
		les, err := h.Handle(ev)
		require.NoError(t, err)
		require.NotNil(t, les[0].ProtoEvent.GetContainer().Owner)
		assert.Equal(t, model.ObjectKind_Task, les[0].ProtoEvent.GetContainer().Owner.OwnerType)
		assert.Equal(t, "task-arn", les[0].ProtoEvent.GetContainer().Owner.OwnerUID)
	})

	t.Run("owner kind unknown", func(t *testing.T) {
		h := NewContainerTerminationHandler(storeWithOwner("eva-lu-ator", &workloadmeta.EntityID{
			ID: "???", Kind: "unknown",
		}))
		ev := workloadmeta.Event{
			Type:   workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.Container{EntityID: workloadmeta.EntityID{ID: "eva-lu-ator", Kind: workloadmeta.KindContainer}},
		}
		les, err := h.Handle(ev)
		require.NoError(t, err)
		assert.Nil(t, les[0].ProtoEvent.GetContainer().Owner)
	})
}

// TestPodTerminationHandlerCanHandle tests the CanHandle method for the PodTerminationHandler.
//
// Test partitions:
// - event type: unset | set
// - entity kind: pod | non-pod
func TestPodTerminationHandlerCanHandle(t *testing.T) {
	h := &PodTerminationHandler{}
	pod := &workloadmeta.KubernetesPod{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod}}
	cont := &workloadmeta.Container{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer}}

	tests := []struct {
		subdomain string
		ev        workloadmeta.Event
		want      bool
	}{
		{"event type unset + entity kind pod", workloadmeta.Event{Type: workloadmeta.EventTypeUnset, Entity: pod}, true},
		{"event type set + entity kind non-pod", workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: cont}, false},
	}
	for _, tt := range tests {
		t.Run(tt.subdomain, func(t *testing.T) {
			assert.Equal(t, tt.want, h.CanHandle(tt.ev))
		})
	}
}

// TestPodTerminationHandlerHandle tests the Handle method for the PodTerminationHandler.
//
// Test partitions:
// - entity type: *KubernetesPod (correct) | other (wrong)
// - finish time: zero | non-zero
func TestPodTerminationHandlerHandle(t *testing.T) {
	h := &PodTerminationHandler{}
	now := time.Now()

	t.Run("entity type wrong", func(t *testing.T) {
		ev := workloadmeta.Event{
			Type:   workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.Container{EntityID: workloadmeta.EntityID{ID: "alyssa-hacker"}},
		}
		_, err := h.Handle(ev)
		assert.Error(t, err)
	})

	t.Run("finish time zero", func(t *testing.T) {
		ev := workloadmeta.Event{
			Type:   workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.KubernetesPod{EntityID: workloadmeta.EntityID{ID: "ben-bitdiddle", Kind: workloadmeta.KindKubernetesPod}},
		}
		les, err := h.Handle(ev)
		require.NoError(t, err)
		require.Len(t, les, 1)
		pod := les[0].ProtoEvent.GetPod()
		assert.Equal(t, "ben-bitdiddle", pod.GetPodUID())
		assert.Nil(t, pod.ExitTimestamp)
	})

	t.Run("finish time non-zero", func(t *testing.T) {
		ev := workloadmeta.Event{
			Type: workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.KubernetesPod{
				EntityID:   workloadmeta.EntityID{ID: "lem-tweakit", Kind: workloadmeta.KindKubernetesPod},
				FinishedAt: now,
			},
		}
		les, err := h.Handle(ev)
		require.NoError(t, err)
		assert.Equal(t, now.Unix(), les[0].ProtoEvent.GetPod().GetExitTimestamp())
	})
}

// TestTaskTerminationHandlerCanHandle tests the CanHandle method for the TaskTerminationHandler.
//
// Test partitions:
// - event type: unset | set
// - entity kind: task | non-task
func TestTaskTerminationHandlerCanHandle(t *testing.T) {
	h := &TaskTerminationHandler{}
	task := &workloadmeta.ECSTask{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindECSTask}}
	cont := &workloadmeta.Container{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer}}

	tests := []struct {
		subdomain string
		ev        workloadmeta.Event
		want      bool
	}{
		{"event type unset + entity kind task", workloadmeta.Event{Type: workloadmeta.EventTypeUnset, Entity: task}, true},
		{"event type set + entity kind non-task", workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: cont}, false},
	}
	for _, tt := range tests {
		t.Run(tt.subdomain, func(t *testing.T) {
			assert.Equal(t, tt.want, h.CanHandle(tt.ev))
		})
	}
}

// TestTaskTerminationHandlerHandle tests the Handle method for the TaskTerminationHandler.
//
// Test partitions:
// - entity type: *ECSTask (correct) | other (wrong)
// - launch type: Fargate (source=runtime) | non-Fargate (source=node-orchestrator)
func TestTaskTerminationHandlerHandle(t *testing.T) {
	h := &TaskTerminationHandler{}
	before := time.Now()

	t.Run("entity type wrong", func(t *testing.T) {
		ev := workloadmeta.Event{
			Type:   workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.Container{EntityID: workloadmeta.EntityID{ID: "ben-bitdiddle"}},
		}
		_, err := h.Handle(ev)
		assert.Error(t, err)
	})

	t.Run("launch type non-Fargate", func(t *testing.T) {
		ev := workloadmeta.Event{
			Type: workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.ECSTask{
				EntityID:   workloadmeta.EntityID{ID: "louis-reasoner", Kind: workloadmeta.KindECSTask},
				LaunchType: workloadmeta.ECSLaunchTypeEC2,
			},
		}
		les, err := h.Handle(ev)
		require.NoError(t, err)
		require.Len(t, les, 1)
		task := les[0].ProtoEvent.GetTask()
		assert.Equal(t, "louis-reasoner", task.GetTaskARN())
		assert.Equal(t, string(workloadmeta.SourceNodeOrchestrator), task.Source)
		assert.GreaterOrEqual(t, task.GetExitTimestamp(), before.Unix())
	})

	t.Run("launch type Fargate", func(t *testing.T) {
		ev := workloadmeta.Event{
			Type: workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.ECSTask{
				EntityID:   workloadmeta.EntityID{ID: "eva-lu-ator", Kind: workloadmeta.KindECSTask},
				LaunchType: workloadmeta.ECSLaunchTypeFargate,
			},
		}
		les, err := h.Handle(ev)
		require.NoError(t, err)
		assert.Equal(t, string(workloadmeta.SourceRuntime), les[0].ProtoEvent.GetTask().Source)
	})
}
