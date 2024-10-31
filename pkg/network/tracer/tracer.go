// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package tracer implements the functionality of the network tracer
package tracer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/cihub/seelog"
	"github.com/cilium/ebpf"
	"go.uber.org/atomic"
	"go4.org/intern"

	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/config/sysctl"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/events"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection"
	"github.com/DataDog/datadog-agent/pkg/network/usm"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/ktime"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultUDPConnTimeoutNanoSeconds = uint64(time.Duration(120) * time.Second)
const tracerModuleName = "network_tracer"

// Telemetry
// Will track the count of expired TCP connections
// We are manually expiring TCP connections because it seems that we are losing some TCP close events
// For now we are only tracking the `tcp_close` probe, but we should also track the `tcp_set_state` probe when
// states are set to `TCP_CLOSE_WAIT`, `TCP_CLOSE` and `TCP_FIN_WAIT1` we should probably also track `tcp_time_wait`
// However there are some caveats by doing that:
// - `tcp_set_state` does not have access to the PID of the connection => we have to remove the PID from the connection tuple (which can lead to issues)
// - We will have multiple probes that can trigger for the same connection close event => we would have to add something to dedupe those
// - - Using the timestamp does not seem to be reliable (we are already seeing unordered connections)
// - - Having IDs for those events would need to have an internal monotonic counter and this is tricky to manage (race conditions, cleaning)
//
// If we want to have a way to track the # of active TCP connections in the future we could use the procfs like here: https://github.com/DataDog/datadog-agent/pull/3728
// to determine whether a connection is truly closed or not
var tracerTelemetry = struct {
	skippedConns         telemetry.Counter
	expiredTCPConns      telemetry.Counter
	closedConns          *telemetry.StatCounterWrapper
	connStatsMapSize     telemetry.Gauge
	payloadSizePerClient telemetry.Gauge
}{
	telemetry.NewCounter(tracerModuleName, "skipped_conns", []string{"ip_proto"}, "Counter measuring skipped connections"),
	telemetry.NewCounter(tracerModuleName, "expired_tcp_conns", []string{}, "Counter measuring expired TCP connections"),
	telemetry.NewStatCounterWrapper(tracerModuleName, "closed_conns", []string{"ip_proto"}, "Counter measuring closed TCP connections"),
	telemetry.NewGauge(tracerModuleName, "conn_stats_map_size", []string{}, "Gauge measuring the size of the active connections map"),
	telemetry.NewGauge(tracerModuleName, "payload_conn_count", []string{"client_id", "ip_proto"}, "Gauge measuring the number of connections in the system-probe payload"),
}

// Tracer implements the functionality of the network tracer
type Tracer struct {
	config      *config.Config
	state       network.State
	conntracker netlink.Conntracker
	reverseDNS  dns.ReverseDNS
	usmMonitor  *usm.Monitor
	ebpfTracer  connection.Tracer
	lastCheck   *atomic.Int64

	bufferLock sync.Mutex

	// Connections for the tracer to exclude
	sourceExcludes []*network.ConnectionFilter
	destExcludes   []*network.ConnectionFilter

	gwLookup network.GatewayLookup

	sysctlUDPConnTimeout       *sysctl.Int
	sysctlUDPConnStreamTimeout *sysctl.Int

	processCache *processCache

	timeResolver *ktime.Resolver

	telemetryComp telemetryComponent.Component

	// Used for connection_protocol data expiration
	connectionProtocolMapCleaner *ddebpf.MapCleaner[netebpf.ConnTuple, netebpf.ProtocolStackWrapper]
}

// NewTracer creates a Tracer
func NewTracer(config *config.Config, telemetryComponent telemetryComponent.Component) (*Tracer, error) {
	tr, err := newTracer(config, telemetryComponent)
	if err != nil {
		return nil, err
	}

	if err := tr.start(); err != nil {
		return nil, err
	}

	return tr, nil
}

