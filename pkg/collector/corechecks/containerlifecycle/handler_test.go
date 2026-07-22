// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"errors"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/contlcycle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// ownerStore is a configurable stub for container owner lookups. A container
// only "exists" in the store if it has an entry in owners, matching the real
// store's behavior of returning an error for containers it doesn't hold.
type ownerStore struct {
	workloadmeta.Component
	owners map[string]*workloadmeta.EntityID
}

func (s *ownerStore) GetContainer(id string) (*workloadmeta.Container, error) {
	if owner, ok := s.owners[id]; ok {
		return &workloadmeta.Container{Owner: owner}, nil
	}
	return nil, errors.New("container not found")
}

func storeWithOwner(containerID string, owner *workloadmeta.EntityID) *ownerStore {
	return &ownerStore{owners: map[string]*workloadmeta.EntityID{containerID: owner}}
}

func emptyStore() *ownerStore { return &ownerStore{} }

// TestContainerTerminationHandlerCanHandle tests the CanHandle method for the ContainerTerminationHandler.
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

// TestPodCreationHandlerCanHandle tests the CanHandle method for the PodCreationHandler.
func TestPodCreationHandlerCanHandle(t *testing.T) {
	h := NewPodCreationHandler()
	pod := &workloadmeta.KubernetesPod{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod}}
	cont := &workloadmeta.Container{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer}}

	tests := []struct {
		subdomain string
		ev        workloadmeta.Event
		want      bool
	}{
		{"event type set + entity kind pod", workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: pod}, true},
		{"event type unset + entity kind pod", workloadmeta.Event{Type: workloadmeta.EventTypeUnset, Entity: pod}, true},
		{"event type set + entity kind non-pod", workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: cont}, false},
	}
	for _, tt := range tests {
		t.Run(tt.subdomain, func(t *testing.T) {
			assert.Equal(t, tt.want, h.CanHandle(tt.ev))
		})
	}
}

// TestPodCreationHandlerHandle tests the Handle method for the PodCreationHandler.
func TestPodCreationHandlerHandle(t *testing.T) {
	now := time.Now()

	t.Run("entity type wrong", func(t *testing.T) {
		h := NewPodCreationHandler()
		ev := workloadmeta.Event{
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{EntityID: workloadmeta.EntityID{ID: "ben-bitdiddle"}},
		}
		_, err := h.Handle(ev)
		assert.Error(t, err)
	})

	t.Run("first set + creation timestamp zero", func(t *testing.T) {
		h := NewPodCreationHandler()
		ev := workloadmeta.Event{
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesPod{EntityID: workloadmeta.EntityID{ID: "alyssa-hacker", Kind: workloadmeta.KindKubernetesPod}},
		}
		les, err := h.Handle(ev)
		require.NoError(t, err)
		require.Len(t, les, 1)
		pod := les[0].ProtoEvent.GetPod()
		assert.Equal(t, "alyssa-hacker", pod.GetPodUID())
		assert.Nil(t, pod.CreationTimestamp)
	})

	t.Run("first set + creation timestamp non-zero", func(t *testing.T) {
		h := NewPodCreationHandler()
		ev := workloadmeta.Event{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesPod{
				EntityID:          workloadmeta.EntityID{ID: "lem-tweakit", Kind: workloadmeta.KindKubernetesPod},
				CreationTimestamp: now,
			},
		}
		les, err := h.Handle(ev)
		require.NoError(t, err)
		assert.Equal(t, now.Unix(), les[0].ProtoEvent.GetPod().GetCreationTimestamp())
	})

	t.Run("repeat set is deduplicated", func(t *testing.T) {
		h := NewPodCreationHandler()
		ev := workloadmeta.Event{
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesPod{EntityID: workloadmeta.EntityID{ID: "louis-reasoner", Kind: workloadmeta.KindKubernetesPod}},
		}
		_, err := h.Handle(ev)
		require.NoError(t, err)
		les, err := h.Handle(ev)
		require.NoError(t, err)
		assert.Nil(t, les)
	})

	t.Run("unset clears local state, allowing a later set to re-emit", func(t *testing.T) {
		h := NewPodCreationHandler()
		podID := workloadmeta.EntityID{ID: "eva-lu-ator", Kind: workloadmeta.KindKubernetesPod}
		setEv := workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: &workloadmeta.KubernetesPod{EntityID: podID}}
		_, err := h.Handle(setEv)
		require.NoError(t, err)

		unsetEv := workloadmeta.Event{Type: workloadmeta.EventTypeUnset, Entity: &workloadmeta.KubernetesPod{EntityID: podID}}
		les, err := h.Handle(unsetEv)
		require.NoError(t, err)
		assert.Nil(t, les)

		les, err = h.Handle(setEv)
		require.NoError(t, err)
		assert.Len(t, les, 1)
	})
}

