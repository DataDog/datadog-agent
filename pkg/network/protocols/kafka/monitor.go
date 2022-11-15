// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kafka

import (
	"fmt"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"sync"

	"github.com/cilium/ebpf"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	filterpkg "github.com/DataDog/datadog-agent/pkg/network/filter"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
)

// MonitorStats is used for holding two kinds of stats:
// * requestsStats which are the kafka data stats
// * telemetry which are telemetry stats
type MonitorStats struct {
	//requestStats map[Key]*RequestStats
	//telemetry    telemetry
}

// Monitor is responsible for:
// * Creating a raw socket and attaching an eBPF filter to it;
// * Polling a perf buffer that contains notifications about HTTP transaction batches ready to be read;
// * Querying these batches by doing a map lookup;
// * Aggregating and emitting metrics based on the received HTTP transactions;
type Monitor struct {
	handler func([]kafkaTX)

	ebpfProgram            *ebpfProgram
	batchManager           *batchManager
	batchCompletionHandler *ddebpf.PerfHandler
	telemetry              *telemetry
	// telemetrySnapshot      *telemetry
	pollRequests chan chan map[Key]*RequestStats
	statkeeper   *kafkaStatKeeper

	// termination
	mux           sync.Mutex
	eventLoopWG   sync.WaitGroup
	closeFilterFn func()
	stopped       bool
}

// NewMonitor returns a new Monitor instance
func NewMonitor(c *config.Config, offsets []manager.ConstantEditor, sockFD *ebpf.Map, bpfTelemetry *errtelemetry.EBPFTelemetry) (*Monitor, error) {
	mgr, err := newEBPFProgram(c, offsets, sockFD, bpfTelemetry)
	if err != nil {
		return nil, fmt.Errorf("error setting up http ebpf program: %s", err)
	}

	if err := mgr.Init(); err != nil {
		return nil, fmt.Errorf("error initializing http ebpf program: %s", err)
	}

	filter, _ := mgr.GetProbe(manager.ProbeIdentificationPair{EBPFSection: kafkaSocketFilterStub, EBPFFuncName: "socket__kafka_filter_entry", UID: probeUID})
	if filter == nil {
		return nil, fmt.Errorf("error retrieving socket filter")
	}

	closeFilterFn, err := filterpkg.HeadlessSocketFilter(c, filter)
	if err != nil {
		return nil, fmt.Errorf("error enabling HTTP traffic inspection: %s", err)
	}

	batchMap, _, err := mgr.GetMap(kafkaBatchesMap)
	if err != nil {
		return nil, err
	}

	batchEventsMap, _, _ := mgr.GetMap(kafkaBatchEvents)
	numCPUs := int(batchEventsMap.MaxEntries())

	telemetry, err := newTelemetry()
	if err != nil {
		return nil, err
	}
	statkeeper := newKAFKAStatkeeper(c, telemetry)

	handler := func(transactions []kafkaTX) {
		log.Debug("in handler")
		if statkeeper != nil {
			statkeeper.Process(transactions)
		}
	}

	batchManager, err := newBatchManager(batchMap, numCPUs)
	if err != nil {
		return nil, fmt.Errorf("couldn't instantiate batch manager: %w", err)
	}

	return &Monitor{
		handler:                handler,
		ebpfProgram:            mgr,
		batchManager:           batchManager,
		batchCompletionHandler: mgr.batchCompletionHandler,
		telemetry:              telemetry,
		pollRequests:           make(chan chan map[Key]*RequestStats),
		closeFilterFn:          closeFilterFn,
		statkeeper:             statkeeper,
	}, nil
}

// Start consuming KAFKA events
func (m *Monitor) Start() error {
	if m == nil {
		return nil
	}

	if err := m.ebpfProgram.Start(); err != nil {
		return err
	}

	m.eventLoopWG.Add(1)
	go func() {
		defer m.eventLoopWG.Done()
		for {
			select {
			case dataEvent, ok := <-m.batchCompletionHandler.DataChannel:
				if !ok {
					return
				}

				transactions, err := m.batchManager.GetTransactionsFrom(dataEvent)
				m.process(transactions, err)
				dataEvent.Done()
			case _, ok := <-m.batchCompletionHandler.LostChannel:
				if !ok {
					return
				}

				m.process(nil, errLostBatch)
			case reply, ok := <-m.pollRequests:
				if !ok {
					return
				}

				transactions := m.batchManager.GetPendingTransactions()
				m.process(transactions, nil)

				m.telemetry.log()
				reply <- m.statkeeper.GetAndResetAllStats()
				//reply <- requestStatsMap
			}
		}
	}()

	return nil
}

// GetKafkaStats returns a map of Kafka stats stored in the following format:
// [source, dest tuple, request path] -> RequestStats object
func (m *Monitor) GetKafkaStats() map[Key]*RequestStats {
	if m == nil {
		return nil
	}

	m.mux.Lock()
	defer m.mux.Unlock()
	if m.stopped {
		return nil
	}

	reply := make(chan map[Key]*RequestStats, 1)
	defer close(reply)
	m.pollRequests <- reply
	return <-reply
}

//
//// GetStats returns the telemetry
//func (m *Monitor) GetStats() map[string]interface{} {
//	empty := map[string]interface{}{}
//	if m == nil {
//		return empty
//	}
//
//	m.mux.Lock()
//	defer m.mux.Unlock()
//	if m.stopped {
//		return empty
//	}
//
//	if m.telemetrySnapshot == nil {
//		return empty
//	}
//
//	return m.telemetrySnapshot.report()
//}

// Stop HTTP monitoring
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

func (m *Monitor) process(transactions []kafkaTX, err error) {
	m.telemetry.aggregate(transactions, err)

	if m.handler != nil && len(transactions) > 0 {
		m.handler(transactions)
	}

	//var requestStatsMap map[Key]*RequestStats
	//for _, transaction := range transactions {
	//	key := Key{
	//		KeyTuple: KeyTuple{
	//			SrcIPHigh: transaction.SrcIPHigh(),
	//			SrcIPLow:  transaction.SrcIPLow(),
	//			SrcPort:   transaction.SrcPort(),
	//			DstIPHigh: transaction.DstIPHigh(),
	//			DstIPLow:  transaction.DstIPLow(),
	//			DstPort:   transaction.DstPort(),
	//		},
	//		TopicName: transaction.TopicName(),
	//	}
	//	requestStats, ok := requestStatsMap[key]
	//	if !ok {
	//		// TODO: Should limit the size of requestStatsMap?
	//		requestStats = new(RequestStats)
	//		requestStatsMap[key] = requestStats
	//	}
	//	requestStats.data[ProduceAPIKey].Count++
	//}

	//if m.handler != nil && len(transactions) > 0 {
	//	m.handler(transactions)
	//}
}

// DumpMaps dumps the maps associated with the monitor
func (m *Monitor) DumpMaps(maps ...string) (string, error) {
	return m.ebpfProgram.DumpMaps(maps...)
}
