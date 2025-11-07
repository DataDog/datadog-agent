// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package snmpscanmanagerimpl

import (
	"testing"
	"time"

	snmpscanmanager "github.com/DataDog/datadog-agent/comp/snmpscanmanager/def"

	"github.com/stretchr/testify/assert"
)

func TestNewScanScheduler(t *testing.T) {
	now := time.Now()

	scanQueue := scanPriorityQueue{
		{req: snmpscanmanager.ScanRequest{DeviceIP: "10.0.0.1"}, nextRun: now},
		{req: snmpscanmanager.ScanRequest{DeviceIP: "10.0.0.2"}, nextRun: now},
	}

	sc := newScanScheduler(scanQueue)
	assert.NotNil(t, sc)
	assert.Equal(t, len(scanQueue), len(sc.(*scanSchedulerImpl).scanQueue))
}

func TestScanScheduler_QueueScan(t *testing.T) {
	now := time.Now()

	sc := newScanScheduler(scanPriorityQueue{})

	sc.QueueScan(scanTask{snmpscanmanager.ScanRequest{DeviceIP: "10.0.0.1"}, now})
	sc.QueueScan(scanTask{snmpscanmanager.ScanRequest{DeviceIP: "10.0.0.2"}, now})
	sc.QueueScan(scanTask{snmpscanmanager.ScanRequest{DeviceIP: "10.0.0.3"}, now})
	sc.QueueScan(scanTask{snmpscanmanager.ScanRequest{DeviceIP: "10.0.0.4"}, now})

	assert.Equal(t, 4, len(sc.(*scanSchedulerImpl).scanQueue))
}

func TestScanScheduler_PopDueScans(t *testing.T) {
	now := time.Now()

	sc := newScanScheduler(scanPriorityQueue{})

	sc.QueueScan(scanTask{snmpscanmanager.ScanRequest{DeviceIP: "10.0.0.1"},
		now.Add(-10 * time.Minute)})
	sc.QueueScan(scanTask{snmpscanmanager.ScanRequest{DeviceIP: "10.0.0.2"},
		now.Add(20 * time.Minute)})
	sc.QueueScan(scanTask{snmpscanmanager.ScanRequest{DeviceIP: "10.0.0.3"},
		now.Add(-1 * time.Minute)})
	sc.QueueScan(scanTask{snmpscanmanager.ScanRequest{DeviceIP: "10.0.0.4"},
		now.Add(10 * time.Minute)})

	dueScans := sc.PopDueScans(now)
	assert.Len(t, dueScans, 2)
	assert.Equal(t, "10.0.0.1", dueScans[0].DeviceIP)
	assert.Equal(t, "10.0.0.3", dueScans[1].DeviceIP)

	assert.Equal(t, 2, len(sc.(*scanSchedulerImpl).scanQueue))
}

func TestScanPriorityQueue_Len(t *testing.T) {
	pq := scanPriorityQueue{}
	assert.Equal(t, 0, pq.Len())

	now := time.Now()
	pq = append(pq, &scanTask{nextRun: now})
	assert.Equal(t, 1, pq.Len())

	pq = append(pq, &scanTask{nextRun: now})
	assert.Equal(t, 2, pq.Len())
}

func TestScanPriorityQueue_Less(t *testing.T) {
	now := time.Now()

	pq := scanPriorityQueue{
		&scanTask{
			req:     snmpscanmanager.ScanRequest{DeviceIP: "10.0.0.1"},
			nextRun: now.Add(2 * time.Minute),
		},
		&scanTask{
			req:     snmpscanmanager.ScanRequest{DeviceIP: "10.0.0.2"},
			nextRun: now.Add(10 * time.Minute),
		},
	}

	assert.True(t, pq.Less(0, 1))
	assert.False(t, pq.Less(1, 0))
}

func TestScanPriorityQueue_Swap(t *testing.T) {
	now := time.Now()

	scanTask1 := &scanTask{
		req:     snmpscanmanager.ScanRequest{DeviceIP: "10.0.0.1"},
		nextRun: now,
	}
	scanTask2 := &scanTask{
		req:     snmpscanmanager.ScanRequest{DeviceIP: "10.0.0.2"},
		nextRun: now,
	}

	pq := scanPriorityQueue{scanTask1, scanTask2}

	pq.Swap(0, 1)

	assert.Equal(t, "10.0.0.2", pq[0].req.DeviceIP)
	assert.Equal(t, "10.0.0.1", pq[1].req.DeviceIP)
}

func TestScanPriorityQueue_PushPop(t *testing.T) {
	now := time.Now()

	pq := scanPriorityQueue{}

	st := &scanTask{
		req:     snmpscanmanager.ScanRequest{DeviceIP: "10.0.0.1"},
		nextRun: now,
	}

	pq.Push(st)
	assert.Equal(t, 1, pq.Len())

	poppedSt := pq.Pop().(*scanTask)
	assert.Equal(t, 0, pq.Len())
	assert.Equal(t, "10.0.0.1", poppedSt.req.DeviceIP)
}
