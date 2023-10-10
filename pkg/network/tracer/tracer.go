// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/ebpf-manager/tracefs"
	"github.com/avast/retry-go/v4"
	"github.com/cihub/seelog"
	"github.com/cilium/ebpf"
	"go.uber.org/atomic"

	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/config/sysctl"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/events"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http2"
	nettelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection"
	"github.com/DataDog/datadog-agent/pkg/network/usm"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
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
	closedConns          *nettelemetry.StatCounterWrapper
	connStatsMapSize     telemetry.Gauge
	payloadSizePerClient telemetry.Gauge
}{
	telemetry.NewCounter(tracerModuleName, "skipped_conns", []string{"ip_proto"}, "Counter measuring skipped connections"),
	telemetry.NewCounter(tracerModuleName, "expired_tcp_conns", []string{}, "Counter measuring expired TCP connections"),
	nettelemetry.NewStatCounterWrapper(tracerModuleName, "closed_conns", []string{"ip_proto"}, "Counter measuring closed TCP connections"),
	telemetry.NewGauge(tracerModuleName, "conn_stats_map_size", []string{}, "Gauge measuring the size of the active connections map"),
	telemetry.NewGauge(tracerModuleName, "payload_conn_count", []string{"client_id", "ip_proto"}, "Gauge measuring the number of connections in the system-probe payload"),
}

// Tracer implements the functionality of the network tracer
type Tracer struct {
	config       *config.Config
	state        network.State
	conntracker  netlink.Conntracker
	reverseDNS   dns.ReverseDNS
	usmMonitor   *usm.Monitor
	ebpfTracer   connection.Tracer
	bpfTelemetry *nettelemetry.EBPFTelemetry
	lastCheck    *atomic.Int64

	bufferLock sync.Mutex

	// Connections for the tracer to exclude
	sourceExcludes []*network.ConnectionFilter
	destExcludes   []*network.ConnectionFilter

	gwLookup *gatewayLookup

	sysctlUDPConnTimeout       *sysctl.Int
	sysctlUDPConnStreamTimeout *sysctl.Int

	processCache *processCache

	timeResolver *TimeResolver
}

// NewTracer creates a Tracer
func NewTracer(config *config.Config) (*Tracer, error) {
	tr, err := newTracer(config)
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
func newTracer(cfg *config.Config) (_ *Tracer, reterr error) {
	if _, err := tracefs.Root(); err != nil {
		return nil, fmt.Errorf("system-probe unsupported: %s", err)
	}

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
		if !http.Supported() {
			errStr := fmt.Sprintf("Universal Service Monitoring (USM) requires a Linux kernel version of %s or higher. We detected %s", http.MinimumKernelVersion, currKernelVersion)
			if !cfg.NPMEnabled {
				return nil, fmt.Errorf(errStr)
			}
			log.Warnf("%s. NPM is explicitly enabled, so system-probe will continue with only NPM features enabled.", errStr)
			cfg.EnableHTTPMonitoring = false
			cfg.EnableHTTP2Monitoring = false
			cfg.EnableNativeTLSMonitoring = false
		}

		if !http2.Supported() {
			cfg.EnableHTTP2Monitoring = false
			log.Warnf("http2 requires a Linux kernel version of %s or higher. We detected %s", http2.MinimumKernelVersion, currKernelVersion)
		}
	}

	tr := &Tracer{
		config:                     cfg,
		lastCheck:                  atomic.NewInt64(time.Now().Unix()),
		sysctlUDPConnTimeout:       sysctl.NewInt(cfg.ProcRoot, "net/netfilter/nf_conntrack_udp_timeout", time.Minute),
		sysctlUDPConnStreamTimeout: sysctl.NewInt(cfg.ProcRoot, "net/netfilter/nf_conntrack_udp_timeout_stream", time.Minute),
	}
	defer func() {
		if reterr != nil {
			tr.Stop()
		}
	}()

	if nettelemetry.EBPFTelemetrySupported() {
		tr.bpfTelemetry = nettelemetry.NewEBPFTelemetry()
		coretelemetry.GetCompatComponent().RegisterCollector(tr.bpfTelemetry)
	}

	tr.ebpfTracer, err = connection.NewTracer(cfg, tr.bpfTelemetry)
	if err != nil {
		return nil, err
	}
	coretelemetry.GetCompatComponent().RegisterCollector(tr.ebpfTracer)

	tr.conntracker, err = newConntracker(cfg, tr.bpfTelemetry)
	if err != nil {
		return nil, err
	}
	coretelemetry.GetCompatComponent().RegisterCollector(tr.conntracker)

	tr.gwLookup = newGatewayLookup(cfg)
	if tr.gwLookup != nil {
		log.Info("gateway lookup enabled")
	}

	tr.reverseDNS = newReverseDNS(cfg)
	tr.usmMonitor = newUSMMonitor(cfg, tr.ebpfTracer, tr.bpfTelemetry)

	if cfg.EnableProcessEventMonitoring {
		if err = events.Init(); err != nil {
			return nil, fmt.Errorf("could not initialize event monitoring: %w", err)
		}

		if tr.processCache, err = newProcessCache(cfg.MaxProcessesTracked, defaultFilteredEnvs); err != nil {
			return nil, fmt.Errorf("could not create process cache; %w", err)
		}
		coretelemetry.GetCompatComponent().RegisterCollector(tr.processCache)
		events.RegisterHandler(tr.processCache)

		if tr.timeResolver, err = NewTimeResolver(); err != nil {
			return nil, fmt.Errorf("could not create time resolver: %w", err)
		}
	}

	tr.sourceExcludes = network.ParseConnectionFilters(cfg.ExcludedSourceConnections)
	tr.destExcludes = network.ParseConnectionFilters(cfg.ExcludedDestinationConnections)
	tr.state = network.NewState(
		cfg.ClientStateExpiry,
		cfg.MaxClosedConnectionsBuffered,
		cfg.MaxConnectionsStateBuffered,
		cfg.MaxDNSStatsBuffered,
		cfg.MaxHTTPStatsBuffered,
		cfg.MaxKafkaStatsBuffered,
	)

	return tr, nil
}

