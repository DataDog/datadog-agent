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
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/cilium/ebpf"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/atomic"

	model "github.com/DataDog/agent-payload/v5/process"

	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
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
	filter "github.com/DataDog/datadog-agent/pkg/network/tracer/networkfilter"
	"github.com/DataDog/datadog-agent/pkg/network/usm"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers"
	netnsutil "github.com/DataDog/datadog-agent/pkg/util/kernel/netns"
	"github.com/DataDog/datadog-agent/pkg/util/ktime"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	statusEndpoint = "/status"
	debugEndpoint  = "/debug"
	connsEndpoint  = "/connections"
	registerURL    = "/register"

	// maxResponseBytes represents the maximum number of bytes we're willing to read
	// from the connections endpoint response
	maxResponseBytes = 10 * 1024 * 1024

	// probeRetryInterval determines the frequency at which we'll attempt to re-initialize
	// the probes that have previously failed to load
	probeRetryInterval = 5 * time.Minute

	// headerTruncationSize is the size that we truncate agent headers to
	headerTruncationSize = 128

	defaultUDPConnTimeoutNanoSeconds = uint64(time.Duration(120) * time.Second)
	tracerModuleName                 = "network_tracer"
)

//nolint:revive // TODO(NET) Fix revive linter
var (
	// ErrTracerNotRunning signals that the tracer is not running
	ErrTracerNotRunning = errors.New("tracer not running")

	// ErrEBPFUnsupported is returned when eBPF is not supported on the host
	ErrEBPFUnsupported = errors.New("eBPF not supported")

	// ErrBPFUtilsLoad is returned when /opt/datadog-agent/embedded/bin/bpf-utils fails to load
	ErrBPFUtilsLoad = errors.New("bpf-utils failed to load")

	// the value of the DD_AGENT_MAJOR_VERSION environment variable
	// Note: This has to be a string because we use this value for logf arguments
	agentMajorVersion = fmt.Sprintf("%d", version.AgentVersion.Major)

	baseURL = ""
)

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
	sourceExcludes []*filter.ConnectionFilter
	destExcludes   []*filter.ConnectionFilter

	gwLookup network.GatewayLookup

	sysctlUDPConnTimeout       *sysctl.Int
	sysctlUDPConnStreamTimeout *sysctl.Int

	processCache *processCache

	timeResolver *ktime.Resolver

	telemetryComp telemetryComponent.Component

	// Used for connection_protocol data expiration
	connectionProtocolMapCleaner *ddebpf.MapCleaner[netebpf.ConnTuple, netebpf.ProtocolStackWrapper]

	connsLock         sync.RWMutex
	connMonitor       *network.Monitor
	compilationResult network.CompilationResultTracer

	probeRetrier *periodicRetrier
	stopChan     chan struct{}
	wg           sync.WaitGroup

	// ebpf checks telemetry
	ebpfChecksCollector *ebpfcheck.Collector

	// Network driver related flags
	missingDriver atomic.Bool

	// DNS capture metrics
	domainLookups *prometheus.CounterVec
	resolvedDNS   *prometheus.CounterVec
}