// newTracer is an internal function used by tests primarily
// (and NewTracer above)
func newTracer(cfg *config.Config, telemetryComponent telemetryComponent.Component) (_ *Tracer, reterr error) {
	// check if current platform is using old kernel API because it affects what kprobe are we going to enable
	currKernelVersion, err := kernel.HostVersion()
	if err != nil {
		// if the platform couldn't be determined, treat it as new kernel case
		log.Warn("could not detect the kernel version, will use kprobes from kernel version >= 4.1.0")
	}

	// check to see if current kernel is earlier than version 4.1.0
	pre410Kernel := currKernelVersion < kernel.VersionCode(4, 1, 0)
	if pre410Kernel {
		log.Infof("detected kernel version %s, will use kprobes from kernel version < 4.1.0", currKernelVersion)
	}

	if cfg.ServiceMonitoringEnabled {
		if err := usmconfig.CheckUSMSupported(cfg); err != nil {
			// this is the case where USM is enabled and NPM is not enabled
			// in config; we implicitly enable the network tracer module
			// in system-probe if USM is enabled
			if !cfg.NPMEnabled {
				return nil, err
			}

			log.Warn(err)
			log.Warnf("NPM is explicitly enabled, so system-probe will continue with only NPM features enabled")
		}
	}

	tr := &Tracer{
		config:                     cfg,
		lastCheck:                  atomic.NewInt64(time.Now().Unix()),
		sysctlUDPConnTimeout:       sysctl.NewInt(cfg.ProcRoot, "net/netfilter/nf_conntrack_udp_timeout", time.Minute),
		sysctlUDPConnStreamTimeout: sysctl.NewInt(cfg.ProcRoot, "net/netfilter/nf_conntrack_udp_timeout_stream", time.Minute),
		telemetryComp:              telemetryComponent,
	}
	defer func() {
		if reterr != nil {
			tr.Stop()
		}
	}()

	tr.ebpfTracer, err = connection.NewTracer(cfg, telemetryComponent)
	if err != nil {
		return nil, err
	}
	telemetry.GetCompatComponent().RegisterCollector(tr.ebpfTracer)

	tr.conntracker, err = newConntracker(cfg, telemetryComponent)
	if err != nil {
		return nil, err
	}
	telemetry.GetCompatComponent().RegisterCollector(tr.conntracker)

	if cfg.EnableGatewayLookup {
		tr.gwLookup = network.NewGatewayLookup(cfg.GetRootNetNs, cfg.MaxTrackedConnections, telemetryComponent)
	}
	if tr.gwLookup != nil {
		log.Info("gateway lookup enabled")
	}

	tr.reverseDNS = newReverseDNS(cfg, telemetryComponent)
	tr.usmMonitor = newUSMMonitor(cfg, tr.ebpfTracer)

	// Set up the connection_protocol map cleaner if protocol classification is enabled
	if cfg.ProtocolClassificationEnabled || usmconfig.IsUSMSupportedAndEnabled(cfg) {
		connectionProtocolMap, err := tr.ebpfTracer.GetMap(probes.ConnectionProtocolMap)
		if err == nil {
			tr.connectionProtocolMapCleaner, err = setupConnectionProtocolMapCleaner(connectionProtocolMap)
			if err != nil {
				log.Warnf("could not set up connection protocol map cleaner: %s", err)
			}
		} else {
			log.Warnf("couldn't get %q map, will not be able to expire connection protocol data: %s", probes.ConnectionProtocolMap, err)
		}
	}

	if cfg.EnableProcessEventMonitoring {
		if tr.processCache, err = newProcessCache(cfg.MaxProcessesTracked); err != nil {
			return nil, fmt.Errorf("could not create process cache; %w", err)
		}
		telemetry.GetCompatComponent().RegisterCollector(tr.processCache)

		if tr.timeResolver, err = ktime.NewResolver(); err != nil {
			return nil, fmt.Errorf("could not create time resolver: %w", err)
		}

		if err = events.Init(); err != nil {
			return nil, fmt.Errorf("could not initialize event monitoring: %w", err)
		}

		events.RegisterHandler(tr.processCache)
	}

	tr.sourceExcludes = network.ParseConnectionFilters(cfg.ExcludedSourceConnections)
	tr.destExcludes = network.ParseConnectionFilters(cfg.ExcludedDestinationConnections)
	tr.state = network.NewState(
		telemetryComponent,
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
	)

	return tr, nil
}