// start starts the tracer. This function is present to separate
// the creation from the start of the tracer for tests
func (tr *Tracer) start() error {
	err := tr.ebpfTracer.Start(tr.storeClosedConnections)
	if err != nil {
		tr.Stop()
		return fmt.Errorf("could not start ebpf manager: %s", err)
	}

	if err = tr.reverseDNS.Start(); err != nil {
		tr.Stop()
		return fmt.Errorf("could not start reverse dns monitor: %w", err)
	}

	return nil
}

func newConntracker(cfg *config.Config, bpfTelemetry *nettelemetry.EBPFTelemetry) (netlink.Conntracker, error) {
	if !cfg.EnableConntrack {
		return netlink.NewNoOpConntracker(), nil
	}

	var c netlink.Conntracker
	var err error

	// try creating ebpf conntracker 3 times in case the module is not loaded on the host yet
	err = retry.Do(
		func() error {
			c, err = NewEBPFConntracker(cfg, bpfTelemetry)
			return err
		},
		retry.Attempts(3),
		retry.Delay(1*time.Second),
	)
	if err == nil {
		return c, nil
	}

	if cfg.AllowNetlinkConntrackerFallback {
		log.Warnf("error initializing ebpf conntracker, falling back to netlink version: %s", err)
		if c, err = netlink.NewConntracker(cfg); err == nil {
			return c, nil
		}
	}

	if cfg.IgnoreConntrackInitFailure {
		log.Warnf("could not initialize conntrack, tracer will continue without NAT tracking: %s", err)
		return netlink.NewNoOpConntracker(), nil
	}

	return nil, fmt.Errorf("error initializing conntracker: %s. set network_config.ignore_conntrack_init_failure to true to ignore conntrack failures on startup", err)
}

func newReverseDNS(c *config.Config) dns.ReverseDNS {
	if !c.DNSInspection {
		return dns.NewNullReverseDNS()
	}

	rdns, err := dns.NewReverseDNS(c)
	if err != nil {
		log.Errorf("could not instantiate dns inspector: %s", err)
		return dns.NewNullReverseDNS()
	}

	log.Info("dns inspection enabled")
	return rdns
}

func (t *Tracer) storeClosedConnections(connections []network.ConnectionStats) {
	var rejected int
	_ = t.timeResolver.Sync()
	for i := range connections {
		cs := &connections[i]
		if t.shouldSkipConnection(cs) {
			connections[rejected], connections[i] = connections[i], connections[rejected]
			rejected++
			tracerTelemetry.skippedConns.Inc(cs.Type.String())
			continue
		}

		cs.IPTranslation = t.conntracker.GetTranslationForConn(*cs)
		t.connVia(cs)
		if cs.IPTranslation != nil {
			t.conntracker.DeleteTranslation(*cs)
		}

		t.addProcessInfo(cs)

		tracerTelemetry.closedConns.Inc(cs.Type.String())
	}

	connections = connections[rejected:]
	t.state.StoreClosedConnections(connections)
}

