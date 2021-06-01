// +build windows

package tracer

import (
	"fmt"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/pkg/errors"
)

const (
	defaultPollInterval = int(15)
)

// Tracer struct for tracking network state and connections
type Tracer struct {
	config          *config.Config
	driverInterface *network.DriverInterface
	stopChan        chan struct{}
	state           network.State
	reverseDNS      network.ReverseDNS

	connStatsActive *network.DriverBuffer
	connStatsClosed *network.DriverBuffer
	connLock        sync.Mutex

	timerInterval int

	// ticker for the polling interval for writing
	inTicker            *time.Ticker
	stopInTickerRoutine chan bool
}

// NewTracer returns an initialized tracer struct
func NewTracer(config *config.Config) (*Tracer, error) {
	di, err := network.NewDriverInterface(config)

	if err != nil && errors.Cause(err) == syscall.Errno(syscall.ERROR_FILE_NOT_FOUND) {
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

	packetSrc := network.NewWindowsPacketSource(di)

	reverseDNS, err := network.NewSocketFilterSnooper(config, packetSrc)
	if err != nil {
		return nil, err
	}

	tr := &Tracer{
		driverInterface: di,
		stopChan:        make(chan struct{}),
		timerInterval:   defaultPollInterval,
		state:           state,
		connStatsActive: network.NewDriverBuffer(512),
		connStatsClosed: network.NewDriverBuffer(512),
		reverseDNS:      reverseDNS,
	}

	return tr, nil
}

// Stop function stops running tracer
func (t *Tracer) Stop() {
	close(t.stopChan)
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

	_, _, err := t.driverInterface.GetConnectionStats(t.connStatsActive, t.connStatsClosed)
	if err != nil {
		log.Errorf("failed to get connections")
		return nil, err
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
	names := t.reverseDNS.Resolve(conns)
	return &network.Connections{Conns: conns, DNS: names}, nil
}

// GetStats returns a map of statistics about the current tracer's internal state
func (t *Tracer) GetStats() (map[string]interface{}, error) {
	driverStats, err := t.driverInterface.GetStats()
	if err != nil {
		log.Errorf("not printing driver stats: %v", err)
	}

	stateStats := t.state.GetStats()
	stats := map[string]interface{}{
		"state": stateStats,
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