// start starts the tracer. This function is present to separate
// the creation from the start of the tracer for tests
func (t *Tracer) start() error {
	err := t.ebpfTracer.Start(t.storeClosedConnection)
	if err != nil {
		t.Stop()
		return fmt.Errorf("could not start ebpf tracer: %s", err)
	}

	if err = t.reverseDNS.Start(); err != nil {
		t.Stop()
		return fmt.Errorf("could not start reverse dns monitor: %w", err)
	}

	return nil
}

func newConntracker(cfg *config.Config, telemetryComponent telemetryComponent.Component) (netlink.Conntracker, error) {
	if !cfg.EnableConntrack {
		return netlink.NewNoOpConntracker(), nil
	}

	var c netlink.Conntracker
	var err error

	if !cfg.EnableEbpfless {
		ns, err := cfg.GetRootNetNs()
		if err != nil {
			log.Warnf("error fetching root net namespace, will not attempt to load nf_conntrack_netlink module: %s", err)
		} else {
			defer ns.Close()
			if err = netlink.LoadNfConntrackKernelModule(ns); err != nil {
				log.Warnf("failed to load conntrack kernel module, though it may already be loaded: %s", err)
			}
		}
		if cfg.EnableEbpfConntracker {
			if c, err = NewEBPFConntracker(cfg, telemetryComponent); err == nil {
				return c, nil
			}
			log.Warnf("error initializing ebpf conntracker: %s", err)
		} else {
			log.Info("ebpf conntracker disabled")
		}

		log.Info("falling back to netlink conntracker")
	}

	if c, err = netlink.NewConntracker(cfg, telemetryComponent); err == nil {
		return c, nil
	}

	if errors.Is(err, netlink.ErrNotPermitted) || cfg.IgnoreConntrackInitFailure {
		log.Warnf("could not initialize conntrack, tracer will continue without NAT tracking: %s", err)
		return netlink.NewNoOpConntracker(), nil
	}

	return nil, fmt.Errorf("error initializing conntracker: %s. set network_config.ignore_conntrack_init_failure to true to ignore conntrack failures on startup", err)
}

func newReverseDNS(c *config.Config, telemetrycomp telemetryComponent.Component) dns.ReverseDNS {
	if !c.DNSInspection {
		return dns.NewNullReverseDNS()
	}

	rdns, err := dns.NewReverseDNS(c, telemetrycomp)
	if err != nil {
		log.Errorf("could not instantiate dns inspector: %s", err)
		return dns.NewNullReverseDNS()
	}

	log.Info("dns inspection enabled")
	return rdns
}

// storeClosedConnection is triggered when:
// * the current closed connection batch fills up
// * the client asks for the current connections
// this function is responsible for storing the closed connections in the state and
// matching failed connections to closed connections
func (t *Tracer) storeClosedConnection(cs *network.ConnectionStats) {
	cs.IsClosed = true
	if t.shouldSkipConnection(cs) {
		tracerTelemetry.skippedConns.IncWithTags(cs.Type.Tags())
		return
	}

	cs.IPTranslation = t.conntracker.GetTranslationForConn(&cs.ConnectionTuple)
	t.connVia(cs)
	if cs.IPTranslation != nil {
		t.conntracker.DeleteTranslation(&cs.ConnectionTuple)
	}

	t.addProcessInfo(cs)

	tracerTelemetry.closedConns.IncWithTags(cs.Type.Tags())
	t.ebpfTracer.GetFailedConnections().MatchFailedConn(cs)

	t.state.StoreClosedConnection(cs)
}

