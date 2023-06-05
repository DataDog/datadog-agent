// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package reorderer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf/perf"

	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

const EventStreamMap = "events"

// OrderedPerfMap implements the EventStream interface
// using an eBPF perf map associated with a event reorder.
type OrderedPerfMap struct {
	perfMap          *manager.PerfMap
	lostEventCounter LostEventCounter
	reordererMonitor *ReordererMonitor
	reOrderer        *ReOrderer
	recordPool       *RecordPool
}

type LostEventCounter interface {
	CountLostEvent(count uint64, perfMapName string, CPU int)
}

// Init the event stream.
func (m *OrderedPerfMap) Init(mgr *manager.Manager, config *config.Config) error {
	var ok bool
	if m.perfMap, ok = mgr.GetPerfMap(EventStreamMap); !ok {
		return errors.New("couldn't find events perf map")
	}

	m.perfMap.PerfMapOptions = manager.PerfMapOptions{
		RecordHandler: m.reOrderer.HandleEvent,
		LostHandler:   m.handleLostEvents,
		RecordGetter:  m.recordPool.Get,
	}

	if config.EventStreamBufferSize != 0 {
		m.perfMap.PerfMapOptions.PerfRingBufferSize = config.EventStreamBufferSize
	}

	return nil
}

func (m *OrderedPerfMap) handleLostEvents(CPU int, count uint64, perfMap *manager.PerfMap, manager *manager.Manager) {
	seclog.Tracef("lost %d events", count)
	if m.lostEventCounter != nil {
		m.lostEventCounter.CountLostEvent(count, perfMap.Name, CPU)
	}
}

// SetMonitor set the monitor
func (m *OrderedPerfMap) SetMonitor(counter LostEventCounter) {
	m.lostEventCounter = counter
}

// Start the event stream.
func (m *OrderedPerfMap) Start(wg *sync.WaitGroup) error {
	wg.Add(2)
	go m.reordererMonitor.Start(wg)
	go m.reOrderer.Start(wg)
	return nil
}

// Pause the event stream.
func (m *OrderedPerfMap) Pause() error {
	return m.perfMap.Pause()
}

// Resume the event stream.
func (m *OrderedPerfMap) Resume() error {
	return m.perfMap.Resume()
}

// ExtractEventInfo extracts cpu and timestamp from the raw data event
func ExtractEventInfo(record *perf.Record) (QuickInfo, error) {
	if len(record.RawSample) < 16 {
		return QuickInfo{}, model.ErrNotEnoughData
	}

	return QuickInfo{
		Cpu:       model.ByteOrder.Uint64(record.RawSample[0:8]),
		Timestamp: model.ByteOrder.Uint64(record.RawSample[8:16]),
	}, nil
}

// NewOrderedPerfMap returned a new ordered perf map.
func NewOrderedPerfMap(ctx context.Context, handler func(int, []byte), statsdClient statsd.ClientInterface) (*OrderedPerfMap, error) {
	recordPool := NewRecordPool()
	reOrderer := NewReOrderer(ctx,
		func(record *perf.Record) {
			defer recordPool.Release(record)
			handler(record.CPU, record.RawSample)
		},
		ExtractEventInfo,
		ReOrdererOpts{
			QueueSize:       10000,
			Rate:            50 * time.Millisecond,
			Retention:       5,
			MetricRate:      5 * time.Second,
			HeapShrinkDelta: 1000,
		})

	monitor, err := NewReOrderMonitor(ctx, statsdClient, reOrderer)
	if err != nil {
		return nil, fmt.Errorf("couldn't create the reorder monitor: %w", err)
	}

	return &OrderedPerfMap{
		reOrderer:        reOrderer,
		recordPool:       recordPool,
		reordererMonitor: monitor,
	}, nil
}
