// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package schedulerevents

import (
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// eventIndex counts the events sent for each workload. The count goes in an
// event tag, so that events of the same workload that look alike, such as two
// evictions for the same reason, are not merged by the aggregation key. It is
// kept in the persistent cache, so that the count continues after a restart.
//
// The index of an event is the larger of the persisted count and the current
// time in unix seconds, not a count from zero. A new leader has no count for the
// workloads the previous leader reported, and a replica that leads again has an
// old one. Either would give its events the same index, and so the same
// aggregation key and tags, as the events of the leader in between. The index
// only has to grow, so the clock keeps it increasing across leaders.
type eventIndex struct {
	cacheKey string
	counters map[string]int
	dirty    bool
	loaded   bool

	now   func() time.Time
	read  func(key string) (string, error)
	write func(key, value string) error
}

func newEventIndex(cacheKey string) *eventIndex {
	return &eventIndex{
		cacheKey: cacheKey,
		counters: map[string]int{},
		now:      time.Now,
		read:     persistentcache.Read,
		write:    persistentcache.WriteAtomic,
	}
}

func (i *eventIndex) load() {
	if i.loaded {
		return
	}
	i.loaded = true

	raw, err := i.read(i.cacheKey)
	if err != nil {
		log.Warnf("Cannot read the event index %q, the count of events per workload starts from zero: %v", i.cacheKey, err)
		return
	}
	if raw == "" {
		return
	}

	var stored map[string]int
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		log.Warnf("Cannot parse the event index %q, the count of events per workload starts from zero: %v", i.cacheKey, err)
		return
	}
	for uid, count := range stored {
		if count >= 0 {
			i.counters[uid] = count
		}
	}
}

// next returns the index of the next event of the workload.
func (i *eventIndex) next(uid string) int {
	i.load()
	// The clock keeps the index above the ones another leader sent meanwhile,
	// also when this replica leads again with a count it persisted long ago.
	index := max(i.counters[uid], int(i.now().Unix()))
	i.counters[uid] = index + 1
	i.dirty = true
	return index
}

// forget drops the count of a workload that does not exist anymore.
func (i *eventIndex) forget(uid string) {
	i.load()
	if _, found := i.counters[uid]; found {
		delete(i.counters, uid)
		i.dirty = true
	}
}

// retain drops the counts of the workloads for which active returns false.
func (i *eventIndex) retain(active func(uid string) bool) {
	i.load()
	for uid := range i.counters {
		if !active(uid) {
			delete(i.counters, uid)
			i.dirty = true
		}
	}
}

// persist writes the counts if they changed since the last call.
func (i *eventIndex) persist() {
	if !i.dirty {
		return
	}
	encoded, err := json.Marshal(i.counters)
	if err != nil {
		log.Warnf("Cannot encode the event index %q: %v", i.cacheKey, err)
		return
	}
	if err := i.write(i.cacheKey, string(encoded)); err != nil {
		log.Warnf("Cannot persist the event index %q, the count of events per workload restarts from zero after a restart: %v", i.cacheKey, err)
		return
	}
	i.dirty = false
}
