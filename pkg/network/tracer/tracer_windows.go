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
	"io"
	"runtime"
	"sync"
	"syscall"
	"time"

	"go4.org/intern"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	driver "github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/network/events"
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

	// windows event handle for stopping the closed connection event loop
	hStopClosedLoopEvent windows.Handle

	processCache *processCache
}

// NewTracer returns an initialized tracer struct
func NewTracer(config *config.Config, telemetry telemetry.Component) (*Tracer, error) {
	if err := driver.Start(); err != nil {
		return nil, fmt.Errorf("error starting driver: %s", err)
	}
	di, err := network.NewDriverInterface(config, driver.NewHandle, telemetry)

	if err != nil && errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		log.Debugf("could not create driver interface: %v", err)
		return nil, fmt.Errorf("The Windows driver was not installed, reinstall the Datadog Agent with network performance monitoring enabled")
	} else if err != nil {
		return nil, fmt.Errorf("could not create windows driver controller: %v", err)
	}

	state := network.NewState(
		telemetry,
		config.ClientStateExpiry,
		config.MaxClosedConnectionsBuffered,
		config.MaxConnectionsStateBuffered,
		config.MaxDNSStatsBuffered,
		config.MaxHTTPStatsBuffered,
		config.MaxKafkaStatsBuffered,
		config.MaxPostgresStatsBuffered,
		config.MaxRedisStatsBuffered,
		config.EnableNPMConnectionRollup,
		config.EnableProcessEventMonitoring,
	)

	reverseDNS := dns.NewNullReverseDNS()
	if config.DNSInspection {
		reverseDNS, err = dns.NewReverseDNS(config, telemetry)
		if err != nil {
			return nil, err
		}
	}

	stopEvent, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create stop event: %w", err)
	}
	tr := &Tracer{
		config:               config,
		driverInterface:      di,
		stopChan:             make(chan struct{}),
		timerInterval:        defaultPollInterval,
		state:                state,
		closedBuffer:         network.NewConnectionBuffer(defaultBufferSize, minBufferSize),
		reverseDNS:           reverseDNS,
		usmMonitor:           newUSMMonitor(config, di.GetHandle()),
		sourceExcludes:       network.ParseConnectionFilters(config.ExcludedSourceConnections),
		destExcludes:         network.ParseConnectionFilters(config.ExcludedDestinationConnections),
		hStopClosedLoopEvent: stopEvent,
	}
	if config.EnableProcessEventMonitoring {
		if tr.processCache, err = newProcessCache(config.MaxProcessesTracked); err != nil {
			return nil, fmt.Errorf("could not create process cache; %w", err)
		}
		if telemetry != nil {
			// the tests don't have a telemetry component
			telemetry.RegisterCollector(tr.processCache)
		}

		if err = events.Init(); err != nil {
			return nil, fmt.Errorf("could not initialize event monitoring: %w", err)
		}

		events.RegisterHandler(tr.processCache)
	}

	tr.closedEventLoop.Add(1)
	go func() {
		defer tr.closedEventLoop.Done()

		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	waitloop:
		for {
			handles := []windows.Handle{tr.hStopClosedLoopEvent, di.GetClosedFlowsEvent()}

			evt, _ := windows.WaitForMultipleObjects(handles, false, windows.INFINITE)
			switch evt {
			case windows.WAIT_OBJECT_0:
				log.Infof("stopping closed connection event loop")
				break waitloop

			case windows.WAIT_OBJECT_0 + 1:
				_, err = tr.driverInterface.GetClosedConnectionStats(tr.closedBuffer, func(c *network.ConnectionStats) bool {
					return !tr.shouldSkipConnection(c)
				})
				closedConnStats := tr.closedBuffer.Connections()

				for i := range closedConnStats {
					tr.addProcessInfo(&closedConnStats[i])
					tr.state.StoreClosedConnection(&closedConnStats[i])
				}

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

	windows.SetEvent(t.hStopClosedLoopEvent)
	t.closedEventLoop.Wait()
	err := t.driverInterface.Close()
	if err != nil {
		log.Errorf("error closing driver interface: %s", err)
	}
	if err := driver.Stop(); err != nil {
		log.Errorf("error stopping driver: %s", err)
	}
	windows.CloseHandle(t.hStopClosedLoopEvent)
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

	for i := range activeConnStats {
		t.addProcessInfo(&activeConnStats[i])
	}
	for i := range closedConnStats {
		t.addProcessInfo(&closedConnStats[i])
		t.state.StoreClosedConnection(&closedConnStats[i])
	}

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
	return conns, nil
}

// RegisterClient registers the client
func (t *Tracer) RegisterClient(clientID string) error {
	t.state.RegisterClient(clientID)
	return nil
}

func (t *Tracer) removeClient(clientID string) {
	t.state.RemoveClient(clientID)
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
func (t *Tracer) DebugEBPFMaps(_ io.Writer, _ ...string) error {
	return ebpf.ErrNotImplemented
}

// DebugConntrackTable is not implemented on this OS for Tracer
type DebugConntrackTable struct{}

// WriteTo is not implemented on this OS for Tracer
func (table *DebugConntrackTable) WriteTo(_ io.Writer, _ int) error {
	return ebpf.ErrNotImplemented
}

// DebugCachedConntrack is not implemented on this OS for Tracer
func (t *Tracer) DebugCachedConntrack(_ context.Context) (*DebugConntrackTable, error) {
	return nil, ebpf.ErrNotImplemented
}

// DebugHostConntrack is not implemented on this OS for Tracer
func (t *Tracer) DebugHostConntrack(_ context.Context) (*DebugConntrackTable, error) {
	return nil, ebpf.ErrNotImplemented
}

// DebugHostConntrackFull is not implemented on this OS for Tracer
func (t *Tracer) DebugHostConntrackFull(_ context.Context) (*DebugConntrackTable, error) {
	return nil, ebpf.ErrNotImplemented
}

// DebugDumpProcessCache is not implemented on this OS for Tracer
func (t *Tracer) DebugDumpProcessCache(_ context.Context) (interface{}, error) {
	return nil, ebpf.ErrNotImplemented
}

// GetNetworkID is not implemented on this OS for Tracer
func (t *Tracer) GetNetworkID(_ context.Context) (string, error) {
	return "", ebpf.ErrNotImplemented
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

func (t *Tracer) addProcessInfo(c *network.ConnectionStats) {
	if t.processCache == nil {
		return
	}

	c.ContainerID.Source, c.ContainerID.Dest = nil, nil

	// on windows, cLastUpdateEpoch is already set as
	// ns since unix epoch.
	ts := c.LastUpdateEpoch
	p, ok := t.processCache.Get(c.Pid, int64(ts))
	if !ok {
		return
	}

	if len(p.Tags) > 0 {
		c.Tags = make([]*intern.Value, len(p.Tags))
		copy(c.Tags, p.Tags)
	}

	if p.ContainerID != nil {
		c.ContainerID.Source = p.ContainerID
	}
}
