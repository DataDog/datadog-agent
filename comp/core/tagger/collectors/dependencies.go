// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kueue"
)

// dependency is something a handler read: an entity looked up by its ID, or
// the entities of a kind found by a key, for lookups by reference such as the
// Kueue Workloads that claim a pod.
type dependency struct {
	kind workloadmeta.Kind
	id   string
	key  string
}

func entityDependency(id workloadmeta.EntityID) dependency {
	return dependency{kind: id.Kind, id: id.ID}
}

func keyDependency(kind workloadmeta.Kind, key string) dependency {
	return dependency{kind: kind, key: key}
}

// dependencyIndex records, for each tagged entity, what its handler read, so
// that a change to any of it can re-tag the entity.
type dependencyIndex struct {
	readers map[dependency]map[workloadmeta.EntityID]struct{}
	reads   map[workloadmeta.EntityID]map[dependency]struct{}
	// keys holds the keys each entity was last found by, so that its next
	// event also reaches the readers of the keys it no longer has.
	keys map[workloadmeta.EntityID][]string
}

func newDependencyIndex() *dependencyIndex {
	return &dependencyIndex{
		readers: make(map[dependency]map[workloadmeta.EntityID]struct{}),
		reads:   make(map[workloadmeta.EntityID]map[dependency]struct{}),
		keys:    make(map[workloadmeta.EntityID][]string),
	}
}

// replace sets what reader depends on, dropping the previous dependencies.
// The index takes ownership of reads.
func (d *dependencyIndex) replace(reader workloadmeta.EntityID, reads map[dependency]struct{}) {
	d.forget(reader)
	if len(reads) == 0 {
		return
	}

	d.reads[reader] = reads
	for read := range reads {
		readers, ok := d.readers[read]
		if !ok {
			readers = make(map[workloadmeta.EntityID]struct{})
			d.readers[read] = readers
		}
		readers[reader] = struct{}{}
	}
}

func (d *dependencyIndex) forget(reader workloadmeta.EntityID) {
	for read := range d.reads[reader] {
		delete(d.readers[read], reader)
		if len(d.readers[read]) == 0 {
			delete(d.readers, read)
		}
	}
	delete(d.reads, reader)
}

// changed returns the dependencies that an event changes: the entity itself,
// and the keys it was found by before the event and after it. keys are the
// keys of the entity in a set event. Workloadmeta sets an entity before
// unsetting it, so the keys of an unset entity are the remembered ones.
func (d *dependencyIndex) changed(ev workloadmeta.Event, keys []string) []dependency {
	id := ev.Entity.GetID()
	deps := []dependency{entityDependency(id)}
	for _, key := range d.keys[id] {
		deps = append(deps, keyDependency(id.Kind, key))
	}

	if ev.Type != workloadmeta.EventTypeSet || len(keys) == 0 {
		delete(d.keys, id)
		return deps
	}

	for _, key := range keys {
		deps = append(deps, keyDependency(id.Kind, key))
	}
	d.keys[id] = keys
	return deps
}

func (d *dependencyIndex) readersOf(read dependency) []workloadmeta.EntityID {
	readers := make([]workloadmeta.EntityID, 0, len(d.readers[read]))
	for reader := range d.readers[read] {
		readers = append(readers, reader)
	}
	return readers
}

// trackingStore records the lookups made while tracking is on. Only the
// getters overridden below are recorded. Each one must look up an entity by
// its own ID, so that the recorded ID matches the ID of that entity's events.
// Lookups by reference record a key with trackKey instead.
type trackingStore struct {
	workloadmeta.Component

	tracking bool
	reads    map[dependency]struct{}
}

func (s *trackingStore) startTracking() {
	s.tracking = true
}

func (s *trackingStore) stopTracking() map[dependency]struct{} {
	reads := s.reads
	s.tracking = false
	s.reads = nil
	return reads
}

func (s *trackingStore) record(dep dependency) {
	if !s.tracking {
		return
	}
	if s.reads == nil {
		s.reads = make(map[dependency]struct{})
	}
	s.reads[dep] = struct{}{}
}

func (s *trackingStore) track(kind workloadmeta.Kind, id string) {
	s.record(entityDependency(workloadmeta.EntityID{Kind: kind, ID: id}))
}

// trackKey records that the tags being computed depend on the entities of
// kind that dependencyKeys returns key for.
func (s *trackingStore) trackKey(kind workloadmeta.Kind, key string) {
	s.record(keyDependency(kind, key))
}

func (s *trackingStore) GetKubernetesKueueQueue(id string) (*workloadmeta.KubernetesKueueQueue, error) {
	s.track(workloadmeta.KindKubernetesKueueQueue, id)
	return s.Component.GetKubernetesKueueQueue(id)
}

func (s *trackingStore) GetKubernetesKueueResourceFlavor(id string) (*workloadmeta.KubernetesKueueResourceFlavor, error) {
	s.track(workloadmeta.KindKubernetesKueueResourceFlavor, id)
	return s.Component.GetKubernetesKueueResourceFlavor(id)
}

// GetKubernetesKueueWorkloadForPod records the Workload the pod refers to by
// name, or, for the pods that workloadmeta resolves by UID, the key of the
// Workloads that claim the pod, so that one claiming it later re-tags it. It
// follows the precedence of the lookup it wraps.
func (s *trackingStore) GetKubernetesKueueWorkloadForPod(pod *workloadmeta.KubernetesPod) (*workloadmeta.KubernetesKueueWorkload, error) {
	if name := kueue.WorkloadName(pod); name != "" {
		s.track(workloadmeta.KindKubernetesKueueWorkload, workloadmeta.GenerateKueueWorkloadEntityID(pod.Namespace, name))
	} else if kueue.IsManagedPod(pod) && pod.ID != "" {
		s.trackKey(workloadmeta.KindKubernetesKueueWorkload, kueueJobKey(pod.Namespace, pod.ID))
	}
	return s.Component.GetKubernetesKueueWorkloadForPod(pod)
}

func kueueJobKey(namespace, uid string) string {
	return namespace + "/" + uid
}

func (s *trackingStore) GetKubernetesKAIPodGroup(id string) (*workloadmeta.KubernetesKAIPodGroup, error) {
	s.track(workloadmeta.KindKubernetesKAIPodGroup, id)
	return s.Component.GetKubernetesKAIPodGroup(id)
}

// GetContainer records the lookup only when the container isn't there yet,
// so its arrival re-tags the reader, but its later changes don't.
func (s *trackingStore) GetContainer(id string) (*workloadmeta.Container, error) {
	container, err := s.Component.GetContainer(id)
	if err != nil {
		s.track(workloadmeta.KindContainer, id)
	}
	return container, err
}