func (t *Tracer) addProcessInfo(c *network.ConnectionStats) {
	if t.processCache == nil {
		return
	}

	c.ContainerID.Source, c.ContainerID.Dest = nil, nil

	ts := t.timeResolver.ResolveMonotonicTimestamp(c.LastUpdateEpoch)
	p, ok := t.processCache.Get(c.Pid, ts.UnixNano())
	if !ok {
		return
	}

	if log.ShouldLog(seelog.TraceLvl) {
		log.Tracef("got process cache entry for pid %d: %+v", c.Pid, p)
	}

	if len(p.Tags) > 0 {
		c.Tags = make([]*intern.Value, len(p.Tags))
		copy(c.Tags, p.Tags)
	}

	if p.ContainerID != nil {
		c.ContainerID.Source = p.ContainerID
	}
}

// Pause bypasses the eBPF programs
func (t *Tracer) Pause() error {
	if err := t.ebpfTracer.Pause(); err != nil {
		return err
	}
	if err := t.usmMonitor.Pause(); err != nil {
		return err
	}
	return nil
}

// Resume enables the previously bypassed eBPF programs
func (t *Tracer) Resume() error {
	if err := t.ebpfTracer.Resume(); err != nil {
		return err
	}
	if err := t.usmMonitor.Resume(); err != nil {
		return err
	}
	return nil
}

// Stop stops the tracer
func (t *Tracer) Stop() {
	if t.gwLookup != nil {
		t.gwLookup.Close()
	}
	if t.reverseDNS != nil {
		t.reverseDNS.Close()
	}
	if t.ebpfTracer != nil {
		t.ebpfTracer.Stop()
		telemetry.GetCompatComponent().UnregisterCollector(t.ebpfTracer)
	}
	if t.usmMonitor != nil {
		t.usmMonitor.Stop()
	}
	if t.conntracker != nil {
		t.conntracker.Close()
		telemetry.GetCompatComponent().UnregisterCollector(t.conntracker)
	}
	if t.processCache != nil {
		events.UnregisterHandler(t.processCache)
		t.processCache.Stop()
		telemetry.GetCompatComponent().UnregisterCollector(t.processCache)
	}
	t.connectionProtocolMapCleaner.Stop()
}

// GetActiveConnections returns the delta for connection info from the last time it was called with the same clientID
func (t *Tracer) GetActiveConnections(clientID string) (*network.Connections, error) {
	t.bufferLock.Lock()
	defer t.bufferLock.Unlock()
	if log.ShouldLog(seelog.TraceLvl) {
		log.Tracef("GetActiveConnections clientID=%s", clientID)
	}
	t.ebpfTracer.FlushPending()

	buffer := network.ClientPool.Get(clientID)
	latestTime, active, err := t.getConnections(buffer.ConnectionBuffer)
	if err != nil {
		return nil, fmt.Errorf("error retrieving connections: %s", err)
	}

	delta := t.state.GetDelta(clientID, latestTime, active, t.reverseDNS.GetDNSStats(), t.usmMonitor.GetProtocolStats())

	ips := make(map[util.Address]struct{}, len(delta.Conns)/2)
	var udpConns, tcpConns int
	for i := range delta.Conns {
		conn := &delta.Conns[i]
		ips[conn.Source] = struct{}{}
		ips[conn.Dest] = struct{}{}
		switch conn.Type {
		case network.UDP:
			udpConns++
		case network.TCP:
			tcpConns++
		}
	}

	tracerTelemetry.payloadSizePerClient.Set(float64(udpConns), clientID, network.UDP.String())
	tracerTelemetry.payloadSizePerClient.Set(float64(tcpConns), clientID, network.TCP.String())

	buffer.ConnectionBuffer.Assign(delta.Conns)
	conns := network.NewConnections(buffer)
	conns.DNS = t.reverseDNS.Resolve(ips)
	conns.HTTP = delta.HTTP
	conns.HTTP2 = delta.HTTP2
	conns.Kafka = delta.Kafka
	conns.Postgres = delta.Postgres
	conns.Redis = delta.Redis
	conns.ConnTelemetry = t.state.GetTelemetryDelta(clientID, t.getConnTelemetry(len(active)))
	conns.CompilationTelemetryByAsset = t.getRuntimeCompilationTelemetry()
	conns.KernelHeaderFetchResult = int32(kernel.HeaderProvider.GetResult())
	conns.CORETelemetryByAsset = ebpftelemetry.GetCORETelemetryByAsset()
	conns.PrebuiltAssets = netebpf.GetModulesInUse()
	t.lastCheck.Store(time.Now().Unix())

	return conns, nil
}

