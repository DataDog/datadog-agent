// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package http

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
)

type EtwMonitor struct {
	ei         *httpEtwInterface
	telemetry  *telemetry
	statkeeper *httpStatKeeper

	mux         sync.Mutex
	eventLoopWG sync.WaitGroup
}

// NewDriverMonitor returns a new DriverMonitor instance
func NewEtwMonitor(c *config.Config) (Monitor, error) {
	ei := newHttpEtwInterface()

	telemetry := newTelemetry()

	return &EtwMonitor{
		ei:         ei,
		telemetry:  telemetry,
		statkeeper: newHTTPStatkeeper(c, telemetry),
	}, nil
}

// Start consuming HTTP events
func (m *EtwMonitor) Start() {
	m.ei.startReadingHttpTransaction()

	m.eventLoopWG.Add(1)
	go func() {
		defer m.eventLoopWG.Done()
		for {
			select {
			case transactionBatch, ok := <-m.ei.dataChannel:
				if !ok {
					return
				}
				m.process(transactionBatch)
			}
		}
	}()

	return
}

func (m *EtwMonitor) process(transactionBatch []driver.HttpTransactionType) {
	// transactions, err := m.ei.flushPendingTransactions()
	// if err != nil {
	// 	log.Warnf("Failed to flush pending http transactions: %v", err)
	// }

	// transactions = make([]httpTX, len(transactionBatch))
	// for i := range transactionBatch {
	// 	transactions[i] = httpTX(transactionBatch[i])
	// }

	m.mux.Lock()
	defer m.mux.Unlock()

	// m.telemetry.aggregate(transactions, nil)

	// m.statkeeper.Process(transactions)
}

// func removeDuplicates(stats map[Key]RequestStats) {
// 	// With localhost traffic, the driver will create a flow for both endpoints. Both
// 	// these flows will be normalized so that source=client and dest=server, which
// 	// results in 2 identical http transactions being sent up to userspace & processed.
// 	// To fix this, we'll find all localhost keys and half their transaction counts.

// 	for k, v := range stats {
// 		if isLocalhost(k) {
// 			for i := 0; i < NumStatusClasses; i++ {
// 				v[i].Count = v[i].Count / 2
// 				stats[k] = v
// 			}
// 		}
// 	}
// }

// GetHTTPStats returns a map of HTTP stats stored in the following format:
// [source, dest tuple, request path] -> RequestStats object
func (m *EtwMonitor) GetHTTPStats() map[Key]RequestStats {
	// transactions, err := m.di.flushPendingTransactions()
	// if err != nil {
	// 	log.Warnf("Failed to flush pending http transactions: %v", err)
	// }

	// m.process(transactions)

	// m.mux.Lock()
	// defer m.mux.Unlock()

	// stats := m.statkeeper.GetAndResetAllStats()
	// removeDuplicates(stats)

	// delta := m.telemetry.reset()
	// delta.report()

	// return stats

	return nil
}

// GetStats gets driver stats related to the HTTP handle
func (m *EtwMonitor) GetStats() (map[string]int64, error) {
	return m.ei.getStats()
}

// Stop HTTP monitoring
func (m *EtwMonitor) Stop() error {
	m.ei.close()
	m.eventLoopWG.Wait()
	return nil
}
