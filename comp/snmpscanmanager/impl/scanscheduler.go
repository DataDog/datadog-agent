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
	QueueScanTask(scanTask scanTask)
	PopDueScans(now time.Time) []snmpscanmanager.ScanRequest
}

type scanSchedulerImpl struct {
	taskQueue scanTaskPriorityQueue
}

type scanTaskPriorityQueue []*scanTask

type scanTask struct {
	req     snmpscanmanager.ScanRequest
	nextRun time.Time
}

func newScanScheduler(taskQueue scanTaskPriorityQueue) scanScheduler {
	if taskQueue == nil {
		taskQueue = make(scanTaskPriorityQueue, 0)
	}

	sc := &scanSchedulerImpl{
		taskQueue: taskQueue,
	}
	heap.Init(&sc.taskQueue)
	return sc
}

func (sc *scanSchedulerImpl) QueueScanTask(scanTask scanTask) {
	heap.Push(&sc.taskQueue, &scanTask)
}

func (sc *scanSchedulerImpl) PopDueScans(now time.Time) []snmpscanmanager.ScanRequest {
	var dueScans []snmpscanmanager.ScanRequest
	for sc.taskQueue.Len() > 0 {
		nextTask := sc.taskQueue[0]
		if nextTask.nextRun.After(now) {
			break
		}

		dueScanTask := heap.Pop(&sc.taskQueue).(*scanTask)
		dueScans = append(dueScans, dueScanTask.req)
	}
	return dueScans
}

func (pq scanTaskPriorityQueue) Len() int {
	return len(pq)
}

func (pq scanTaskPriorityQueue) Less(i1, i2 int) bool {
	return pq[i1].nextRun.Before(pq[i2].nextRun)
}

func (pq scanTaskPriorityQueue) Swap(i1, i2 int) {
	pq[i1], pq[i2] = pq[i2], pq[i1]
}

func (pq *scanTaskPriorityQueue) Push(x interface{}) {
	*pq = append(*pq, x.(*scanTask))
}

func (pq *scanTaskPriorityQueue) Pop() interface{} {
	old := *pq
	*pq = old[:len(old)-1]
	return old[len(old)-1]
}
