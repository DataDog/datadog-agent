// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package tracer

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection"
	networkfilter "github.com/DataDog/datadog-agent/pkg/network/tracer/networkfilter"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Tracer struct for tracking network state and connections on Darwin
type Tracer struct {
	config         *config.Config
	connTracer     connection.Tracer
	state          network.State
	reverseDNS     dns.ReverseDNS
	closeConnChan  chan struct{}
	staticTableSet bool //nolint:unused // reserved for interface compatibility with Linux tracer
	sourceExcludes []*networkfilter.ConnectionFilter
	destExcludes   []*networkfilter.ConnectionFilter
}

// NewTracer returns an initialized tracer struct for Darwin
func NewTracer(cfg *config.Config, telemetryComp telemetry.Component, _ statsd.ClientInterface) (*Tracer, error) {
	// Create connection tracer (uses libpcap on Darwin)
	connTracer, err := connection.NewTracer(cfg, telemetryComp)
	if err != nil {
		return nil, fmt.Errorf("error creating connection tracer: %w", err)
	}

	// Initialize state for tracking connections per client
	state := network.NewState(
		telemetryComp,
		cfg.ClientStateExpiry,
		cfg.MaxClosedConnectionsBuffered,
		cfg.MaxConnectionsStateBuffered,
		cfg.MaxDNSStatsBuffered,
		cfg.MaxHTTPStatsBuffered,
		cfg.MaxKafkaStatsBuffered,
		cfg.MaxPostgresStatsBuffered,
		cfg.MaxRedisStatsBuffered,
		cfg.EnableNPMConnectionRollup,
		cfg.EnableProcessEventMonitoring,
		cfg.DNSMonitoringPortList,
	)

	// Initialize reverse DNS
	var reverseDNS dns.ReverseDNS
	if cfg.DNSInspection {
		reverseDNS, err = dns.NewReverseDNS(cfg, telemetryComp)
		if err != nil {
			log.Warnf("could not initialize DNS monitoring: %s", err)
			reverseDNS = dns.NewNullReverseDNS()
		}
	} else {
		reverseDNS = dns.NewNullReverseDNS()
	}

	tr := &Tracer{
		config:         cfg,
		connTracer:     connTracer,
		state:          state,
		reverseDNS:     reverseDNS,
		closeConnChan:  make(chan struct{}),
		sourceExcludes: networkfilter.ParseConnectionFilters(cfg.ExcludedSourceConnections),
		destExcludes:   networkfilter.ParseConnectionFilters(cfg.ExcludedDestinationConnections),
	}

	// Start the connection tracer with a callback for closed connections
	err = connTracer.Start(func(conn *network.ConnectionStats) {
		tr.storeClosedConnection(conn)
	})
	if err != nil {
		return nil, fmt.Errorf("error starting connection tracer: %w", err)
	}

	return tr, nil
}

// Stop halts all network monitoring
func (t *Tracer) Stop() {
	if t.connTracer != nil {
		t.connTracer.Stop()
	}
	if t.reverseDNS != nil {
		t.reverseDNS.Close()
	}
	close(t.closeConnChan)
}

// GetActiveConnections returns the network connections for the given client
func (t *Tracer) GetActiveConnections(clientID string) (*network.Connections, func(), error) {
	// Get connections from the tracer
	buffer := network.ClientPool.Get(clientID)
	err := t.connTracer.GetConnections(buffer.ConnectionBuffer, func(c *network.ConnectionStats) bool {
		return !t.shouldSkipConnection(c)
	})
	if err != nil {
		return nil, nil, fmt.Errorf("error getting connections: %w", err)
	}

	// Convert buffer to connections slice
	activeConns := buffer.Connections()

	// Get DNS stats if available
	var dnsStats dns.StatsByKeyByNameByType
	if t.reverseDNS != nil {
		dnsStats = t.reverseDNS.GetDNSStats()
	}

	// Get connection delta for this client from state
	delta := t.state.GetDelta(
		clientID,
		uint64(time.Now().UnixNano()),
		activeConns,
		dnsStats,
		nil, // USM stats (not implemented on Darwin yet)
	)

	// Collect unique IPs for DNS resolution
	ips := make(map[util.Address]struct{}, len(delta.Conns)/2)
	for _, conn := range delta.Conns {
		ips[conn.Source] = struct{}{}
		ips[conn.Dest] = struct{}{}
	}

	// Clear closed connections after delta computation
	t.state.RemoveExpiredClients(time.Now())

	// Assign delta connections to buffer and create Connections object
	buffer.Assign(delta.Conns)
	conns := network.NewConnections(buffer)
	if t.reverseDNS != nil {
		conns.DNS = t.reverseDNS.Resolve(ips)
	}
	conns.USMData = delta.USMData
	conns.ConnTelemetry = t.state.GetTelemetryDelta(clientID, t.getConnTelemetry())

	return conns, func() {}, nil
}

