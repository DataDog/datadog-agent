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
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultMaxTrackedConnections = 65536
)

// Monitor is the interface to HTTP monitoring
type Monitor interface {
	Start()
	GetHTTPStats() map[Key]RequestStats
	GetStats() (map[string]int64, error)
	Stop() error
}

// DriverMonitor is responsible for aggregating and emitting metrics based on
// batches of HTTP transactions received from the driver interface
type DriverMonitor struct {
	di         *httpDriverInterface
	telemetry  *telemetry
	statkeeper *httpStatKeeper

	mux         sync.Mutex
	eventLoopWG sync.WaitGroup
}

// NewDriverMonitor returns a new DriverMonitor instance
func NewDriverMonitor(c *config.Config) (Monitor, error) {
	di, err := newDriverInterface()
	if err != nil {
		return nil, err
	}

	if uint64(c.MaxTrackedConnections) != defaultMaxTrackedConnections {
		maxFlows := uint64(c.MaxTrackedConnections)
		err := di.setMaxFlows(maxFlows)
		if err != nil {
			log.Warnf("Failed to set max number of flows in driver http filter to %v %v", maxFlows, err)
		}
	}

	telemetry := newTelemetry()

	return &DriverMonitor{
		di:         di,
		telemetry:  telemetry,
		statkeeper: newHTTPStatkeeper(c, telemetry),
	}, nil
}

// Start consuming HTTP events
func (m *DriverMonitor) Start() {
	m.di.startReadingBuffers()

	m.eventLoopWG.Add(1)
	go func() {
		defer m.eventLoopWG.Done()
		for {
			select {
			case transactionBatch, ok := <-m.di.dataChannel:
				if !ok {
					return
				}
				m.process(transactionBatch)
			}
		}
	}()

	return
}

func (m *DriverMonitor) process(transactionBatch []driver.HttpTransactionType) {
	transactions := make([]httpTX, len(transactionBatch))
	for i := range transactionBatch {
		transactions[i] = httpTX(transactionBatch[i])
	}

	m.mux.Lock()
	defer m.mux.Unlock()

	m.telemetry.aggregate(transactions, nil)

	m.statkeeper.Process(transactions)
}

// GetHTTPStats returns a map of HTTP stats stored in the following format:
// [source, dest tuple, request path] -> RequestStats object
func (m *DriverMonitor) GetHTTPStats() map[Key]RequestStats {
	transactions, err := m.di.flushPendingTransactions()
	if err != nil {
		log.Warnf("Failed to flush pending http transactions: %v", err)
	}

	m.process(transactions)

	m.mux.Lock()
	defer m.mux.Unlock()

	stats := m.statkeeper.GetAndResetAllStats()
	removeDuplicates(stats)

	delta := m.telemetry.reset()
	delta.report()

	return stats
}

func removeDuplicates(stats map[Key]RequestStats) {
	// With localhost traffic, the driver will create a flow for both endpoints. Both
	// these flows will be normalized so that source=client and dest=server, which
	// results in 2 identical http transactions being sent up to userspace & processed.
	// To fix this, we'll find all localhost keys and half their transaction counts.

	for k, v := range stats {
		if isLocalhost(k) {
			for i := 0; i < NumStatusClasses; i++ {
				v[i].Count = v[i].Count / 2
				stats[k] = v
			}
		}
	}
}

func isLocalhost(k Key) bool {
	var sAddr util.Address
	if k.SrcIPHigh == 0 {
		sAddr = util.V4Address(uint32(k.SrcIPLow))
	} else {
		sAddr = util.V6Address(k.SrcIPLow, k.SrcIPHigh)
	}

	return sAddr.IsLoopback()
}

// GetStats gets driver stats related to the HTTP handle
func (m *DriverMonitor) GetStats() (map[string]int64, error) {
	return m.di.getStats()
}

// Stop HTTP monitoring
func (m *DriverMonitor) Stop() error {
	err := m.di.close()
	m.eventLoopWG.Wait()
	return err
}
