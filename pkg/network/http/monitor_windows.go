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
	GetHTTPStats() map[Key]*RequestStats
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
func NewDriverMonitor(c *config.Config, dh driver.Handle) (Monitor, error) {
	di, err := newDriverInterface(c, dh)
	if err != nil {
		return nil, err
	}

	telemetry, err := newTelemetry()
	if err != nil {
		return nil, err
	}

	return &DriverMonitor{
		di:         di,
		telemetry:  telemetry,
		statkeeper: newHTTPStatkeeper(c, telemetry),
	}, nil
}

// Start consuming HTTP events
func (m *DriverMonitor) Start() {
	log.Infof("Driver Monitor: starting")
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
				// dbtodo
				// the linux side has an error code potentially, that
				// gets aggregated under the hood.  Do we need somthing
				// analogous
				m.process(transactionBatch, nil)
			}
		}
	}()

	return
}

func (m *DriverMonitor) process(transactionBatch []FullHttpTransaction, err error) {
	transactions := make([]httpTX, len(transactionBatch))
	for i := range transactionBatch {
		transactions[i] = &transactionBatch[i]

	}

	m.mux.Lock()
	defer m.mux.Unlock()

	m.telemetry.aggregate(transactions, err)
	m.statkeeper.Process(transactions)
}

// GetHTTPStats returns a map of HTTP stats stored in the following format:
// [source, dest tuple, request path] -> RequestStats object
func (m *DriverMonitor) GetHTTPStats() map[Key]*RequestStats {
	// dbtodo  This is now going to cause any pending transactions
	// to be read and then stuffed into the channel.  Which then I think
	// creates a race condition that there still could be some mid-
	// process when we come back
	m.di.readAllPendingTransactions()

	m.mux.Lock()
	defer m.mux.Unlock()

	stats := m.statkeeper.GetAndResetAllStats()
	removeDuplicates(stats)

	delta := m.telemetry.reset()
	delta.report()

	return stats
}

func removeDuplicates(stats map[Key]*RequestStats) {
	// With localhost traffic, the driver will create a flow for both endpoints. Both
	// these flows will be normalized so that source=client and dest=server, which
	// results in 2 identical http transactions being sent up to userspace & processed.
	// To fix this, we'll find all localhost keys and half their transaction counts.

	for k, v := range stats {
		if isLocalhost(k) {
			v.HalfAllCounts()
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