// TestContainerCreationHandlerCanHandle tests the CanHandle method for the ContainerCreationHandler.
func TestContainerCreationHandlerCanHandle(t *testing.T) {
	h := NewContainerCreationHandler(emptyStore())
	cont := &workloadmeta.Container{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer}}
	pod := &workloadmeta.KubernetesPod{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod}}

	tests := []struct {
		subdomain string
		ev        workloadmeta.Event
		want      bool
	}{
		{"event type set + entity kind container", workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: cont}, true},
		{"event type unset + entity kind container", workloadmeta.Event{Type: workloadmeta.EventTypeUnset, Entity: cont}, true},
		{"event type set + entity kind non-container", workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: pod}, false},
	}
	for _, tt := range tests {
		t.Run(tt.subdomain, func(t *testing.T) {
			assert.Equal(t, tt.want, h.CanHandle(tt.ev))
		})
	}
}

// TestContainerCreationHandlerHandle tests the Handle method for the ContainerCreationHandler.
func TestContainerCreationHandlerHandle(t *testing.T) {
	now := time.Now()

	t.Run("entity type wrong", func(t *testing.T) {
		h := NewContainerCreationHandler(emptyStore())
		ev := workloadmeta.Event{
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesPod{EntityID: workloadmeta.EntityID{ID: "ben-bitdiddle"}},
		}
		_, err := h.Handle(ev)
		assert.Error(t, err)
	})

	t.Run("first set + owner kind none", func(t *testing.T) {
		h := NewContainerCreationHandler(emptyStore())
		ev := workloadmeta.Event{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{
				EntityID:   workloadmeta.EntityID{ID: "alyssa-hacker", Kind: workloadmeta.KindContainer},
				EntityMeta: workloadmeta.EntityMeta{Name: "alyssa-container"},
			},
		}
		les, err := h.Handle(ev)
		require.NoError(t, err)
		require.Len(t, les, 1)
		ctr := les[0].ProtoEvent.GetContainer()
		assert.Equal(t, "alyssa-hacker", ctr.GetContainerID())
		assert.Equal(t, "alyssa-container", ctr.GetContainerName())
		assert.Nil(t, ctr.OptionalCreationTimestamp)
		assert.Nil(t, ctr.Owner)
	})

	t.Run("first set + creation timestamp non-zero + owner kind pod", func(t *testing.T) {
		h := NewContainerCreationHandler(storeWithOwner("lem-tweakit", &workloadmeta.EntityID{
			ID: "pod-uid", Kind: workloadmeta.KindKubernetesPod,
		}))
		ev := workloadmeta.Event{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{ID: "lem-tweakit", Kind: workloadmeta.KindContainer},
				State:    workloadmeta.ContainerState{CreatedAt: now},
			},
		}
		les, err := h.Handle(ev)
		require.NoError(t, err)
		ctr := les[0].ProtoEvent.GetContainer()
		assert.Equal(t, now.Unix(), ctr.GetCreationTimestamp())
		require.NotNil(t, ctr.Owner)
		assert.Equal(t, model.ObjectKind_Pod, ctr.Owner.OwnerType)
		assert.Equal(t, "pod-uid", ctr.Owner.OwnerUID)
	})

	t.Run("repeat set is deduplicated", func(t *testing.T) {
		h := NewContainerCreationHandler(emptyStore())
		ev := workloadmeta.Event{
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{EntityID: workloadmeta.EntityID{ID: "louis-reasoner", Kind: workloadmeta.KindContainer}},
		}
		_, err := h.Handle(ev)
		require.NoError(t, err)
		les, err := h.Handle(ev)
		require.NoError(t, err)
		assert.Nil(t, les)
	})

	t.Run("unset clears local state, allowing a later set to re-emit", func(t *testing.T) {
		h := NewContainerCreationHandler(emptyStore())
		containerID := workloadmeta.EntityID{ID: "eva-lu-ator", Kind: workloadmeta.KindContainer}
		setEv := workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: &workloadmeta.Container{EntityID: containerID}}
		_, err := h.Handle(setEv)
		require.NoError(t, err)

		unsetEv := workloadmeta.Event{Type: workloadmeta.EventTypeUnset, Entity: &workloadmeta.Container{EntityID: containerID}}
		les, err := h.Handle(unsetEv)
		require.NoError(t, err)
		assert.Nil(t, les)

		les, err = h.Handle(setEv)
		require.NoError(t, err)
		assert.Len(t, les, 1)
	})

	t.Run("owner arrives after first set re-emits with owner", func(t *testing.T) {
		store := emptyStore()
		h := NewContainerCreationHandler(store)
		containerID := workloadmeta.EntityID{ID: "ben-bitdiddle", Kind: workloadmeta.KindContainer}
		ev := workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: &workloadmeta.Container{EntityID: containerID}}

		les, err := h.Handle(ev)
		require.NoError(t, err)
		require.Len(t, les, 1)
		assert.Nil(t, les[0].ProtoEvent.GetContainer().Owner)

		store.owners = map[string]*workloadmeta.EntityID{
			"ben-bitdiddle": {ID: "pod-uid", Kind: workloadmeta.KindKubernetesPod},
		}

		les, err = h.Handle(ev)
		require.NoError(t, err)
		require.Len(t, les, 1)
		require.NotNil(t, les[0].ProtoEvent.GetContainer().Owner)
		assert.Equal(t, "pod-uid", les[0].ProtoEvent.GetContainer().Owner.OwnerUID)

		les, err = h.Handle(ev)
		require.NoError(t, err)
		assert.Nil(t, les)
	})
}