// RegisterClient registers a clientID with the tracer
func (t *Tracer) RegisterClient(clientID string) error {
	t.state.RegisterClient(clientID)
	return nil
}

func (t *Tracer) removeClient(clientID string) {
	t.state.RemoveClient(clientID)
}

func (t *Tracer) getConnTelemetry(mapSize int) map[network.ConnTelemetryType]int64 {
	kprobeStats := ebpftelemetry.GetProbeTotals()
	tm := map[network.ConnTelemetryType]int64{
		network.MonotonicKprobesTriggered: int64(kprobeStats.Hits),
		network.MonotonicKprobesMissed:    int64(kprobeStats.Misses),
		network.ConnsBpfMapSize:           int64(mapSize),
		network.MonotonicConnsClosed:      tracerTelemetry.closedConns.Load(),
	}

	stats, err := t.getStats(stateStats)
	if err != nil {
		return nil
	}

	stateStats := stats["state"].(map[string]int64)
	if ccd, ok := stateStats["closed_conn_dropped"]; ok {
		tm[network.MonotonicClosedConnDropped] = ccd
	}
	if cd, ok := stateStats["conn_dropped"]; ok {
		tm[network.MonotonicConnDropped] = cd
	}

	return tm
}

func (t *Tracer) getRuntimeCompilationTelemetry() map[string]network.RuntimeCompilationTelemetry {
	telemetryByAsset := map[string]runtime.CompilationTelemetry{
		"tracer":          runtime.Tracer.GetTelemetry(),
		"conntrack":       runtime.Conntrack.GetTelemetry(),
		"usm":             runtime.Usm.GetTelemetry(),
		"oomKill":         runtime.OomKill.GetTelemetry(),
		"runtimeSecurity": runtime.RuntimeSecurity.GetTelemetry(),
		"tcpQueueLength":  runtime.TcpQueueLength.GetTelemetry(),
	}

	result := make(map[string]network.RuntimeCompilationTelemetry)
	for assetName, telemetry := range telemetryByAsset {
		tm := network.RuntimeCompilationTelemetry{
			RuntimeCompilationEnabled:  telemetry.CompilationEnabled(),
			RuntimeCompilationResult:   telemetry.CompilationResult(),
			RuntimeCompilationDuration: telemetry.CompilationDurationNS(),
		}
		result[assetName] = tm
	}

	return result
}