func (t *Tracer) addProcessInfo(c *network.ConnectionStats) {
	if t.processCache == nil {
		return
	}

	c.ContainerID = nil

	ts := t.timeResolver.ResolveMonotonicTimestamp(c.LastUpdateEpoch)
	p, ok := t.processCache.Get(c.Pid, int64(ts))
	if !ok {
		return
	}

	if log.ShouldLog(seelog.TraceLvl) {
		log.Tracef("got process cache entry for pid %d: %+v", c.Pid, p)
	}

	if c.Tags == nil {
		c.Tags = make(map[string]struct{}, 3)
	}

	addTag := func(k, v string) {
		if v == "" {
			return
		}
		c.Tags[k+":"+v] = struct{}{}
	}

	addTag("env", p.Env("DD_ENV"))
	addTag("version", p.Env("DD_VERSION"))
	addTag("service", p.Env("DD_SERVICE"))

	containerID := p.ContainerID.Get().(string)
	if containerID != "" {
		c.ContainerID = &containerID
	}
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
		coretelemetry.GetCompatComponent().UnregisterCollector(t.ebpfTracer)
	}
	if t.usmMonitor != nil {
		t.usmMonitor.Stop()
	}
	if t.conntracker != nil {
		t.conntracker.Close()
		coretelemetry.GetCompatComponent().UnregisterCollector(t.conntracker)
	}
	if t.processCache != nil {
		events.UnregisterHandler(t.processCache)
		t.processCache.Stop()
		coretelemetry.GetCompatComponent().UnregisterCollector(t.processCache)
	}
	if t.bpfTelemetry != nil {
		coretelemetry.GetCompatComponent().UnregisterCollector(t.bpfTelemetry)
	}
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
	conns.DNSStats = delta.DNSStats
	conns.HTTP = delta.HTTP
	conns.HTTP2 = delta.HTTP2
	conns.Kafka = delta.Kafka
	conns.ConnTelemetry = t.state.GetTelemetryDelta(clientID, t.getConnTelemetry(len(active)))
	conns.CompilationTelemetryByAsset = t.getRuntimeCompilationTelemetry()
	conns.KernelHeaderFetchResult = int32(kernel.HeaderProvider.GetResult())
	conns.CORETelemetryByAsset = ddebpf.GetCORETelemetryByAsset()
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
	kprobeStats := ddebpf.GetProbeTotals()
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
			tracerTelemetry.closedConns.Inc(c.Type.String())
			return false
		}

		if t.shouldSkipConnection(c) {
			tracerTelemetry.skippedConns.Inc(c.Type.String())
			return false
		}
		return true
	})
	if err != nil {
		return 0, nil, err
	}

	activeConnections = activeBuffer.Connections()
	_ = t.timeResolver.Sync()
	for i := range activeConnections {
		activeConnections[i].IPTranslation = t.conntracker.GetTranslationForConn(activeConnections[i])
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
		t.conntracker.DeleteTranslation(*entry)

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
	bpfMapStats
	bpfHelperStats
	kafkaStats
)

var allStats = []statsComp{
	stateStats,
	tracerStats,
	httpStats,
	bpfMapStats,
	bpfHelperStats,
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
		case bpfMapStats:
			ret[nettelemetry.EBPFMapTelemetryNS] = t.bpfTelemetry.GetMapsTelemetry()
		case bpfHelperStats:
			ret[nettelemetry.EBPFHelperTelemetryNS] = t.bpfTelemetry.GetHelpersTelemetry()
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
func (t *Tracer) DebugEBPFMaps(maps ...string) (string, error) {
	tracerMaps, err := t.ebpfTracer.DumpMaps(maps...)
	if err != nil {
		return "", err
	}
	if t.usmMonitor == nil {
		return "tracer:\n" + tracerMaps, nil
	}
	usmMaps, err := t.usmMonitor.DumpMaps(maps...)
	if err != nil {
		return "", err
	}
	return "tracer:\n" + tracerMaps + "\nhttp_monitor:\n" + usmMaps, nil
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
	if conn.Type == network.UDP || !procutil.PidExists(int(conn.Pid)) {
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
func (t *Tracer) DebugCachedConntrack(ctx context.Context) (interface{}, error) {
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

	return struct {
		RootNS  uint32
		Entries map[uint32][]netlink.DebugConntrackEntry
	}{
		RootNS:  rootNS,
		Entries: table,
	}, nil
}

// DebugHostConntrack dumps the NAT conntrack data obtained from the host via netlink.
func (t *Tracer) DebugHostConntrack(ctx context.Context) (interface{}, error) {
	ns, err := t.config.GetRootNetNs()
	if err != nil {
		return nil, err
	}
	defer ns.Close()

	rootNS, err := kernel.GetInoForNs(ns)
	if err != nil {
		return nil, err
	}
	table, err := netlink.DumpHostTable(ctx, t.config)
	if err != nil {
		return nil, err
	}

	return struct {
		RootNS  uint32
		Entries map[uint32][]netlink.DebugConntrackEntry
	}{
		RootNS:  rootNS,
		Entries: table,
	}, nil
}

// DebugDumpProcessCache dumps the process cache
func (t *Tracer) DebugDumpProcessCache(ctx context.Context) (interface{}, error) {
	if t.processCache != nil {
		return t.processCache.Dump()
	}

	return nil, nil
}

func newUSMMonitor(c *config.Config, tracer connection.Tracer, bpfTelemetry *nettelemetry.EBPFTelemetry) *usm.Monitor {
	// Shared with the USM program
	sockFDMap := tracer.GetMap(probes.SockByPidFDMap)
	connectionProtocolMap := tracer.GetMap(probes.ConnectionProtocolMap)

	monitor, err := usm.NewMonitor(c, connectionProtocolMap, sockFDMap, bpfTelemetry)
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