// TestPodStateHandlerCanHandle tests the CanHandle method for the PodStateHandler.
func TestPodStateHandlerCanHandle(t *testing.T) {
	h := NewPodStateHandler()
	pod := &workloadmeta.KubernetesPod{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod}}
	cont := &workloadmeta.Container{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer}}

	tests := []struct {
		subdomain string
		ev        workloadmeta.Event
		want      bool
	}{
		{"event type set + entity kind pod", workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: pod}, true},
		{"event type unset + entity kind pod", workloadmeta.Event{Type: workloadmeta.EventTypeUnset, Entity: pod}, true},
		{"event type set + entity kind non-pod", workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: cont}, false},
	}
	for _, tt := range tests {
		t.Run(tt.subdomain, func(t *testing.T) {
			assert.Equal(t, tt.want, h.CanHandle(tt.ev))
		})
	}
}

// TestPodStateHandlerHandle tests the Handle method for the PodStateHandler.
func TestPodStateHandlerHandle(t *testing.T) {
	t.Run("entity type wrong", func(t *testing.T) {
		h := NewPodStateHandler()
		ev := workloadmeta.Event{
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{EntityID: workloadmeta.EntityID{ID: "ben-bitdiddle"}},
		}
		_, err := h.Handle(ev)
		assert.Error(t, err)
	})

	t.Run("first observation with no phase and no conditions emits nothing", func(t *testing.T) {
		h := NewPodStateHandler()
		ev := workloadmeta.Event{
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesPod{EntityID: workloadmeta.EntityID{ID: "alyssa-hacker", Kind: workloadmeta.KindKubernetesPod}},
		}
		les, err := h.Handle(ev)
		require.NoError(t, err)
		assert.Nil(t, les)
	})

	t.Run("first observation of phase and a condition reports unknown as the last-observed state", func(t *testing.T) {
		h := NewPodStateHandler()
		now := time.Now()
		ev := workloadmeta.Event{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{ID: "lem-tweakit", Kind: workloadmeta.KindKubernetesPod},
				Phase:    "Pending",
				Conditions: []workloadmeta.KubernetesPodCondition{
					{Type: "Ready", Status: "False", LastTransitionTime: now},
				},
			},
		}
		les, err := h.Handle(ev)
		require.NoError(t, err)
		require.Len(t, les, 2)

		phaseTransition := les[0].ProtoEvent.GetPod().GetTransition()
		assert.Equal(t, model.PodStatusField_POD_STATUS_FIELD_PHASE, phaseTransition.GetField())
		assert.Nil(t, phaseTransition.LastObservedState)
		assert.Equal(t, "Pending", phaseTransition.GetNewState().GetPhase())
		assert.Equal(t, model.Precision_PRECISION_APPROXIMATE, phaseTransition.GetPrecision())

		conditionTransition := les[1].ProtoEvent.GetPod().GetTransition()
		assert.Equal(t, model.PodStatusField_POD_STATUS_FIELD_CONDITION, conditionTransition.GetField())
		assert.Nil(t, conditionTransition.LastObservedState)
		assert.Equal(t, "False", conditionTransition.GetNewState().GetCondition().GetStatus())
		assert.Equal(t, model.Precision_PRECISION_EXACT, conditionTransition.GetPrecision())
		assert.Equal(t, now.Unix(), conditionTransition.GetTransitionTimestamp())
	})

	t.Run("phase change reports the prior phase as the last-observed state", func(t *testing.T) {
		h := NewPodStateHandler()
		podID := workloadmeta.EntityID{ID: "louis-reasoner", Kind: workloadmeta.KindKubernetesPod}
		_, err := h.Handle(workloadmeta.Event{
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesPod{EntityID: podID, Phase: "Pending"},
		})
		require.NoError(t, err)

		les, err := h.Handle(workloadmeta.Event{
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesPod{EntityID: podID, Phase: "Running"},
		})
		require.NoError(t, err)
		require.Len(t, les, 1)
		transition := les[0].ProtoEvent.GetPod().GetTransition()
		assert.Equal(t, "Pending", transition.GetLastObservedState().GetPhase())
		assert.Equal(t, "Running", transition.GetNewState().GetPhase())
	})

	t.Run("unchanged condition emits nothing", func(t *testing.T) {
		h := NewPodStateHandler()
		podID := workloadmeta.EntityID{ID: "eva-lu-ator", Kind: workloadmeta.KindKubernetesPod}
		now := time.Now()
		ev := workloadmeta.Event{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesPod{
				EntityID:   podID,
				Conditions: []workloadmeta.KubernetesPodCondition{{Type: "Ready", Status: "True", LastTransitionTime: now}},
			},
		}
		_, err := h.Handle(ev)
		require.NoError(t, err)

		les, err := h.Handle(ev)
		require.NoError(t, err)
		assert.Nil(t, les)
	})

	t.Run("condition status change is unknowable, and a same-status timestamp advance proves a missed intermediate", func(t *testing.T) {
		h := NewPodStateHandler()
		podID := workloadmeta.EntityID{ID: "ben-bitdiddle", Kind: workloadmeta.KindKubernetesPod}
		t0 := time.Now()

		_, err := h.Handle(workloadmeta.Event{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesPod{
				EntityID:   podID,
				Conditions: []workloadmeta.KubernetesPodCondition{{Type: "Ready", Status: "False", LastTransitionTime: t0}},
			},
		})
		require.NoError(t, err)

		t1 := t0.Add(time.Second)
		les, err := h.Handle(workloadmeta.Event{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesPod{
				EntityID:   podID,
				Conditions: []workloadmeta.KubernetesPodCondition{{Type: "Ready", Status: "True", LastTransitionTime: t1}},
			},
		})
		require.NoError(t, err)
		require.Len(t, les, 1)
		assert.Equal(t, model.MissedIntermediate_MISSED_INTERMEDIATE_UNKNOWABLE, les[0].ProtoEvent.GetPod().GetTransition().GetMissedIntermediate())

		t2 := t1.Add(time.Second)
		les, err = h.Handle(workloadmeta.Event{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesPod{
				EntityID:   podID,
				Conditions: []workloadmeta.KubernetesPodCondition{{Type: "Ready", Status: "True", LastTransitionTime: t2}},
			},
		})
		require.NoError(t, err)
		require.Len(t, les, 1)
		assert.Equal(t, model.MissedIntermediate_MISSED_INTERMEDIATE_PROVEN, les[0].ProtoEvent.GetPod().GetTransition().GetMissedIntermediate())
	})

	t.Run("a same-status timestamp advance proves a missed intermediate even when the reason also changed", func(t *testing.T) {
		h := NewPodStateHandler()
		podID := workloadmeta.EntityID{ID: "alyssa-hacker", Kind: workloadmeta.KindKubernetesPod}
		t0 := time.Now()

		_, err := h.Handle(workloadmeta.Event{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesPod{
				EntityID:   podID,
				Conditions: []workloadmeta.KubernetesPodCondition{{Type: "Ready", Status: "True", Reason: "PodReady", LastTransitionTime: t0}},
			},
		})
		require.NoError(t, err)

		t1 := t0.Add(time.Second)
		les, err := h.Handle(workloadmeta.Event{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesPod{
				EntityID:   podID,
				Conditions: []workloadmeta.KubernetesPodCondition{{Type: "Ready", Status: "True", Reason: "ContainersReady", LastTransitionTime: t1}},
			},
		})
		require.NoError(t, err)
		require.Len(t, les, 1)
		assert.Equal(t, model.MissedIntermediate_MISSED_INTERMEDIATE_PROVEN, les[0].ProtoEvent.GetPod().GetTransition().GetMissedIntermediate())
	})

	t.Run("unset clears shadow state, so a reappearing pod is treated as a first observation", func(t *testing.T) {
		h := NewPodStateHandler()
		podID := workloadmeta.EntityID{ID: "lem-tweakit", Kind: workloadmeta.KindKubernetesPod}

		_, err := h.Handle(workloadmeta.Event{
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesPod{EntityID: podID, Phase: "Running"},
		})
		require.NoError(t, err)

		les, err := h.Handle(workloadmeta.Event{Type: workloadmeta.EventTypeUnset, Entity: &workloadmeta.KubernetesPod{EntityID: podID}})
		require.NoError(t, err)
		assert.Nil(t, les)

		les, err = h.Handle(workloadmeta.Event{
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesPod{EntityID: podID, Phase: "Pending"},
		})
		require.NoError(t, err)
		require.Len(t, les, 1)
		assert.Nil(t, les[0].ProtoEvent.GetPod().GetTransition().LastObservedState)
		assert.Equal(t, "Pending", les[0].ProtoEvent.GetPod().GetTransition().GetNewState().GetPhase())
	})
}