// RegisterClient registers a new client for connection tracking
func (t *Tracer) RegisterClient(clientID string) error {
	t.state.RegisterClient(clientID)
	return nil
}

// GetStats returns telemetry statistics
func (t *Tracer) GetStats() (map[string]interface{}, error) {
	stats := map[string]interface{}{
		"state": t.state.GetStats(),
		"tracer": map[string]interface{}{
			"type": "darwin_pcap",
		},
	}

	return stats, nil
}

// getConnTelemetry returns connection telemetry for the current state
func (t *Tracer) getConnTelemetry() map[network.ConnTelemetryType]int64 {
	return map[network.ConnTelemetryType]int64{}
}

// DebugNetworkState returns the current network state for debugging
func (t *Tracer) DebugNetworkState(clientID string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"state":       t.state.DumpState(clientID),
		"tracer_type": "darwin_pcap",
	}, nil
}

// DebugNetworkMaps returns connections for debugging (similar to GetActiveConnections but without delta)
func (t *Tracer) DebugNetworkMaps() (*network.Connections, error) {
	buffer := network.ClientPool.Get("debug")
	err := t.connTracer.GetConnections(buffer.ConnectionBuffer, func(c *network.ConnectionStats) bool {
		return !t.shouldSkipConnection(c)
	})
	if err != nil {
		return nil, fmt.Errorf("error getting connections: %w", err)
	}

	conns := buffer.Connections()
	buffer.Assign(conns)
	return network.NewConnections(buffer), nil
}

// DebugEBPFMaps is not applicable on Darwin (no eBPF)
func (t *Tracer) DebugEBPFMaps(w io.Writer, _ ...string) error {
	_, err := w.Write([]byte("eBPF maps not available on Darwin\n"))
	return err
}

// DebugDumpProcessCache is not implemented on Darwin yet
func (t *Tracer) DebugDumpProcessCache(_ any) (interface{}, error) {
	return map[string]string{
		"error": "process cache not implemented on Darwin",
	}, nil
}

// DebugCachedConntrack is not applicable on Darwin (no conntrack)
func (t *Tracer) DebugCachedConntrack(_ any) (*DebugConntrackTable, error) {
	return nil, errors.New("conntrack not available on Darwin")
}

// DebugHostConntrack is not applicable on Darwin (no conntrack)
func (t *Tracer) DebugHostConntrack(_ any) (*DebugConntrackTable, error) {
	return nil, errors.New("conntrack not available on Darwin")
}

// DumpProcessCache is not implemented on Darwin yet
func (t *Tracer) DumpProcessCache(w io.Writer) error {
	_, err := w.Write([]byte("process cache not implemented on Darwin\n"))
	return err
}

// storeClosedConnection handles closed connections from the tracer callback
func (t *Tracer) storeClosedConnection(conn *network.ConnectionStats) {
	if t.shouldSkipConnection(conn) {
		return
	}
	t.state.StoreClosedConnection(conn)
}

// Note: shouldSkipConnection is defined in tracer_shared.go

// DebugConntrackTable is a stub type for Darwin (conntrack is Linux-only)
type DebugConntrackTable struct{}

// WriteTo is a stub for Darwin (conntrack is Linux-only)
func (table *DebugConntrackTable) WriteTo(_ io.Writer, _ int) error {
	return errors.New("conntrack not available on Darwin")
}

// StaticTable is not applicable on Darwin
func (t *Tracer) StaticTable(_ uint8) []network.ConnectionStats {
	return nil
}
