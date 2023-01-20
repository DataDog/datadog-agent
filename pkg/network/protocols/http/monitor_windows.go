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

// Monitor is the interface to HTTP monitoring
type Monitor interface {
	Start()
	GetHTTPStats() map[Key]*RequestStats
	GetStats() (map[string]int64, error)
	Stop() error
}

// WindowsMonitor is responsible for aggregating and emitting metrics based on
// batches of HTTP transactions received from the driver interface
type WindowsMonitor struct {
	di         *httpDriverInterface
	hei        *httpEtwInterface
	telemetry  *telemetry
	statkeeper *httpStatKeeper

	mux         sync.Mutex
	eventLoopWG sync.WaitGroup
}

// NewWindowsMonitor returns a new WindowsMonitor instance
func NewWindowsMonitor(c *config.Config, dh driver.Handle) (Monitor, error) {
	di, err := newDriverInterface(c, dh)
	if err != nil {
		return nil, err
	}
	hei := newHttpEtwInterface(c)

	hei.setMaxFlows(uint64(c.MaxTrackedConnections))
	hei.setMaxRequestBytes(uint64(c.HTTPMaxRequestFragment))
	hei.setCapturedProtocols(c.EnableHTTPMonitoring, c.EnableHTTPSMonitoring)

	telemetry, err := newTelemetry()
	if err != nil {
		return nil, err
	}

	return &WindowsMonitor{
		di:         di,
		hei:        hei,
		telemetry:  telemetry,
		statkeeper: newHTTPStatkeeper(c, telemetry),
	}, nil
}

// Start consuming HTTP events
func (m *WindowsMonitor) Start() {
	log.Infof("Driver Monitor: starting")
	m.di.startReadingBuffers()
	m.hei.startReadingHttpFlows()

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
				m.process(transactionBatch)
			case transactions, ok := <-m.hei.dataChannel:
				if !ok {
					return
				}
				// dbtodo
				// the linux side has an error code potentially, that
				// gets aggregated under the hood.  Do we need somthing
				// analogous
				if len(transactions) > 0 {
					m.process(transactions)
				}
			}
		}
	}()

	return
}

func (m *WindowsMonitor) process(transactionBatch []WinHttpTransaction) {
	m.mux.Lock()
	defer m.mux.Unlock()

	for i := range transactionBatch {
		tx := httpTX(&transactionBatch[i])
		m.telemetry.count(tx)
		m.statkeeper.Process(tx)
	}
}

// GetHTTPStats returns a map of HTTP stats stored in the following format:
// [source, dest tuple, request path] -> RequestStats object
func (m *WindowsMonitor) GetHTTPStats() map[Key]*RequestStats {
	// dbtodo  This is now going to cause any pending transactions
	// to be read and then stuffed into the channel.  Which then I think
	// creates a race condition that there still could be some mid-
	// process when we come back
	m.di.readAllPendingTransactions()

	m.mux.Lock()
	defer m.mux.Unlock()

	stats := m.statkeeper.GetAndResetAllStats()
	removeDuplicates(stats)

	m.telemetry.log()

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
	// this little hack is because ipv6 loopback (::1) has the property of having
	// the top 64 bits be zero (just like an ipv4).  Could just skip the call to
	// IsLoopback() below, but leaving it to allow the underlying library to do
	// the check as originally desired.
	if k.SrcIPHigh == 0 && k.SrcIPLow != uint64(0x0100000000000000) {
		sAddr = util.V4Address(uint32(k.SrcIPLow))
	} else {
		sAddr = util.V6Address(k.SrcIPLow, k.SrcIPHigh)
	}

	return sAddr.IsLoopback()
}

// GetStats gets driver stats related to the HTTP handle
func (m *WindowsMonitor) GetStats() (map[string]int64, error) {
	return m.di.getStats()
}

// Stop HTTP monitoring
func (m *WindowsMonitor) Stop() error {
	err := m.di.close()
	m.hei.close()
	m.eventLoopWG.Wait()
	return err
}