// NewTracer creates a Tracer
func NewTracer(config *config.Config, telemetryComponent telemetryComponent.Component, statsd statsd.ClientInterface) (*Tracer, error) {
	tr, err := newTracer(config, telemetryComponent, statsd)
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
func newTracer(cfg *config.Config, telemetryComponent telemetryComponent.Component, statsd statsd.ClientInterface) (_ *Tracer, reterr error) {
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
	tr.usmMonitor = newUSMMonitor(cfg, tr.ebpfTracer, statsd)

	// Set up the connection_protocol map cleaner if protocol classification is enabled
	if cfg.ProtocolClassificationEnabled || usmconfig.IsUSMSupportedAndEnabled(cfg) {
		connectionProtocolMap, err := tr.ebpfTracer.GetMap(probes.ConnectionProtocolMap)
		if err == nil {
			tr.connectionProtocolMapCleaner, err = setupConnectionProtocolMapCleaner(connectionProtocolMap, probes.ConnectionProtocolMap)
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

	tr.sourceExcludes = filter.ParseConnectionFilters(cfg.ExcludedSourceConnections)
	tr.destExcludes = filter.ParseConnectionFilters(cfg.ExcludedDestinationConnections)
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

func loadEbpfConntracker(cfg *config.Config, telemetryComponent telemetryComponent.Component) (netlink.Conntracker, error) {
	if !cfg.EnableEbpfConntracker {
		log.Info("ebpf conntracker disabled")
		return nil, nil
	}

	if err := netlink.LoadNfConntrackKernelModule(cfg); err != nil {
		log.Warnf("failed to load conntrack kernel module, though it may already be loaded: %s", err)
	}

	return NewEBPFConntracker(cfg, telemetryComponent)
}

func newConntracker(cfg *config.Config, telemetryComponent telemetryComponent.Component) (netlink.Conntracker, error) {
	if !cfg.EnableConntrack {
		return netlink.NewNoOpConntracker(), nil
	}

	var clb netlink.Conntracker
	var c netlink.Conntracker
	var err error
	if !cfg.EnableEbpfless {
		if c, err = loadEbpfConntracker(cfg, telemetryComponent); err != nil {
			log.Warnf("error initializing ebpf conntracker: %s", err)
			log.Info("falling back to netlink conntracker")
		}

		if clb, err = newCiliumLoadBalancerConntracker(cfg); err != nil {
			log.Warnf("cilium lb conntracker is enabled, but failed to load: %s", err)
		}
	}

	if c == nil {
		if c, err = netlink.NewConntracker(cfg, telemetryComponent); err != nil {
			if errors.Is(err, netlink.ErrNotPermitted) || cfg.IgnoreConntrackInitFailure {
				log.Warnf("could not initialize netlink conntracker: %s", err)
			} else {
				return nil, fmt.Errorf("error initializing conntracker: %s. set network_config.ignore_conntrack_init_failure to true to ignore conntrack failures on startup", err)
			}
		}
	}

	c = chainConntrackers(c, clb)
	if c.GetType() == "" {
		// no-op conntracker
		log.Warnf("connection tracking is disabled")
	}

	return c, nil
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

	if log.ShouldLog(log.TraceLvl) {
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
	if t.gwLookup != nil {
		t.gwLookup.Close()
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
func (t *Tracer) GetActiveConnections(clientID string) (*network.Connections, func(), error) {
	t.bufferLock.Lock()
	defer t.bufferLock.Unlock()
	if log.ShouldLog(log.TraceLvl) {
		log.Tracef("GetActiveConnections clientID=%s", clientID)
	}
	t.ebpfTracer.FlushPending()

	buffer := network.ClientPool.Get(clientID)
	latestTime, active, err := t.getConnections(buffer.ConnectionBuffer)
	if err != nil {
		return nil, nil, fmt.Errorf("error retrieving connections: %s", err)
	}

	usmStats, cleanup := t.usmMonitor.GetProtocolStats()
	delta := t.state.GetDelta(clientID, latestTime, active, t.reverseDNS.GetDNSStats(), usmStats)

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
	conns.KernelHeaderFetchResult = int32(headers.HeaderProvider.GetResult())
	conns.CORETelemetryByAsset = ddebpf.GetCORETelemetryByAsset()
	conns.PrebuiltAssets = netebpf.GetModulesInUse()
	t.lastCheck.Store(time.Now().Unix())

	return conns, cleanup, nil
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

func (t *Tracer) getCachedConntrack() *cachedConntrack {
	newConntrack := netlink.NewConntrack
	// if we already established that netlink conntracker is not supported, don't try again
	if t.conntracker.GetType() == "" {
		newConntrack = netlink.NewNoOpConntrack
	}
	return newCachedConntrack(t.config.ProcRoot, newConntrack, 128)
}

// getConnections returns all the active connections in the ebpf maps along with the latest timestamp.  It takes
// a reusable buffer for appending the active connections so that this doesn't continuously allocate
func (t *Tracer) getConnections(activeBuffer *network.ConnectionBuffer) (latestUint uint64, activeConnections []network.ConnectionStats, err error) {
	cachedConntrack := t.getCachedConntrack()
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

	if log.ShouldLog(log.DebugLvl) {
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

	rootNS, err := netnsutil.GetInoForNs(ns)
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

	rootNS, err := netnsutil.GetInoForNs(ns)
	if err != nil {
		return nil, err
	}
	table, err := netlink.DumpHostTable(ctx, t.config, t.telemetryComp)
	if err != nil {
		return nil, err
	}

	// some clients have tens of thousands of connections and we need to stop early
	// if netlink takes too long. we indicate this behavior occurred early with IsTruncated
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

func newUSMMonitor(c *config.Config, tracer connection.Tracer, statsd statsd.ClientInterface) *usm.Monitor {
	if !usmconfig.IsUSMSupportedAndEnabled(c) {
		// If USM is not supported, or if USM is not enabled, we should not start the USM monitor.
		return nil
	}

	// Shared map between NPM and USM
	connectionProtocolMap, err := tracer.GetMap(probes.ConnectionProtocolMap)
	if err != nil {
		log.Warnf("couldn't get %q map: %s", probes.ConnectionProtocolMap, err)
	}

	monitor, err := usm.NewMonitor(c, connectionProtocolMap, statsd)
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
	err := netnsutil.WithRootNS(kernel.ProcFSRoot(), func() error {
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
const connProtoCleaningInterval = 65 * time.Second // slight jitter to avoid all maps cleaning at the same time

// setupConnectionProtocolMapCleaner sets up a map cleaner for the connectionProtocolMap.
// It will run every connProtoCleaningInterval and delete entries older than connProtoTTL.
func setupConnectionProtocolMapCleaner(connectionProtocolMap *ebpf.Map, name string) (*ddebpf.MapCleaner[netebpf.ConnTuple, netebpf.ProtocolStackWrapper], error) {
	mapCleaner, err := ddebpf.NewMapCleaner[netebpf.ConnTuple, netebpf.ProtocolStackWrapper](connectionProtocolMap, 1, name, "npm_tracer")
	if err != nil {
		return nil, err
	}

	ttl := connProtoTTL.Nanoseconds()
	mapCleaner.Clean(connProtoCleaningInterval, nil, nil, func(now int64, _ netebpf.ConnTuple, val netebpf.ProtocolStackWrapper) bool {
		return (now - int64(val.Updated)) > ttl
	})

	return mapCleaner, nil
}

// Helper function (placeholder - needs implementation based on actual struct fields)
// This function converts the internal ConnectionStats struct to the model.Connection struct used for payloads.
func ToModelConnection(cs *network.ConnectionStats) *model.Connection {
	// This requires mapping all relevant fields. Example mapping:
	return &model.Connection{
		Pid:                     cs.Pid,
		PidCreateTime:           cs.PidCreateTime,
		NetNS:                   cs.NetNS,
		Family:                  model.ConnectionFamily(cs.Family),
		Type:                    model.ConnectionType(cs.Type),
		Laddr:                   &model.Addr{Ip: cs.Source.String(), Port: int32(cs.SPort)},
		Raddr:                   &model.Addr{Ip: cs.Dest.String(), Port: int32(cs.DPort)},
		Direction:               model.ConnectionDirection(cs.Direction),
		IntraHost:               cs.IntraHost,
		DnsSuccessfulResponses:  cs.DNSStatsSuccessfulResponses,
		DnsFailedResponses:      cs.DNSStatsFailedResponses,
		DnsTimeouts:             cs.DNSStatsTimeouts,
		DnsSuccessLatencySum:    cs.DNSStatsSuccessLatencySum,
		DnsFailureLatencySum:    cs.DNSStatsFailureLatencySum,
		DnsCountByRcode:         cs.DNSStatsCountByRcode,
		LastUpdateEpoch:         cs.LastUpdateEpoch,
		IsAssured:               cs.IsAssured,
		MonotonicSentBytes:      cs.Monotonic.SentBytes,
		MonotonicRecvBytes:      cs.Monotonic.RecvBytes,
		MonotonicSentPackets:    cs.Monotonic.SentPackets,
		MonotonicRecvPackets:    cs.Monotonic.RecvPackets,
		MonotonicRetransmits:    uint32(cs.Monotonic.Retransmits),
		MonotonicTcpEstablished: uint32(cs.Monotonic.TCPEstablished),
		MonotonicTcpClosed:      uint32(cs.Monotonic.TCPClosed),
		LastSentBytes:           cs.Last.SentBytes,
		LastRecvBytes:           cs.Last.RecvBytes,
		LastSentPackets:         cs.Last.SentPackets,
		LastRecvPackets:         cs.Last.RecvPackets,
		LastRetransmits:         uint32(cs.Last.Retransmits),
		LastTcpEstablished:      uint32(cs.Last.TCPEstablished),
		LastTcpClosed:           uint32(cs.Last.TCPClosed),
		RouteIdx:                -1, // RouteIdx is handled later during batching
		NatRootNetns:            cs.NatRootNetNS,
		Rtt:                     cs.RTT,
		RttVar:                  cs.RTTVar,
		IpTranslation:           toModelIPTranslation(cs.IPTranslation),
		// ProtocolStack, ContainerID, Tags, DNSStatsByDomain/Offset, TagsIdx are handled later or derived.
	}
}

func toModelIPTranslation(ipt *network.IPTranslation) *model.IPTranslation {
	if ipt == nil {
		return nil
	}
	return &model.IPTranslation{
		ReplSrcIP:   ipt.ReplSrcIP.String(),
		ReplDstIP:   ipt.ReplDstIP.String(),
		ReplSrcPort: int32(ipt.ReplSrcPort),
		ReplDstPort: int32(ipt.ReplDstPort),
	}
}

// GetConnections returns the list of connections collected by the tracer for the given clientID.
func (t *Tracer) GetConnections(clientID string) (*Connections, error) {
	if clientID == network.DEBUGCLIENT {
		return t.getConnectionsSynchronous(clientID)
	}

	t.connsLock.Lock()
	defer t.connsLock.Unlock()

	if t.state == nil {
		return nil, ErrTracerNotRunning
	}

	// Fetch latest *active* connections from eBPF maps etc.
	latestActiveConnsResult := t.connMonitor.FlushConnections() // Assuming this returns active connections + metadata

	// Get regular delta. state.GetDelta now resets the near capacity flag for this client.
	delta := t.state.GetDelta(clientID, latestActiveConnsResult.LastUpdateEpoch, latestActiveConnsResult.Conns, t.getDNSStatsWithLock(), t.getProtocolStatsWithLock())

	// Start building the payload
	payload := &Connections{
		BufferedConns: t.getBufferredConnectionsWithLock(),
		// Adjust slice capacity - emergencyClosedConns is removed
		Conns:                       make([]*model.Connection, 0, len(delta.Conns)),
		DNS:                         t.getDNSObjectWithLock(),
		HTTP:                        delta.HTTP,
		HTTP2:                       delta.HTTP2,
		Kafka:                       delta.Kafka,
		Postgres:                    delta.Postgres,
		Redis:                       delta.Redis,
		ConnTelemetryMap:            t.getConnTelemetryWithLock(),
		CompilationTelemetryByAsset: t.getCompilationTelemetryWithLock(),
		KernelHeaderFetchResult:     t.getKernelHeaderFetchResultWithLock(),
		CORETelemetryByAsset:        t.getCORETelemetryWithLock(),
		PrebuiltEBPFAssets:          t.getPrebuiltAssetsWithLock(),
	}

	// Convert ConnectionStats from the delta (active + normally closed) to model.Connection
	for i := range delta.Conns {
		if !delta.Conns[i].IsEmpty() {
			payload.Conns = append(payload.Conns, ToModelConnection(&delta.Conns[i]))
		}
	}

	return payload, nil
}

// Add new checkCapacityHandler
func (t *Tracer) checkCapacityHandler(w http.ResponseWriter, r *http.Request) {
	if t.state == nil {
		log.Error("checkCapacityHandler called before tracer state is initialized")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// No client ID needed, checks global flag
	if t.state.IsClosedConnectionsNearCapacity() {
		log.Debug("Responding to capacity check: Near capacity (200 OK)")
		w.WriteHeader(http.StatusOK) // 200 OK indicates near capacity
	} else {
		log.Trace("Responding to capacity check: Not near capacity (204 No Content)")
		w.WriteHeader(http.StatusNoContent) // 204 No Content indicates not near capacity
	}
}

// Register add underlying http handlers to the provided serve mux.
func (t *Tracer) Register(httpMux *http.ServeMux) error {
	if !t.config.EnableSystemProbeBuiltinEndpoints {
		log.Debug("Network tracer endpoints disabled")
		return nil
	}

	// Existing handlers
	httpMux.HandleFunc(baseURL+connsEndpoint, otelhttp.WrapHandler(http.HandlerFunc(t.connectionsHandler), "connections_handler"))
	httpMux.HandleFunc(baseURL+registerURL, otelhttp.WrapHandler(http.HandlerFunc(t.registerHandler), "register_handler"))
	httpMux.HandleFunc(baseURL+statusEndpoint, otelhttp.WrapHandler(http.HandlerFunc(t.statusHandler), "status_handler"))
	httpMux.HandleFunc(baseURL+debugEndpoint, otelhttp.WrapHandler(http.HandlerFunc(t.debugHandler), "debug_handler"))

	// Register *new* endpoint for capacity check
	newEndpointPath := baseURL + "/connections/check_capacity" // Changed path
	log.Infof("Registering network tracer endpoint: %s", newEndpointPath)
	httpMux.HandleFunc(newEndpointPath, otelhttp.WrapHandler(http.HandlerFunc(t.checkCapacityHandler), "check_capacity")) // Use new handler

	return nil
}

func (t *Tracer) GetTelemetry() map[network.ConnTelemetryType]int64 {
	t.connsLock.RLock()
	defer t.connsLock.RUnlock()
	return t.getConnTelemetryWithLock()
}

// Connections returns the batch of connections ready to be sent to the process-agent
func (t *Tracer) getConnectionsSynchronous(clientID string) (*Connections, error) {
	t.connsLock.Lock()
	defer t.connsLock.Unlock()

	if t.state == nil {
		return nil, ErrTracerNotRunning
	}

	now := uint64(time.Now().UnixNano())

	active, closed, telemetryDelta := t.fetchAndProcessConnections(clientID, now)

	// Construct the Connections payload immediately
	payload := &Connections{
		BufferedConns:               t.getBufferredConnectionsWithLock(),
		Conns:                       make([]*model.Connection, 0, len(active)+len(closed)),
		DNS:                         t.getDNSObjectWithLock(),
		HTTP:                        t.protocolMonitor.GetHTTPStats(),
		HTTP2:                       t.protocolMonitor.GetHTTP2Stats(),
		Kafka:                       t.protocolMonitor.GetKafkaStats(),
		Postgres:                    t.protocolMonitor.GetPostgresStats(),
		Redis:                       t.protocolMonitor.GetRedisStats(),
		ConnTelemetryMap:            telemetryDelta,
		CompilationTelemetryByAsset: t.getCompilationTelemetryWithLock(),
		KernelHeaderFetchResult:     t.getKernelHeaderFetchResultWithLock(),
		CORETelemetryByAsset:        t.getCORETelemetryWithLock(),
		PrebuiltEBPFAssets:          t.getPrebuiltAssetsWithLock(),
	}

	for i := range active {
		payload.Conns = append(payload.Conns, ToModelConnection(&active[i]))
	}
	for i := range closed {
		payload.Conns = append(payload.Conns, ToModelConnection(&closed[i]))
	}

	return payload, nil
}

func (t *Tracer) fetchAndProcessConnections(clientID string, latestTime uint64) (active, closed []network.ConnectionStats, telemetry map[network.ConnTelemetryType]int64) {
	conns := t.connMonitor.FlushConnections()

	// Add active connections to the state
	active, closed = t.state.StoreConnections(clientID, latestTime, conns)

	// resolve DNS
	t.reverseDNS.Resolve(closed)
	t.reverseDNS.Resolve(active)

	telemetry = t.state.GetTelemetryDelta(clientID, t.GetTelemetry())

	return active, closed, telemetry
}

// run starts the main tracer loop
func (t *Tracer) run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("network tracer loop exited abnormally: %s", r)
		}
	}()

	ticker := network.NewTimeTicker(t.config.AggregatorFlushInterval)
	defer ticker.Stop()

	probeRetryTicker := t.probeRetrier.GetTicker()
	defer t.probeRetrier.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.stopChan:
			return
		case <-ticker.C:
			// this is expected to be low frequency (every 30s by default)
			t.flushConnections()
			t.bufferedData.Tick()
			t.state.RemoveExpiredClients(time.Now())
		case <-probeRetryTicker:
			// this is expected to be very low frequency (every 5m by default)
			t.retryFailedProbes()
		}
	}
}

// start begins the tracer by initializing the BPF maps, starting the perf readers, and launching the periodic flush timer.
// Must be called before GetConnections.
func (t *Tracer) start(ctx context.Context) (err error) {
	t.connsLock.Lock()
	defer t.connsLock.Unlock()

	if t.state == nil {
		return ErrTracerNotRunning
	}

	tracers := map[network.TracerType]struct{}{}

	// Conntracker
	conntracker, err := network.NewConntracker(t.config)
	if err != nil {
		return fmt.Errorf("could not initialize conntracker: %w", err)
	}
	t.conntracker = conntracker

	tracers[network.ConntrackTracer] = struct{}{}

	if t.config.EnableRuntimeCompiler {
		log.Info("runtime compilation is enabled")
	}
	if t.config.AllowPrecompiledFallback {
		log.Info("precompiled fallback is allowed")
	}
	if t.config.AllowCOREFallback {
		log.Info("CORE fallback is allowed")
	}

	buf, compilationResult, err := network.LoadTracer(t.config)
	t.compilationResult = compilationResult
	if err != nil {
		if errors.Is(err, ebpf.ErrNotSupported) {
			return ErrEBPFUnsupported
		}

		// For the offset guesser
		if errors.Is(err, ebpf.ErrCouldNotAutoload) {
			// We need to ensure the driver files exist
			// Note that the offset guesser is *not* compatible with the driver workflow
			if !driver.PlatformSupportsDefaultDriver() {
				return fmt.Errorf("could not load module: %w. Kernel headers are likely missing. See our documentation for instructions on how to install them.", err)
			}
			// check if the driver is loaded
			driverInterface, err := driver.NewInterface()
			if err != nil {
				return fmt.Errorf("unable to load driver interface: %w", err)
			}

			if !driverInterface.IsLoaded() {
				return fmt.Errorf("unable to load module: %w. Ensure the datadog-network-driver has been loaded", err)
			}

			return fmt.Errorf("unable to load module: %w. Offset guessing failed.", err)
		}

		// If we get a compilation error, we should try to load the driver
		if errors.Is(err, ebpf.ErrCompilation) && driver.PlatformSupportsDefaultDriver() {
			driverInterface, err := driver.NewInterface()
			if err != nil {
				return fmt.Errorf("unable to load driver interface: %w", err)
			}

			if !driverInterface.IsLoaded() {
				return fmt.Errorf("unable to load module: %w. Ensure the datadog-network-driver has been loaded", err)
			}
		}

		// In any case, return the error
		return fmt.Errorf("unable to load module: %w", err)
	}

	defer func() {
		if err != nil {
			_ = buf.Close()
		}
	}()

	tracers[network.TracerType(t.compilationResult.TracerType)] = struct{}{}

	var missingProbes []probes.ProbeFuncName
	connMonitor, err := network.NewMonitor(t.config, buf.CollectionSpec(), telemetryComponent)
	if err != nil {
		var me *network.ErrMissingProbes
		if errors.As(err, &me) {
			missingProbes = me.Probes
		} else {
			return fmt.Errorf("error initializing network monitor: %w", err)
		}
	}

	// Add the compilation result to the state for debugging
	t.bpfTelemetry.CompilationResult = compilationResult

	// Start the monitor
	if err := connMonitor.Start(); err != nil {
		return fmt.Errorf("error starting network monitor: %w", err)
	}
	connMonitor.MapCleaner.Register(t.state)
	t.connMonitor = connMonitor
	t.mapCleaner = connMonitor.MapCleaner

	// Initialize the protocol monitors
	protocolMonitor, err := usm.NewMonitor(t.config, t.connMonitor.ConnResolver, t.bpfTelemetry, buf)
	if err != nil {
		var me *network.ErrMissingProbes
		if errors.As(err, &me) {
			missingProbes = append(missingProbes, me.Probes...)
		} else {
			return fmt.Errorf("error initializing protocol monitor: %w", err)
		}
	}
	t.protocolMonitor = protocolMonitor

	if len(missingProbes) > 0 {
		log.Warnf("network tracer failed to initialize the following probes: %s", strings.Join(probesList(missingProbes), ", "))
		t.probeRetrier.Start(missingProbes)
	}

	t.reverseDNS.Start(ctx)
	go t.run(ctx)

	// initialize the ebpf checks collector
	t.ebpfChecksCollector = ebpfcheck.NewCollector(t.config, telemetryComponent)
	go t.ebpfChecksCollector.Run(ctx)

	// update platform specific metadata
	installinfo.AddModule("npm", t.compilationResult.FullTracerType())

	var tracerTypeList []string
	for tracerType := range tracers {
		tracerTypeList = append(tracerTypeList, string(tracerType))
	}
	installinfo.AddModule("tracers", strings.Join(tracerTypeList, ","))

	log.Infof("network tracer initialized")
	return nil
}

// flushConnections calls the connection monitor to flush the connections
// and adds the connections to the state.
func (t *Tracer) flushConnections() {
	t.connsLock.Lock()
	defer t.connsLock.Unlock()

	conns := t.connMonitor.FlushConnections()

	// Add active connections to the state
	active, closed := t.state.StoreConnections(t.bufferedData.GetClients(), uint64(time.Now().UnixNano()), conns)

	// resolve DNS
	t.reverseDNS.Resolve(closed)
	t.reverseDNS.Resolve(active)

	// TODO: Add telemetry

	// Flush the connections to the buffered data
	t.bufferedData.AddConnection(active)
	t.bufferedData.AddConnection(closed)
	t.bufferedData.AddProtocols(t.protocolMonitor.GetHTTPStats())
	t.bufferedData.AddProtocols(t.protocolMonitor.GetHTTP2Stats())
	t.bufferedData.AddProtocols(t.protocolMonitor.GetKafkaStats())
	t.bufferedData.AddProtocols(t.protocolMonitor.GetPostgresStats())
	t.bufferedData.AddProtocols(t.protocolMonitor.GetRedisStats())
}

func (t *Tracer) retryFailedProbes() {
	t.connsLock.Lock()
	defer t.connsLock.Unlock()

	if t.state == nil {
		return
	}

	failedProbes := t.probeRetrier.GetFailedProbes()
	if len(failedProbes) == 0 {
		return
	}

	log.Infof("attempting to re-initialize failed probes: %s", strings.Join(probesList(failedProbes), ", "))

	buf, _, err := network.LoadTracer(t.config)
	if err != nil {
		log.Warnf("failed to load BPF module: %s", err)
		return
	}
	defer buf.Close()

	var newlyEnabledProbes []probes.ProbeFuncName
	programs := buf.CollectionSpec().Programs

	newlyEnabledProbes = append(newlyEnabledProbes, t.connMonitor.RetryProbes(programs)...)
	newlyEnabledProbes = append(newlyEnabledProbes, t.protocolMonitor.RetryProbes(programs)...)

	if len(newlyEnabledProbes) > 0 {
		log.Infof("successfully re-initialized the following probes: %s", strings.Join(probesList(newlyEnabledProbes), ", "))
		t.probeRetrier.SuccessfulRetry(newlyEnabledProbes)
	}
}

func (t *Tracer) getConnectionsWithLock(clientID string) *network.Connections {
	// Process buffered data for the given clientID
	conns := t.bufferedData.GetConnections(clientID)
	conns.DNS = t.getDNSObjectWithLock()
	conns.HTTP = t.bufferedData.GetProtocolStats(clientID, protocols.HTTP).(map[http.Key]*http.RequestStats)
	conns.HTTP2 = t.bufferedData.GetProtocolStats(clientID, protocols.HTTP2).(map[http.Key]*http.RequestStats)
	conns.Kafka = t.bufferedData.GetProtocolStats(clientID, protocols.Kafka).(map[kafka.Key]*kafka.RequestStats)
	conns.Postgres = t.bufferedData.GetProtocolStats(clientID, protocols.Postgres).(map[postgres.Key]*postgres.RequestStat)
	conns.Redis = t.bufferedData.GetProtocolStats(clientID, protocols.Redis).(map[redis.Key]*redis.RequestStat)
	conns.ConnTelemetryMap = t.getConnTelemetryWithLock()
	conns.CompilationTelemetryByAsset = t.getCompilationTelemetryWithLock()
	conns.KernelHeaderFetchResult = t.getKernelHeaderFetchResultWithLock()
	conns.CORETelemetryByAsset = t.getCORETelemetryWithLock()
	conns.PrebuiltEBPFAssets = t.getPrebuiltAssetsWithLock()
	return conns
}

func (t *Tracer) getDNSObjectWithLock() map[string]*model.DNSEntry {
	// Fetch reverse DNS entries
	dnsEntries := t.reverseDNS.GetDNS()
	// Convert to payload format
	payloadDNS := make(map[string]*model.DNSEntry, len(dnsEntries))
	for addr, entry := range dnsEntries {
		payloadDNS[addr.String()] = &model.DNSEntry{Names: entry}
	}
	return payloadDNS
}

func (t *Tracer) getDNSStatsWithLock() map[network.DNSKey]map[network.DNSHostname]map[network.DNSType]network.DNSStats {
	return t.reverseDNS.GetStats()
}

func (t *Tracer) getProtocolStatsWithLock() map[protocols.ProtocolType]interface{} {
	stats := make(map[protocols.ProtocolType]interface{})
	stats[protocols.HTTP] = t.protocolMonitor.GetHTTPStats()
	stats[protocols.HTTP2] = t.protocolMonitor.GetHTTP2Stats()
	stats[protocols.Kafka] = t.protocolMonitor.GetKafkaStats()
	stats[protocols.Postgres] = t.protocolMonitor.GetPostgresStats()
	stats[protocols.Redis] = t.protocolMonitor.GetRedisStats()
	return stats
}

func (t *Tracer) getConnTelemetryWithLock() map[string]int64 {
	// Combine state telemetry with BPF telemetry
	telemetry := t.state.GetTelemetryDelta(network.ProcessAgentClientID, t.GetTelemetry())
	connTelemetryMap := make(map[string]int64, len(telemetry))
	for k, v := range telemetry {
		connTelemetryMap[string(k)] = v
	}
	return connTelemetryMap
}

func (t *Tracer) getCompilationTelemetryWithLock() map[string]*model.RuntimeCompilationTelemetry {
	return t.bpfTelemetry.GetCompilationTelemetry()
}

func (t *Tracer) getKernelHeaderFetchResultWithLock() model.KernelHeaderFetchResult {
	return t.bpfTelemetry.GetKernelHeaderFetchResult()
}

func (t *Tracer) getCORETelemetryWithLock() map[string]model.COREResult {
	return t.bpfTelemetry.GetCORETelemetry()
}

func (t *Tracer) getPrebuiltAssetsWithLock() []string {
	return t.bpfTelemetry.GetPrebuiltEBPFAssets()
}

func (t *Tracer) getBufferredConnectionsWithLock() network.BufferedTicker {
	return t.bufferedData.GetDeepCopy()
}

func probesList(probes []probes.ProbeFuncName) []string {
	list := make([]string, 0, len(probes))
	for _, p := range probes {
		list = append(list, string(p))
	}
	return list
}
