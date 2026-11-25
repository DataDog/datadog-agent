// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package schedulerevents

import "time"

// eventIndex gives each event of a workload an index. The index goes in an
// event tag, so that events of the same workload that look alike, such as two
// evictions for the same reason, are not merged by the aggregation key.
//
// It is kept in memory only. The index of an event is the larger of the last
// index of the workload plus one and the current time in unix seconds. The
// clock keeps the index above the ones another leader sent, or the ones sent
// before a restart, and the last index keeps two events of the same second
// apart. The index only has to grow, so it needs nothing from earlier runs.
type eventIndex struct {
	last map[string]int
	now  func() time.Time
}

func newEventIndex() *eventIndex {
	return &eventIndex{last: map[string]int{}, now: time.Now}
}

// next returns the index of the next event of the workload.
func (i *eventIndex) next(uid string) int {
	index := int(i.now().Unix())
	if previous, found := i.last[uid]; found {
		index = max(previous+1, index)
	}
	i.last[uid] = index
	return index
}

// forget drops the last index of a workload that does not exist anymore.
func (i *eventIndex) forget(uid string) {
	delete(i.last, uid)
}