// getConnections returns all the active connections in the ebpf maps along with the latest timestamp.  It takes
// a reusable buffer for appending the active connections so that this doesn't continuously allocate
func (t *Tracer) getConnections(activeBuffer *network.ConnectionBuffer) (latestUint uint64, activeConnections []network.ConnectionStats, err error) {
	cachedConntrack := newCachedConntrack(t.config.ProcRoot, netlink.NewConntrack, 128)
	defer func() { _ = cachedConntrack.Close() }()

	latestTime, err := ddebpf.NowNanoseconds()
	if err != nil {
		return 0, nil, fmt.Errorf("error retrieving latest timestamp: %s", err)
	}

	var expired []network.ConnectionStats
	err = t.ebpfTracer.GetConnections(activeBuffer, func(c *network.ConnectionStats) bool {
		if t.connectionExpired(c, uint64(latestTime), cachedConntrack) {
			expired = append(expired, *c)
			if c.Type == network.TCP {
				tracerTelemetry.expiredTCPConns.Inc()
			}
			tracerTelemetry.closedConns.IncWithTags(c.Type.Tags())
			return false
		}

		if t.shouldSkipConnection(c) {
			tracerTelemetry.skippedConns.IncWithTags(c.Type.Tags())
			return false
		}
		return true
	})
	if err != nil {
		return 0, nil, err
	}

	activeConnections = activeBuffer.Connections()
	for i := range activeConnections {
		activeConnections[i].IPTranslation = t.conntracker.GetTranslationForConn(&activeConnections[i].ConnectionTuple)
		// do gateway resolution only on active connections outside
		// the map iteration loop to not add to connections while
		// iterating (leads to ever-increasing connections in the map,
		// since gateway resolution connects to the ec2 metadata
		// endpoint)
		t.connVia(&activeConnections[i])
		t.addProcessInfo(&activeConnections[i])
	}

	// get rid of stale process entries in the cache
	t.processCache.Trim()

	// remove stale failed connections from map
	t.ebpfTracer.GetFailedConnections().RemoveExpired()

	entryCount := len(activeConnections)
	if entryCount >= int(t.config.MaxTrackedConnections) {
		log.Errorf("connection tracking map size has reached the limit of %d. Accurate connection count and data volume metrics will be affected. Increase config value `system_probe_config.max_tracked_connections` to correct this.", t.config.MaxTrackedConnections)
	} else if (float64(entryCount) / float64(t.config.MaxTrackedConnections)) >= 0.9 {
		log.Warnf("connection tracking map size of %d is approaching the limit of %d. The config value `system_probe_config.max_tracked_connections` may be increased to avoid any accuracy problems.", entryCount, t.config.MaxTrackedConnections)
	}
	tracerTelemetry.connStatsMapSize.Set(float64(entryCount))

	// Remove expired entries
	t.removeEntries(expired)

	// check for expired clients in the state
	t.state.RemoveExpiredClients(time.Now())

	latestTime, err = ddebpf.NowNanoseconds()
	if err != nil {
		return 0, nil, fmt.Errorf("error retrieving latest timestamp: %s", err)
	}
	return uint64(latestTime), activeConnections, nil
}

func (t *Tracer) removeEntries(entries []network.ConnectionStats) {
	now := time.Now()
	// Byte keys of the connections to remove
	toRemove := make([]*network.ConnectionStats, 0, len(entries))
	// Remove the entries from the eBPF Map
	for i := range entries {
		entry := &entries[i]
		err := t.ebpfTracer.Remove(entry)
		if err != nil {
			if !errors.Is(err, ebpf.ErrKeyNotExist) {
				log.Warnf("failed to remove entry from connections: %s", err)
			}
			continue
		}

		// Delete conntrack entry for this connection
		t.conntracker.DeleteTranslation(&entry.ConnectionTuple)

		// Append the connection key to the keys to remove from the userspace state
		toRemove = append(toRemove, entry)
	}

	t.state.RemoveConnections(toRemove)

	if log.ShouldLog(seelog.DebugLvl) {
		log.Debugf("Removed %d connection entries in %s", len(toRemove), time.Since(now))
	}
}

func (t *Tracer) timeoutForConn(c *network.ConnectionStats) uint64 {
	if c.Type == network.TCP {
		return uint64(t.config.TCPConnTimeout.Nanoseconds())
	}

	return t.udpConnTimeout(c.IsAssured)
}

func (t *Tracer) udpConnTimeout(isAssured bool) uint64 {
	if isAssured {
		if v, err := t.sysctlUDPConnStreamTimeout.Get(); err == nil {
			return uint64(time.Duration(v) * time.Second)
		}

	} else {
		if v, err := t.sysctlUDPConnTimeout.Get(); err == nil {
			return uint64(time.Duration(v) * time.Second)
		}
	}

	return defaultUDPConnTimeoutNanoSeconds
}

type statsComp int

const (
	conntrackStats statsComp = iota
	dnsStats
	epbfStats
	gatewayLookupStats
	httpStats
	kprobesStats
	stateStats
	tracerStats
	processCacheStats
	kafkaStats
)

var allStats = []statsComp{
	stateStats,
	tracerStats,
	httpStats,
}

