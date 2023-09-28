// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package tracer

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	driver "github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/network/usm"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultPollInterval = int(15)
	defaultBufferSize   = 512
	minBufferSize       = 256
)

// Tracer struct for tracking network state and connections
type Tracer struct {
	config          *config.Config
	driverInterface *network.DriverInterface
	stopChan        chan struct{}
	state           network.State
	reverseDNS      dns.ReverseDNS
	usmMonitor      usm.Monitor

	closedBuffer *network.ConnectionBuffer
	connLock     sync.Mutex

	timerInterval int

	// Connections for the tracer to exclude
	sourceExcludes []*network.ConnectionFilter
	destExcludes   []*network.ConnectionFilter

	// polling loop for connection event
	closedEventLoop sync.WaitGroup
}

// NewTracer returns an initialized tracer struct
func NewTracer(config *config.Config) (*Tracer, error) {
	if err := driver.Start(); err != nil {
		return nil, fmt.Errorf("error starting driver: %s", err)
	}
	di, err := network.NewDriverInterface(config, driver.NewHandle)

	if err != nil && errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		log.Debugf("could not create driver interface: %v", err)
		return nil, fmt.Errorf("The Windows driver was not installed, reinstall the Datadog Agent with network performance monitoring enabled")
	} else if err != nil {
		return nil, fmt.Errorf("could not create windows driver controller: %v", err)
	}

	state := network.NewState(
		config.ClientStateExpiry,
		config.MaxClosedConnectionsBuffered,
		config.MaxConnectionsStateBuffered,
		config.MaxDNSStatsBuffered,
		config.MaxHTTPStatsBuffered,
		config.MaxKafkaStatsBuffered,
	)

	reverseDNS := dns.NewNullReverseDNS()
	if config.DNSInspection {
		reverseDNS, err = dns.NewReverseDNS(config)
		if err != nil {
			return nil, err
		}
	}

	tr := &Tracer{
		config:          config,
		driverInterface: di,
		stopChan:        make(chan struct{}),
		timerInterval:   defaultPollInterval,
		state:           state,
		closedBuffer:    network.NewConnectionBuffer(defaultBufferSize, minBufferSize),
		reverseDNS:      reverseDNS,
		usmMonitor:      newUSMMonitor(config, di.GetHandle()),
		sourceExcludes:  network.ParseConnectionFilters(config.ExcludedSourceConnections),
		destExcludes:    network.ParseConnectionFilters(config.ExcludedDestinationConnections),
	}
	tr.closedEventLoop.Add(1)
	go func() {
		defer tr.closedEventLoop.Done()

		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	waitloop:
		for {
			evt, _ := windows.WaitForSingleObject(di.GetClosedFlowsEvent(), windows.INFINITE)
			switch evt {
			case windows.WAIT_OBJECT_0:
				_, err = tr.driverInterface.GetClosedConnectionStats(tr.closedBuffer, func(c *network.ConnectionStats) bool {
					return !tr.shouldSkipConnection(c)
				})
				closedConnStats := tr.closedBuffer.Connections()

				tr.state.StoreClosedConnections(closedConnStats)

			case windows.WAIT_FAILED:
				break waitloop

			default:
				log.Infof("got other wait value %v", evt)
			}
		}

	}()
	return tr, nil
}

// Stop function stops running tracer
func (t *Tracer) Stop() {
	close(t.stopChan)
	if t.usmMonitor != nil { //nolint
		_ = t.usmMonitor.Stop()
	}
	t.reverseDNS.Close()
	err := t.driverInterface.Close()
	if err != nil {
		log.Errorf("error closing driver interface: %s", err)
	}
	t.closedEventLoop.Wait()
	if err := driver.Stop(); err != nil {
		log.Errorf("error stopping driver: %s", err)
	}
}

