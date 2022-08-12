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
	"github.com/DataDog/datadog-agent/pkg/network/etw"
)

type EtwMonitor struct {
	hei        *httpEtwInterface
	telemetry  *telemetry
	statkeeper *httpStatKeeper

	mux         sync.Mutex
	eventLoopWG sync.WaitGroup
}

// NewEtwMonitor returns a new EtwMonitor instance
func NewEtwMonitor(c *config.Config) (Monitor, error) {
	hei := newHttpEtwInterface(c)

	hei.setMaxFlows(uint64(c.MaxTrackedConnections))
	hei.setMaxRequestBytes(driver.HttpBufferSize)

	telemetry, err := newTelemetry()
	if err != nil {
		return nil, err
	}

	return &EtwMonitor{
		hei:        hei,
		telemetry:  telemetry,
		statkeeper: newHTTPStatkeeper(c, telemetry),
	}, nil
}

// Start consuming HTTP events
func (m *EtwMonitor) Start() {
	m.hei.startReadingHttpFlows()

	m.eventLoopWG.Add(1)
	go func() {
		defer m.eventLoopWG.Done()
		for {
			select {
			case transactions, ok := <-m.hei.dataChannel:
				if !ok {
					return
				}
				// dbtodo
				// the linux side has an error code potentially, that
				// gets aggregated under the hood.  Do we need somthing
				// analogous
				if len(transactions) > 0 {
					m.process(transactions, nil)
				}
			}
		}
	}()
}

func (m *EtwMonitor) process(transactionBatch []etw.Http, err error) {
	transactions := make([]httpTX, len(transactionBatch))
	for i := range transactionBatch {
		transactions[i] = &etwHttpTX{Http: &transactionBatch[i]}
	}

	m.mux.Lock()
	defer m.mux.Unlock()

	m.telemetry.aggregate(transactions, err)
	m.statkeeper.Process(transactions)
}

func (m *EtwMonitor) removeDuplicates(stats map[Key]*RequestStats) {
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

// GetHTTPStats returns a map of HTTP stats stored in the following format:
// [source, dest tuple, request path] -> RequestStats object
func (m *EtwMonitor) GetHTTPStats() map[Key]*RequestStats {

	transactions := m.hei.getHttpFlows()
	if transactions == nil {
		return nil
	}

	// dbtodo
	// also could there be a relevant error here
	m.process(transactions, nil)

	m.mux.Lock()
	defer m.mux.Unlock()

	stats := m.statkeeper.GetAndResetAllStats()
	m.removeDuplicates(stats)

	delta := m.telemetry.reset()
	delta.report()

	return stats
}

// GetStats gets driver stats related to the HTTP handle
func (m *EtwMonitor) GetStats() (map[string]int64, error) {
	return m.hei.getStats()
}

// Stop HTTP monitoring
func (m *EtwMonitor) Stop() error {
	m.hei.close()
	m.eventLoopWG.Wait()
	return nil
}