func (t *Tracer) getStats(comps ...statsComp) (map[string]interface{}, error) {
	if t.state == nil {
		return nil, fmt.Errorf("internal state not yet initialized")
	}

	if len(comps) == 0 {
		comps = allStats
	}

	ret := map[string]interface{}{}
	for _, c := range comps {
		switch c {
		case stateStats:
			ret["state"] = t.state.GetStats()["telemetry"]
		case tracerStats:
			tracerStats := make(map[string]interface{})
			tracerStats["last_check"] = t.lastCheck.Load()
			tracerStats["runtime"] = runtime.Tracer.GetTelemetry()
			ret["tracer"] = tracerStats
		case httpStats:
			ret["universal_service_monitoring"] = t.usmMonitor.GetUSMStats()
		}
	}

	return ret, nil
}

// GetStats returns a map of statistics about the current tracer's internal state
func (t *Tracer) GetStats() (map[string]interface{}, error) {
	return map[string]interface{}{
		"tracer": map[string]interface{}{
			"last_check": t.lastCheck.Load(),
		},
		"universal_service_monitoring": t.usmMonitor.GetUSMStats(),
	}, nil
}

// DebugNetworkState returns a map with the current tracer's internal state, for debugging
func (t *Tracer) DebugNetworkState(clientID string) (map[string]interface{}, error) {
	if t.state == nil {
		return nil, fmt.Errorf("internal state not yet initialized")
	}
	return t.state.DumpState(clientID), nil
}

// DebugNetworkMaps returns all connections stored in the BPF maps without modifications from network state
func (t *Tracer) DebugNetworkMaps() (*network.Connections, error) {
	activeBuffer := network.NewConnectionBuffer(512, 512)
	_, connections, err := t.getConnections(activeBuffer)
	if err != nil {
		return nil, fmt.Errorf("error retrieving connections: %s", err)
	}
	return &network.Connections{
		BufferedData: network.BufferedData{
			Conns: connections,
		},
	}, nil

}

// DebugEBPFMaps returns all maps registered in the eBPF manager
func (t *Tracer) DebugEBPFMaps(w io.Writer, maps ...string) error {
	io.WriteString(w, "tracer:\n")
	err := t.ebpfTracer.DumpMaps(w, maps...)
	if err != nil {
		return err
	}

	if t.usmMonitor != nil {
		io.WriteString(w, "usm_monitor:\n")
		err := t.usmMonitor.DumpMaps(w, maps...)
		if err != nil {
			return err
		}
	}

	return nil
}

// connectionExpired returns true if the passed in connection has expired
//
// expiry is handled differently for UDP and TCP. For TCP where conntrack TTL is very long, we use a short expiry for userspace tracking
// but use conntrack as a source of truth to keep long-lived idle TCP conns in the userspace state, while evicting closed TCP connections.
// for UDP, the conntrack TTL is lower (two minutes), so the userspace and conntrack expiry are synced to avoid touching conntrack for
// UDP expires
func (t *Tracer) connectionExpired(conn *network.ConnectionStats, latestTime uint64, ctr *cachedConntrack) bool {
	timeout := t.timeoutForConn(conn)
	if !conn.IsExpired(latestTime, timeout) {
		return false
	}

	// skip connection check for udp connections or if
	// the pid for the connection is dead
	// conn.Pid can be 0 when ebpf-less tracer is running
	if conn.Type == network.UDP || (conn.Pid > 0 && !procutil.PidExists(int(conn.Pid))) {
		return true
	}

	exists, err := ctr.Exists(conn)
	if err != nil {
		log.Warnf("error checking conntrack for connection %s: %s", conn.String(), err)
	}
	if !exists {
		exists, err = ctr.ExistsInRootNS(conn)
		if err != nil {
			log.Warnf("error checking conntrack for connection in root ns %s: %s", conn.String(), err)
		}
	}

	return !exists
}

func (t *Tracer) connVia(cs *network.ConnectionStats) {
	if t.gwLookup == nil {
		return // gateway lookup is not enabled
	}

	cs.Via = t.gwLookup.Lookup(cs)
}

