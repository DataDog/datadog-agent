// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build nodefilter

package nodefilter

import (
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// podStore implements cache.ReflectorStore. A pod fans out into several
// workloadmeta entities (one KubernetesPod plus one Container per container
// the runtime has created), so, unlike a plain cache.Store, it tracks the
// entity IDs it derived per pod UID to be able to unset all of them once the
// pod itself is deleted or disappears from a Replace.
type podStore struct {
	wlmetaStore                workloadmeta.Component
	collectEphemeralContainers bool

	mu   sync.Mutex
	seen map[types.UID][]workloadmeta.EntityID
}

func newPodStore(wlmetaStore workloadmeta.Component, collectEphemeralContainers bool) *podStore {
	return &podStore{
		wlmetaStore:                wlmetaStore,
		collectEphemeralContainers: collectEphemeralContainers,
		seen:                       make(map[types.UID][]workloadmeta.EntityID),
	}
}

// Add notifies the workloadmeta store with EventTypeSet events for the pod
// and its containers.
func (s *podStore) Add(obj interface{}) error {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("nodefilter pod store: unsupported object type %T", obj)
	}

	events := parsePod(pod, s.collectEphemeralContainers)
	entityIDs := entityIDsFromEvents(events)

	s.mu.Lock()
	previousEntityIDs := s.seen[pod.UID]
	s.seen[pod.UID] = entityIDs
	s.mu.Unlock()

	// A container that disappeared between two updates of the same pod (the
	// runtime recreated it under a new ID, e.g. on restart) never goes
	// through Delete, since the pod itself never disappears: unset it here.
	events = append(events, unsetEventsForEntityIDs(removedEntityIDs(previousEntityIDs, entityIDs))...)

	s.wlmetaStore.Notify(events)

	return nil
}

// Update notifies the workloadmeta store with EventTypeSet events for the pod
// and its containers.
func (s *podStore) Update(obj interface{}) error {
	return s.Add(obj)
}

// Delete notifies the workloadmeta store with EventTypeUnset events for the
// pod and every container previously derived from it.
func (s *podStore) Delete(obj interface{}) error {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("nodefilter pod store: unsupported object type %T", obj)
	}

	s.mu.Lock()
	entityIDs := s.seen[pod.UID]
	delete(s.seen, pod.UID)
	s.mu.Unlock()

	s.wlmetaStore.Notify(unsetEventsForEntityIDs(entityIDs))

	return nil
}

// Replace diffs the given list against what was previously seen, and unsets
// any pod (and its containers) that is no longer present.
func (s *podStore) Replace(list []interface{}, _ string) error {
	seenNow := make(map[types.UID][]workloadmeta.EntityID, len(list))
	var events []workloadmeta.CollectorEvent

	for _, obj := range list {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return fmt.Errorf("nodefilter pod store: unsupported object type %T", obj)
		}

		podEvents := parsePod(pod, s.collectEphemeralContainers)
		seenNow[pod.UID] = entityIDsFromEvents(podEvents)
		events = append(events, podEvents...)
	}

	s.mu.Lock()
	seenBefore := s.seen
	s.seen = seenNow
	s.mu.Unlock()

	for uid, previousEntityIDs := range seenBefore {
		currentEntityIDs, stillPresent := seenNow[uid]
		if !stillPresent {
			events = append(events, unsetEventsForEntityIDs(previousEntityIDs)...)
			continue
		}
		// The pod itself survived the replace, but one of its containers may
		// not have (e.g. restarted under a new ID): unset those too.
		events = append(events, unsetEventsForEntityIDs(removedEntityIDs(previousEntityIDs, currentEntityIDs))...)
	}

	s.wlmetaStore.Notify(events)

	return nil
}

// Resync is never called: the reflector is configured with a zero resync
// period, matching the kubeapiserver collector's own reflector_store.go.
func (s *podStore) Resync() error {
	panic("not implemented")
}

func entityIDsFromEvents(events []workloadmeta.CollectorEvent) []workloadmeta.EntityID {
	entityIDs := make([]workloadmeta.EntityID, 0, len(events))
	for _, event := range events {
		entityIDs = append(entityIDs, event.Entity.GetID())
	}
	return entityIDs
}

// removedEntityIDs returns the entity IDs present in previous but absent from
// current, e.g. a container an updated pod no longer reports because the
// runtime recreated it under a new ID.
func removedEntityIDs(previous, current []workloadmeta.EntityID) []workloadmeta.EntityID {
	if len(previous) == 0 {
		return nil
	}

	currentSet := make(map[workloadmeta.EntityID]struct{}, len(current))
	for _, entityID := range current {
		currentSet[entityID] = struct{}{}
	}

	var removed []workloadmeta.EntityID
	for _, entityID := range previous {
		if _, ok := currentSet[entityID]; !ok {
			removed = append(removed, entityID)
		}
	}
	return removed
}

func unsetEventsForEntityIDs(entityIDs []workloadmeta.EntityID) []workloadmeta.CollectorEvent {
	events := make([]workloadmeta.CollectorEvent, 0, len(entityIDs))
	for _, entityID := range entityIDs {
		entity, err := entityFromEntityID(entityID)
		if err != nil {
			log.Debugf("nodefilter pod store: %s", err)
			continue
		}

		events = append(events, workloadmeta.CollectorEvent{
			Type:   workloadmeta.EventTypeUnset,
			Source: workloadmeta.SourceNodeOrchestrator,
			Entity: entity,
		})
	}
	return events
}

// entityFromEntityID reconstructs a bare entity (EntityID only) suitable for
// an EventTypeUnset notification. nodefilter only ever produces pods and
// containers, so only those two kinds need to be handled here.
func entityFromEntityID(entityID workloadmeta.EntityID) (workloadmeta.Entity, error) {
	switch entityID.Kind {
	case workloadmeta.KindKubernetesPod:
		return &workloadmeta.KubernetesPod{EntityID: entityID}, nil
	case workloadmeta.KindContainer:
		return &workloadmeta.Container{EntityID: entityID}, nil
	}

	return nil, fmt.Errorf("unsupported entity kind: %s", entityID.Kind)
}