// GetActiveConnections returns all active connections
func (t *Tracer) GetActiveConnections(clientID string) (*network.Connections, error) {
	t.connLock.Lock()
	defer t.connLock.Unlock()

	defer func() {
		t.closedBuffer.Reset()
	}()

	buffer := network.ClientPool.Get(clientID)
	_, err := t.driverInterface.GetOpenConnectionStats(buffer.ConnectionBuffer, func(c *network.ConnectionStats) bool {
		return !t.shouldSkipConnection(c)
	})
	if err != nil {
		return nil, fmt.Errorf("error retrieving open connections from driver: %w", err)
	}
	_, err = t.driverInterface.GetClosedConnectionStats(t.closedBuffer, func(c *network.ConnectionStats) bool {
		return !t.shouldSkipConnection(c)
	})
	if err != nil {
		return nil, fmt.Errorf("error retrieving closed connections from driver: %w", err)
	}
	activeConnStats := buffer.Connections()
	closedConnStats := t.closedBuffer.Connections()

	// check for expired clients in the state
	t.state.RemoveExpiredClients(time.Now())

	t.state.StoreClosedConnections(closedConnStats)

	var delta network.Delta
	if t.usmMonitor != nil { //nolint
		delta = t.state.GetDelta(clientID, uint64(time.Now().Nanosecond()), activeConnStats, t.reverseDNS.GetDNSStats(), t.usmMonitor.GetHTTPStats())
	} else {
		delta = t.state.GetDelta(clientID, uint64(time.Now().Nanosecond()), activeConnStats, t.reverseDNS.GetDNSStats(), nil)
	}

	ips := make(map[util.Address]struct{}, len(delta.Conns)/2)
	for _, conn := range delta.Conns {
		ips[conn.Source] = struct{}{}
		ips[conn.Dest] = struct{}{}
	}

	buffer.Assign(delta.Conns)
	conns := network.NewConnections(buffer)
	conns.DNS = t.reverseDNS.Resolve(ips)
	conns.ConnTelemetry = t.state.GetTelemetryDelta(clientID, t.getConnTelemetry())
	conns.HTTP = delta.HTTP
	conns.DNSStats = delta.DNSStats
	return conns, nil
}

// RegisterClient registers the client
func (t *Tracer) RegisterClient(clientID string) error {
	t.state.RegisterClient(clientID)
	return nil
}

func (t *Tracer) getConnTelemetry() map[network.ConnTelemetryType]int64 {
	return map[network.ConnTelemetryType]int64{
		network.MonotonicDNSPacketsDropped: driver.HandleTelemetry.ReadPacketsSkipped.Load(),
	}
}

func (t *Tracer) getStats() (map[string]interface{}, error) {
	stats := map[string]interface{}{
		"state": t.state.GetStats(),
	}
	return stats, nil
}

// GetStats returns a map of statistics about the current tracer's internal state
func (t *Tracer) GetStats() (map[string]interface{}, error) {
	return t.getStats()
}

// DebugNetworkState returns a map with the current tracer's internal state, for debugging
func (t *Tracer) DebugNetworkState(_ string) (map[string]interface{}, error) {
	return nil, ebpf.ErrNotImplemented
}

// DebugNetworkMaps returns all connections stored in the maps without modifications from network state
func (t *Tracer) DebugNetworkMaps() (*network.Connections, error) {
	return nil, ebpf.ErrNotImplemented
}

// DebugEBPFMaps is not implemented on this OS for Tracer
func (t *Tracer) DebugEBPFMaps(maps ...string) (string, error) {
	return "", ebpf.ErrNotImplemented
}

// DebugCachedConntrack is not implemented on this OS for Tracer
func (t *Tracer) DebugCachedConntrack(ctx context.Context) (interface{}, error) {
	return nil, ebpf.ErrNotImplemented
}

// DebugHostConntrack is not implemented on this OS for Tracer
func (t *Tracer) DebugHostConntrack(ctx context.Context) (interface{}, error) {
	return nil, ebpf.ErrNotImplemented
}

// DebugDumpProcessCache is not implemented on this OS for Tracer
func (t *Tracer) DebugDumpProcessCache(ctx context.Context) (interface{}, error) {
	return nil, ebpf.ErrNotImplemented
}

func newUSMMonitor(c *config.Config, dh driver.Handle) usm.Monitor {
	if !c.EnableHTTPMonitoring && !c.EnableNativeTLSMonitoring {
		return nil
	}
	log.Infof("http monitoring has been enabled")

	var monitor usm.Monitor
	var err error

	monitor, err = usm.NewWindowsMonitor(c, dh)

	if err != nil {
		log.Errorf("could not instantiate http monitor: %s", err)
		return nil
	}
	monitor.Start()
	return monitor
}