// DebugCachedConntrack dumps the cached NAT conntrack data
func (t *Tracer) DebugCachedConntrack(ctx context.Context) (*DebugConntrackTable, error) {
	ns, err := t.config.GetRootNetNs()
	if err != nil {
		return nil, err
	}
	defer ns.Close()

	rootNS, err := kernel.GetInoForNs(ns)
	if err != nil {
		return nil, err
	}
	table, err := t.conntracker.DumpCachedTable(ctx)
	if err != nil {
		return nil, err
	}

	return &DebugConntrackTable{
		Kind:    "cached-" + t.conntracker.GetType(),
		RootNS:  rootNS,
		Entries: table,
	}, nil
}

// DebugHostConntrack dumps the NAT conntrack data obtained from the host via netlink.
func (t *Tracer) DebugHostConntrack(ctx context.Context) (*DebugConntrackTable, error) {
	ns, err := t.config.GetRootNetNs()
	if err != nil {
		return nil, err
	}
	defer ns.Close()

	rootNS, err := kernel.GetInoForNs(ns)
	if err != nil {
		return nil, err
	}
	table, err := netlink.DumpHostTable(ctx, t.config, t.telemetryComp)
	if err != nil {
		return nil, err
	}

	// some clients have tens of thousands of connections and we need to stop early
	// if netlink takes too long. we indicate this behavior occured early with IsTruncated
	isTruncated := errors.Is(ctx.Err(), context.DeadlineExceeded)

	return &DebugConntrackTable{
		Kind:        "host-nat",
		RootNS:      rootNS,
		Entries:     table,
		IsTruncated: isTruncated,
	}, nil
}

// DebugDumpProcessCache dumps the process cache
func (t *Tracer) DebugDumpProcessCache(_ context.Context) (interface{}, error) {
	if t.processCache != nil {
		return t.processCache.Dump()
	}

	return nil, nil
}

func newUSMMonitor(c *config.Config, tracer connection.Tracer) *usm.Monitor {
	if !usmconfig.IsUSMSupportedAndEnabled(c) {
		// If USM is not supported, or if USM is not enabled, we should not start the USM monitor.
		return nil
	}

	// Shared map between NPM and USM
	connectionProtocolMap, err := tracer.GetMap(probes.ConnectionProtocolMap)
	if err != nil {
		log.Warnf("couldn't get %q map: %s", probes.ConnectionProtocolMap, err)
	}

	monitor, err := usm.NewMonitor(c, connectionProtocolMap)
	if err != nil {
		log.Errorf("usm initialization failed: %s", err)
		return nil
	}

	if err := monitor.Start(); err != nil {
		log.Errorf("usm startup failed: %s", err)
		return nil
	}

	return monitor
}

// GetNetworkID retrieves the vpc_id (network_id) from IMDS
func (t *Tracer) GetNetworkID(context context.Context) (string, error) {
	id := ""
	err := kernel.WithRootNS(kernel.ProcFSRoot(), func() error {
		var err error
		id, err = ec2.GetNetworkID(context)
		return err
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

const connProtoTTL = 3 * time.Minute
const connProtoCleaningInterval = 5 * time.Minute

// setupConnectionProtocolMapCleaner sets up a map cleaner for the connectionProtocolMap.
// It will run every connProtoCleaningInterval and delete entries older than connProtoTTL.
func setupConnectionProtocolMapCleaner(connectionProtocolMap *ebpf.Map) (*ddebpf.MapCleaner[netebpf.ConnTuple, netebpf.ProtocolStackWrapper], error) {
	mapCleaner, err := ddebpf.NewMapCleaner[netebpf.ConnTuple, netebpf.ProtocolStackWrapper](connectionProtocolMap, 1024)
	if err != nil {
		return nil, err
	}

	ttl := connProtoTTL.Nanoseconds()
	mapCleaner.Clean(connProtoCleaningInterval, nil, nil, func(now int64, _ netebpf.ConnTuple, val netebpf.ProtocolStackWrapper) bool {
		return (now - int64(val.Updated)) > ttl
	})

	return mapCleaner, nil
}
