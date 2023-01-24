// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kafka

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	manager "github.com/DataDog/ebpf-manager"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	filterpkg "github.com/DataDog/datadog-agent/pkg/network/filter"
)

// MonitorStats is used for holding two kinds of stats:
// * requestsStats which are the kafka data stats
// * telemetry which are telemetry stats
type MonitorStats struct {
	requestStats map[Key]*RequestStats
	telemetry    telemetry
}

// Monitor is responsible for:
// * Creating a raw socket and attaching an eBPF filter to it;
// * Polling a perf buffer that contains notifications about Kafka transaction batches ready to be read;
// * Querying these batches by doing a map lookup;
// * Aggregating and emitting metrics based on the received Kafka transactions;
type Monitor struct {
	handler func([]*ebpfKafkaTx)

	consumer       *events.Consumer
	ebpfProgram    *ebpfProgram
	telemetry      *telemetry
	statkeeper     *kafkaStatKeeper
	processMonitor *monitor.ProcessMonitor

	// termination
	closeFilterFn func()
}

// NewMonitor returns a new Monitor instance
func NewMonitor(c *config.Config, bpfTelemetry *errtelemetry.EBPFTelemetry) (*Monitor, error) {
	mgr, err := newEBPFProgram(c, bpfTelemetry)
	if err != nil {
		return nil, fmt.Errorf("error setting up kafka ebpf program: %s", err)
	}

	if err := mgr.Init(); err != nil {
		return nil, fmt.Errorf("error initializing kafka ebpf program: %s", err)
	}

	filter, _ := mgr.GetProbe(manager.ProbeIdentificationPair{EBPFSection: kafkaSocketFilterStub, EBPFFuncName: "socket__kafka_filter_entry", UID: probeUID})
	if filter == nil {
		return nil, fmt.Errorf("error retrieving socket filter")
	}

	closeFilterFn, err := filterpkg.HeadlessSocketFilter(c, filter)
	if err != nil {
		return nil, fmt.Errorf("error enabling Kafka traffic inspection: %s", err)
	}

	telemetry := newTelemetry()
	if err != nil {
		closeFilterFn()
		return nil, err
	}

	statkeeper := newKafkaStatkeeper(c, telemetry)
	processMonitor := monitor.GetProcessMonitor()

	return &Monitor{
		ebpfProgram:    mgr,
		telemetry:      telemetry,
		closeFilterFn:  closeFilterFn,
		statkeeper:     statkeeper,
		processMonitor: processMonitor,
	}, nil
}

// Start consuming Kafka events
func (m *Monitor) Start() error {
	if m == nil {
		return nil
	}

	var err error
	m.consumer, err = events.NewConsumer(
		"http",
		m.ebpfProgram.Manager.Manager,
		m.process,
	)
	if err != nil {
		return err
	}
	m.consumer.Start()

	if err := m.ebpfProgram.Start(); err != nil {
		return err
	}

	return m.processMonitor.Initialize()

	//m.eventLoopWG.Add(1)
	//go func() {
	//	defer m.eventLoopWG.Done()
	//	for {
	//		select {
	//		case dataEvent, ok := <-m.batchCompletionHandler.DataChannel:
	//			if !ok {
	//				return
	//			}
	//
	//			transactions, err := m.batchManager.GetTransactionsFrom(dataEvent)
	//			m.process(transactions, err)
	//			dataEvent.Done()
	//		case _, ok := <-m.batchCompletionHandler.LostChannel:
	//			if !ok {
	//				return
	//			}
	//
	//			m.process(nil, errLostBatch)
	//		case reply, ok := <-m.pollRequests:
	//			if !ok {
	//				return
	//			}
	//
	//			transactions := m.batchManager.GetPendingTransactions()
	//			m.process(transactions, nil)
	//
	//			delta := m.telemetry.reset()
	//
	//			// For now, we still want to report the telemetry as it contains more information than what
	//			// we're extracting via network tracer.
	//			delta.report()
	//
	//			reply <- MonitorStats{
	//				requestStats: m.statkeeper.GetAndResetAllStats(),
	//				telemetry:    delta,
	//			}
	//		}
	//	}
	//}()
	//return nil
}

// GetKafkaStats returns a map of Kafka stats stored in the following format:
// [source, dest tuple, TopicName] -> RequestStats object
func (m *Monitor) GetKafkaStats() map[Key]*RequestStats {
	if m == nil {
		return nil
	}

	m.consumer.Sync()
	m.telemetry.log()
	return m.statkeeper.GetAndResetAllStats()

	m.mux.Lock()
	defer m.mux.Unlock()
	if m.stopped {
		return nil
	}

	reply := make(chan MonitorStats, 1)
	defer close(reply)
	m.pollRequests <- reply
	stats := <-reply
	m.telemetrySnapshot = &stats.telemetry
	return stats.requestStats
}

// GetStats returns the telemetry
func (m *Monitor) GetStats() map[string]interface{} {
	empty := map[string]interface{}{}
	if m == nil {
		return empty
	}

	m.mux.Lock()
	defer m.mux.Unlock()
	if m.stopped {
		return empty
	}

	if m.telemetrySnapshot == nil {
		return empty
	}

	return m.telemetrySnapshot.report()
}

// Stop Kafka monitoring
func (m *Monitor) Stop() {
	if m == nil {
		return
	}

	m.mux.Lock()
	defer m.mux.Unlock()
	if m.stopped {
		return
	}

	m.ebpfProgram.Close()
	m.closeFilterFn()
	close(m.pollRequests)
	m.eventLoopWG.Wait()
	m.stopped = true
}

func (m *Monitor) process(data []byte) {
	//m.telemetry.aggregate(tx, err)

	//if m.handler != nil && len(transactions) > 0 {
	//	m.handler(transactions)
	//}

	tx := (*ebpfKafkaTx)(unsafe.Pointer(&data[0]))
	m.statkeeper.Process(tx)
}

// DumpMaps dumps the maps associated with the monitor
func (m *Monitor) DumpMaps(maps ...string) (string, error) {
	return m.ebpfProgram.DumpMaps(maps...)
}
