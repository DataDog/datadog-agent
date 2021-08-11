// +build windows,npm

package tracer

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultPollInterval = int(15)
	defaultBufferSize   = 512
)

// Tracer struct for tracking network state and connections
type Tracer struct {
	config          *config.Config
	driverInterface *network.DriverInterface
	stopChan        chan struct{}
	state           network.State
	reverseDNS      dns.ReverseDNS

	connStatsActive *network.DriverBuffer
	connStatsClosed *network.DriverBuffer
	connLock        sync.Mutex

	timerInterval int

	// ticker for the polling interval for writing
	inTicker            *time.Ticker
	stopInTickerRoutine chan bool

	// Connections for the tracer to exclude
	sourceExcludes []*network.ConnectionFilter
	destExcludes   []*network.ConnectionFilter
}

// NewTracer returns an initialized tracer struct
func NewTracer(config *config.Config) (*Tracer, error) {
	di, err := network.NewDriverInterface(config)

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
		config.CollectDNSDomains,
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
		connStatsActive: network.NewDriverBuffer(defaultBufferSize),
		connStatsClosed: network.NewDriverBuffer(defaultBufferSize),
		reverseDNS:      reverseDNS,
		sourceExcludes:  network.ParseConnectionFilters(config.ExcludedSourceConnections),
		destExcludes:    network.ParseConnectionFilters(config.ExcludedDestinationConnections),
	}

	return tr, nil
}

// Stop function stops running tracer
func (t *Tracer) Stop() {
	close(t.stopChan)
	t.reverseDNS.Close()
	err := t.driverInterface.Close()
	if err != nil {
		log.Errorf("error closing driver interface: %s", err)
	}
}

// GetActiveConnections returns all active connections
func (t *Tracer) GetActiveConnections(clientID string) (*network.Connections, error) {
	t.connLock.Lock()
	defer t.connLock.Unlock()

	t.connStatsActive.Reset()
	t.connStatsClosed.Reset()

	_, _, err := t.driverInterface.GetConnectionStats(t.connStatsActive, t.connStatsClosed, func(c *network.ConnectionStats) bool {
		return !t.shouldSkipConnection(c)
	})
	if err != nil {
		return nil, fmt.Errorf("error retrieving connections from driver: %w", err)
	}

	activeConnStats := t.connStatsActive.Connections()
	closedConnStats := t.connStatsClosed.Connections()

	for _, connStat := range closedConnStats {
		t.state.StoreClosedConnection(&connStat)
	}

	// check for expired clients in the state
	t.state.RemoveExpiredClients(time.Now())

	delta := t.state.GetDelta(clientID, uint64(time.Now().Nanosecond()), activeConnStats, t.reverseDNS.GetDNSStats(), nil)
	conns := delta.Connections
	var ips []util.Address
	for _, conn := range delta.Connections {
		ips = append(ips, conn.Source, conn.Dest)
	}
	names := t.reverseDNS.Resolve(ips)
	return &network.Connections{Conns: conns, DNS: names}, nil
}

// GetStats returns a map of statistics about the current tracer's internal state
func (t *Tracer) GetStats() (map[string]interface{}, error) {
	driverStats, err := t.driverInterface.GetStats()
	if err != nil {
		log.Errorf("not printing driver stats: %v", err)
	}

	stats := map[string]interface{}{
		"state": t.state.GetStats(),
		"dns":   t.reverseDNS.GetStats(),
	}
	for _, name := range network.DriverExpvarNames {
		stats[string(name)] = driverStats[name]
	}
	return stats, nil
}

// DebugNetworkState returns a map with the current tracer's internal state, for debugging
func (t *Tracer) DebugNetworkState(_ string) (map[string]interface{}, error) {
	return nil, ebpf.ErrNotImplemented
}

// DebugNetworkMaps returns all connections stored in the maps without modifications from network state
func (t *Tracer) DebugNetworkMaps() (*network.Connections, error) {
	return nil, ebpf.ErrNotImplemented
}
