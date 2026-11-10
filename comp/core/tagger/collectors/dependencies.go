// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// dependencyIndex records, for each tagged entity, the entities its handler
// looked up in the store, so that a change to any of them can re-tag it.
type dependencyIndex struct {
	readers map[workloadmeta.EntityID]map[workloadmeta.EntityID]struct{}
	tagged  map[workloadmeta.EntityID]taggedEntity
}

type taggedEntity struct {
	reads      map[workloadmeta.EntityID]struct{}
	isComplete bool
}

func newDependencyIndex() *dependencyIndex {
	return &dependencyIndex{
		readers: make(map[workloadmeta.EntityID]map[workloadmeta.EntityID]struct{}),
		tagged:  make(map[workloadmeta.EntityID]taggedEntity),
	}
}

// replace sets the entities that reader depends on, dropping the previous
// ones. isComplete is the completeness of the event reader was tagged from,
// reused when a dependency re-tags it. The index takes ownership of reads.
func (d *dependencyIndex) replace(reader workloadmeta.EntityID, reads map[workloadmeta.EntityID]struct{}, isComplete bool) {
	d.forget(reader)
	if len(reads) == 0 {
		return
	}

	d.tagged[reader] = taggedEntity{reads: reads, isComplete: isComplete}
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
	for read := range d.tagged[reader].reads {
		delete(d.readers[read], reader)
		if len(d.readers[read]) == 0 {
			delete(d.readers, read)
		}
	}
	delete(d.tagged, reader)
}

func (d *dependencyIndex) readersOf(read workloadmeta.EntityID) []workloadmeta.EntityID {
	readers := make([]workloadmeta.EntityID, 0, len(d.readers[read]))
	for reader := range d.readers[read] {
		readers = append(readers, reader)
	}
	return readers
}

func (d *dependencyIndex) isComplete(reader workloadmeta.EntityID) bool {
	return d.tagged[reader].isComplete
}

// trackingStore records the lookups made while tracking is on. Only the
// getters overridden below are recorded. Each one must look up an entity by
// its own ID, so that the recorded ID matches the ID of that entity's events.
type trackingStore struct {
	workloadmeta.Component

	tracking bool
	reads    map[workloadmeta.EntityID]struct{}
}

func (s *trackingStore) startTracking() {
	s.tracking = true
}

func (s *trackingStore) stopTracking() map[workloadmeta.EntityID]struct{} {
	reads := s.reads
	s.tracking = false
	s.reads = nil
	return reads
}

func (s *trackingStore) track(kind workloadmeta.Kind, id string) {
	if !s.tracking {
		return
	}
	if s.reads == nil {
		s.reads = make(map[workloadmeta.EntityID]struct{})
	}
	s.reads[workloadmeta.EntityID{Kind: kind, ID: id}] = struct{}{}
}

func (s *trackingStore) GetKubernetesKueueQueue(id string) (*workloadmeta.KubernetesKueueQueue, error) {
	s.track(workloadmeta.KindKubernetesKueueQueue, id)
	return s.Component.GetKubernetesKueueQueue(id)
}

func (s *trackingStore) GetKubernetesKueueResourceFlavor(id string) (*workloadmeta.KubernetesKueueResourceFlavor, error) {
	s.track(workloadmeta.KindKubernetesKueueResourceFlavor, id)
	return s.Component.GetKubernetesKueueResourceFlavor(id)
}

func (s *trackingStore) GetKubernetesKueueWorkload(id string) (*workloadmeta.KubernetesKueueWorkload, error) {
	s.track(workloadmeta.KindKubernetesKueueWorkload, id)
	return s.Component.GetKubernetesKueueWorkload(id)
}

func (s *trackingStore) GetKubernetesMetadata(id workloadmeta.KubeMetadataEntityID) (*workloadmeta.KubernetesMetadata, error) {
	s.track(workloadmeta.KindKubernetesMetadata, string(id))
	return s.Component.GetKubernetesMetadata(id)
}
