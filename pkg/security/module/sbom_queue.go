// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/sbom"
)

// sbomQueue holds the latest unsent report of each workload by request ID, the
// container ID or "" for the host, as each report is a full snapshot.
type sbomQueue struct {
	mu      sync.Mutex
	reports map[string]*sbom.ScanResult
	order   []string // request IDs of reports, oldest first
	size    int

	// ready holds a token while reports wait.
	ready chan struct{}
}

func newSBOMQueue(size int) *sbomQueue {
	return &sbomQueue{
		reports: make(map[string]*sbom.ScanResult),
		size:    size,
		ready:   make(chan struct{}, 1),
	}
}

// push queues report, and reports whether it dropped the oldest report of
// another workload to make room.
func (q *sbomQueue) push(report *sbom.ScanResult) bool {
	q.mu.Lock()
	dropped := q.add(report, true)
	q.mu.Unlock()
	q.signal()
	return dropped
}

// requeue puts back the reports a failed send left, except those a newer
// report replaced since.
func (q *sbomQueue) requeue(reports []*sbom.ScanResult) {
	q.mu.Lock()
	for _, report := range reports {
		q.add(report, false)
	}
	q.mu.Unlock()
	q.signal()
}

// take returns the queued reports, oldest first, and empties the queue.
func (q *sbomQueue) take() []*sbom.ScanResult {
	q.mu.Lock()
	defer q.mu.Unlock()

	reports := make([]*sbom.ScanResult, 0, len(q.order))
	for _, id := range q.order {
		reports = append(reports, q.reports[id])
	}
	clear(q.reports)
	q.order = nil
	return reports
}

func (q *sbomQueue) add(report *sbom.ScanResult, replace bool) (dropped bool) {
	id := report.RequestID
	if _, ok := q.reports[id]; ok {
		if replace {
			q.reports[id] = report
		}
		return false
	}

	if len(q.order) >= q.size {
		delete(q.reports, q.order[0])
		q.order = q.order[1:]
		dropped = true
	}
	q.reports[id] = report
	q.order = append(q.order, id)
	return dropped
}

func (q *sbomQueue) signal() {
	select {
	case q.ready <- struct{}{}:
	default:
	}
}
