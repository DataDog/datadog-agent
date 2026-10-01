// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build nodefilter

package nodefilter

import (
	"fmt"
	"slices"
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
	nodeName                   string
	collectEphemeralContainers bool

	mu   sync.Mutex
	seen map[types.UID][]workloadmeta.EntityID
}

func newPodStore(wlmetaStore workloadmeta.Component, nodeName string, collectEphemeralContainers bool) *podStore {
	return &podStore{
		wlmetaStore:                wlmetaStore,
		nodeName:                   nodeName,
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

	s.wlmetaStore.Notify(s.track(pod))

	return nil
}

// track records the entities pod yields and returns the events that bring
// workloadmeta up to date with it: a Set for the pod and each of its
// containers, and an Unset for each container it no longer reports. Such a
// container (the runtime recreated it under a new ID, e.g. on restart) never
// goes through Delete, since the pod itself never disappears.
func (s *podStore) track(pod *corev1.Pod) []workloadmeta.CollectorEvent {
	events := parsePod(pod, s.collectEphemeralContainers)
	entityIDs := entityIDsFromEvents(events)

	s.mu.Lock()
	previousEntityIDs := s.seen[pod.UID]
	s.seen[pod.UID] = entityIDs
	s.mu.Unlock()

	return append(events, unsetEventsForEntityIDs(removedEntityIDs(previousEntityIDs, entityIDs))...)
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
//
// The list holds every pod on the node, the agent's own included, so an
// empty one means the node name doesn't match the node the agent runs on.
// The API server accepts a field selector on any node name, so this is the
// only sign of it. Replace only runs on the reflector's (re)lists, so warning
// here doesn't flood the log.
func (s *podStore) Replace(list []interface{}, _ string) error {
	if len(list) == 0 {
		log.Warnf("%s found no pods on node %q, not even the agent's own, so telemetry won't get Kubernetes tags: set one of the environment variables listed in otelcollector.standalone.node_from_env_var to the pod's spec.nodeName through the downward API", componentName, s.nodeName)
	}

	var events []workloadmeta.CollectorEvent
	listed := make(map[types.UID]struct{}, len(list))

	// A pod that survived the replace may have lost containers (e.g. restarted
	// under a new ID), which track unsets.
	for _, obj := range list {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return fmt.Errorf("nodefilter pod store: unsupported object type %T", obj)
		}

		listed[pod.UID] = struct{}{}
		events = append(events, s.track(pod)...)
	}

	// A pod missing from the list is gone, along with its containers.
	s.mu.Lock()
	for uid, entityIDs := range s.seen {
		if _, ok := listed[uid]; !ok {
			events = append(events, unsetEventsForEntityIDs(entityIDs)...)
			delete(s.seen, uid)
		}
	}
	s.mu.Unlock()

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
	return slices.DeleteFunc(slices.Clone(previous), func(entityID workloadmeta.EntityID) bool {
		return slices.Contains(current, entityID)
	})
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
