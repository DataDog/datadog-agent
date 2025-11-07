// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package snmpscanmanagerimpl

import (
	"container/heap"
	"time"

	snmpscanmanager "github.com/DataDog/datadog-agent/comp/snmpscanmanager/def"
)

type scanScheduler interface {
	QueueScan(scanTask scanTask)
	PopDueScans(now time.Time) []snmpscanmanager.ScanRequest
}

type scanSchedulerImpl struct {
	scanQueue scanPriorityQueue
}

type scanPriorityQueue []*scanTask

type scanTask struct {
	req     snmpscanmanager.ScanRequest
	nextRun time.Time
}

func newScanScheduler(scanQueue scanPriorityQueue) scanScheduler {
	sc := &scanSchedulerImpl{
		scanQueue: scanQueue,
	}
	heap.Init(&sc.scanQueue)
	return sc
}

func (sc *scanSchedulerImpl) QueueScan(scanTask scanTask) {
	heap.Push(&sc.scanQueue, &scanTask)
}

func (sc *scanSchedulerImpl) PopDueScans(now time.Time) []snmpscanmanager.ScanRequest {
	var dueScans []snmpscanmanager.ScanRequest
	for sc.scanQueue.Len() > 0 {
		nextTask := sc.scanQueue[0]
		if nextTask.nextRun.After(now) {
			break
		}

		dueScanTask := heap.Pop(&sc.scanQueue).(*scanTask)
		dueScans = append(dueScans, dueScanTask.req)
	}
	return dueScans
}

func (pq scanPriorityQueue) Len() int {
	return len(pq)
}

func (pq scanPriorityQueue) Less(i1, i2 int) bool {
	return pq[i1].nextRun.Before(pq[i2].nextRun)
}

func (pq scanPriorityQueue) Swap(i1, i2 int) {
	pq[i1], pq[i2] = pq[i2], pq[i1]
}

func (pq *scanPriorityQueue) Push(x interface{}) {
	*pq = append(*pq, x.(*scanTask))
}

func (pq *scanPriorityQueue) Pop() interface{} {
	old := *pq
	*pq = old[:len(old)-1]
	return old[len(old)-1]
}
