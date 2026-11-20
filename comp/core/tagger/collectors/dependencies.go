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
	reads   map[workloadmeta.EntityID]map[workloadmeta.EntityID]struct{}
}

func newDependencyIndex() *dependencyIndex {
	return &dependencyIndex{
		readers: make(map[workloadmeta.EntityID]map[workloadmeta.EntityID]struct{}),
		reads:   make(map[workloadmeta.EntityID]map[workloadmeta.EntityID]struct{}),
	}
}

// replace sets the entities that reader depends on, dropping the previous
// ones. The index takes ownership of reads.
func (d *dependencyIndex) replace(reader workloadmeta.EntityID, reads map[workloadmeta.EntityID]struct{}) {
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

func (d *dependencyIndex) readersOf(read workloadmeta.EntityID) []workloadmeta.EntityID {
	readers := make([]workloadmeta.EntityID, 0, len(d.readers[read]))
	for reader := range d.readers[read] {
		readers = append(readers, reader)
	}
	return readers
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

func (s *trackingStore) GetKubernetesKAIPodGroup(id string) (*workloadmeta.KubernetesKAIPodGroup, error) {
	s.track(workloadmeta.KindKubernetesKAIPodGroup, id)
	return s.Component.GetKubernetesKAIPodGroup(id)
}
