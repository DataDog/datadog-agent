// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package usm

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Monitor is the interface to HTTP monitoring
type Monitor interface {
	Start()
	GetHTTPStats() map[protocols.ProtocolType]interface{}
	Stop() error
}

// WindowsMonitor is responsible for aggregating and emitting metrics based on
// batches of HTTP transactions received from the driver interface
type WindowsMonitor struct {
	di         *http.HttpDriverInterface
	hei        *http.EtwInterface
	telemetry  *http.Telemetry
	statkeeper *http.StatKeeper

	mux         sync.Mutex
	eventLoopWG sync.WaitGroup
}

// NewWindowsMonitor returns a new WindowsMonitor instance
func NewWindowsMonitor(c *config.Config, dh driver.Handle) (Monitor, error) {
	di, err := http.NewDriverInterface(c, dh)
	if err != nil {
		return nil, err
	}
	hei := http.NewEtwInterface(c)

	hei.SetMaxFlows(uint64(c.MaxTrackedConnections))
	hei.SetMaxRequestBytes(uint64(c.HTTPMaxRequestFragment))
	hei.SetCapturedProtocols(c.EnableHTTPMonitoring, c.EnableNativeTLSMonitoring)

	telemetry := http.NewTelemetry("http")

	return &WindowsMonitor{
		di:         di,
		hei:        hei,
		telemetry:  telemetry,
		statkeeper: http.NewStatkeeper(c, telemetry),
	}, nil
}

// Start consuming HTTP events
func (m *WindowsMonitor) Start() {
	log.Infof("Driver Monitor: starting")
	m.di.StartReadingBuffers()
	m.hei.StartReadingHttpFlows()

	m.eventLoopWG.Add(1)
	go func() {
		defer m.eventLoopWG.Done()
		for {
			select {
			case transactionBatch, ok := <-m.di.DataChannel:
				if !ok {
					return
				}
				// dbtodo
				// the linux side has an error code potentially, that
				// gets aggregated under the hood.  Do we need somthing
				// analogous
				m.process(transactionBatch)
			case transactions, ok := <-m.hei.DataChannel:
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

func (m *WindowsMonitor) process(transactionBatch []http.WinHttpTransaction) {
	m.mux.Lock()
	defer m.mux.Unlock()

	for i := range transactionBatch {
		tx := http.Transaction(&transactionBatch[i])
		m.telemetry.Count(tx)
		m.statkeeper.Process(tx)
	}
}

// GetHTTPStats returns a map of HTTP stats stored in the following format:
// [source, dest tuple, request path] -> RequestStats object
func (m *WindowsMonitor) GetHTTPStats() map[protocols.ProtocolType]interface{} {
	// dbtodo  This is now going to cause any pending transactions
	// to be read and then stuffed into the channel.  Which then I think
	// creates a race condition that there still could be some mid-
	// process when we come back
	m.di.ReadAllPendingTransactions()

	m.mux.Lock()
	defer m.mux.Unlock()

	stats := m.statkeeper.GetAndResetAllStats()
	//removeDuplicates(stats)

	m.telemetry.Log()

	ret := make(map[protocols.ProtocolType]interface{})
	ret[protocols.HTTP] = stats

	return ret
}

// Stop HTTP monitoring
func (m *WindowsMonitor) Stop() error {
	err := m.di.Close()
	m.hei.Close()
	m.eventLoopWG.Wait()
	return err
}
