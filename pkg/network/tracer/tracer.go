// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package tracer

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/config/sysctl"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/atomicstats"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultUDPConnTimeoutNanoSeconds = uint64(time.Duration(120) * time.Second)

// Tracer implements the functionality of the network tracer
type Tracer struct {
	config       *config.Config
	state        network.State
	conntracker  netlink.Conntracker
	reverseDNS   dns.ReverseDNS
	httpMonitor  *http.Monitor
	ebpfTracer   connection.Tracer
	bpfTelemetry *telemetry.EBPFTelemetry

	// Telemetry
	skippedConns *atomic.Int64 `stats:""`
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
	expiredTCPConns  *atomic.Int64 `stats:""`
	closedConns      *atomic.Int64 `stats:""`
	connStatsMapSize *atomic.Int64 `stats:""`
	lastCheck        *atomic.Int64 `stats:""`

	activeBuffer *network.ConnectionBuffer
	bufferLock   sync.Mutex

	// Connections for the tracer to exclude
	sourceExcludes []*network.ConnectionFilter
	destExcludes   []*network.ConnectionFilter

	gwLookup *gatewayLookup

	sysctlUDPConnTimeout       *sysctl.Int
	sysctlUDPConnStreamTimeout *sysctl.Int
}

// NewTracer creates a Tracer
func NewTracer(config *config.Config) (*Tracer, error) {
	// make sure debugfs is mounted
	if mounted, err := kernel.IsDebugFSMounted(); !mounted {
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

	usmSupported := currKernelVersion >= http.MinimumKernelVersion
	if !usmSupported && config.ServiceMonitoringEnabled {
		errStr := fmt.Sprintf("Universal Service Monitoring (USM) requires a Linux kernel version of %s or higher. We detected %s", http.MinimumKernelVersion, currKernelVersion)
		if !config.NPMEnabled {
			return nil, fmt.Errorf(errStr)
		}
		log.Warnf("%s. NPM is explicitly enabled, so system-probe will continue with only NPM features enabled.", errStr)
		config.EnableHTTPMonitoring = false
		config.EnableHTTPSMonitoring = false
	}

	offsetBuf, err := netebpf.ReadOffsetBPFModule(config.BPFDir, config.BPFDebug)
	if err != nil {
		return nil, fmt.Errorf("could not read offset bpf module: %s", err)
	}
	defer offsetBuf.Close()

	// Offset guessing has been flaky for some customers, so if it fails we'll retry it up to 5 times
	needsOffsets := !config.EnableRuntimeCompiler || config.AllowPrecompiledFallback
	var constantEditors []manager.ConstantEditor
	if needsOffsets {
		for i := 0; i < 5; i++ {
			constantEditors, err = runOffsetGuessing(config, offsetBuf)
			if err == nil {
				break
			}
			time.Sleep(1 * time.Second)
		}
		if err != nil {
			return nil, fmt.Errorf("error guessing offsets: %s", err)
		}
	}

	var bpfTelemetry *telemetry.EBPFTelemetry
	if usmSupported {
		bpfTelemetry = telemetry.NewEBPFTelemetry()
	}
	ebpfTracer, err := kprobe.New(config, constantEditors, bpfTelemetry)
	if err != nil {
		return nil, err
	}

	conntracker, err := newConntracker(config, bpfTelemetry)
	if err != nil {
		return nil, err
	}

	state := network.NewState(
		config.ClientStateExpiry,
		config.MaxClosedConnectionsBuffered,
		config.MaxConnectionsStateBuffered,
		config.MaxDNSStatsBuffered,
		config.MaxHTTPStatsBuffered,
	)

	gwLookup := newGatewayLookup(config)
	if gwLookup != nil {
		log.Info("gateway lookup enabled")
	}

	tr := &Tracer{
		config:                     config,
		state:                      state,
		reverseDNS:                 newReverseDNS(config),
		httpMonitor:                newHTTPMonitor(config, ebpfTracer, bpfTelemetry, constantEditors),
		activeBuffer:               network.NewConnectionBuffer(512, 256),
		conntracker:                conntracker,
		sourceExcludes:             network.ParseConnectionFilters(config.ExcludedSourceConnections),
		destExcludes:               network.ParseConnectionFilters(config.ExcludedDestinationConnections),
		sysctlUDPConnTimeout:       sysctl.NewInt(config.ProcRoot, "net/netfilter/nf_conntrack_udp_timeout", time.Minute),
		sysctlUDPConnStreamTimeout: sysctl.NewInt(config.ProcRoot, "net/netfilter/nf_conntrack_udp_timeout_stream", time.Minute),
		gwLookup:                   gwLookup,
		ebpfTracer:                 ebpfTracer,

		skippedConns:     atomic.NewInt64(0),
		expiredTCPConns:  atomic.NewInt64(0),
		closedConns:      atomic.NewInt64(0),
		connStatsMapSize: atomic.NewInt64(0),
		lastCheck:        atomic.NewInt64(0),
		bpfTelemetry:     bpfTelemetry,
	}

	err = ebpfTracer.Start(tr.storeClosedConnections)
	if err != nil {
		tr.Stop()
		return nil, fmt.Errorf("could not start ebpf manager: %s", err)
	}

	if err = tr.reverseDNS.Start(); err != nil {
		return nil, fmt.Errorf("could not start reverse dns monitor: %w", err)
	}

	return tr, nil
}

func newConntracker(cfg *config.Config, bpfTelemetry *telemetry.EBPFTelemetry) (netlink.Conntracker, error) {
	if !cfg.EnableConntrack {
		return netlink.NewNoOpConntracker(), nil
	}

	var c netlink.Conntracker
	var err error
	if cfg.EnableRuntimeCompiler {
		c, err = NewEBPFConntracker(cfg, bpfTelemetry)
		if err == nil {
			return c, nil
		}

		if !cfg.AllowPrecompiledFallback {
			if cfg.IgnoreConntrackInitFailure {
				log.Warnf("could not initialize ebpf conntrack, tracer will continue without NAT tracking: %s", err)
				return netlink.NewNoOpConntracker(), nil
			}
			return nil, fmt.Errorf("error initializing ebpf conntracker: %s. set network_config.ignore_conntrack_init_failure to true to ignore conntrack failures on startup", err)
		}

		log.Warnf("error initializing ebpf conntracker, falling back to netlink version: %s", err)
	}

	c, err = netlink.NewConntracker(cfg)
	if err != nil {
		if cfg.IgnoreConntrackInitFailure {
			log.Warnf("could not initialize netlink conntrack, tracer will continue without NAT tracking: %s", err)
			return netlink.NewNoOpConntracker(), nil
		}
		return nil, fmt.Errorf("could not initialize conntrack: %s. set network_config.ignore_conntrack_init_failure to true to ignore conntrack failures on startup", err)
	}
	return c, nil
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

func runOffsetGuessing(config *config.Config, buf bytecode.AssetReader) ([]manager.ConstantEditor, error) {
	// Enable kernel probes used for offset guessing.
	offsetMgr := newOffsetManager()
	offsetOptions := manager.Options{
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
	}
	enabledProbes, err := offsetGuessProbes(config)
	if err != nil {
		return nil, fmt.Errorf("unable to configure offset guessing probes: %w", err)
	}

	for _, p := range offsetMgr.Probes {
		if _, enabled := enabledProbes[probes.ProbeName(p.EBPFSection)]; !enabled {
			offsetOptions.ExcludedFunctions = append(offsetOptions.ExcludedFunctions, p.EBPFFuncName)
		}
	}
	for probeName, funcName := range enabledProbes {
		offsetOptions.ActivatedProbes = append(
			offsetOptions.ActivatedProbes,
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  string(probeName),
					EBPFFuncName: funcName,
					UID:          "offset",
				},
			})
	}
	if err := offsetMgr.InitWithOptions(buf, offsetOptions); err != nil {
		return nil, fmt.Errorf("could not load bpf module for offset guessing: %s", err)
	}

	if err := offsetMgr.Start(); err != nil {
		return nil, fmt.Errorf("could not start offset ebpf manager: %s", err)
	}
	defer func() {
		err := offsetMgr.Stop(manager.CleanAll)
		if err != nil {
			log.Warnf("error stopping offset ebpf manager: %s", err)
		}
	}()
	start := time.Now()
	editors, err := guessOffsets(offsetMgr, config)
	if err != nil {
		return nil, err
	}
	log.Infof("socket struct offset guessing complete (took %v)", time.Since(start))
	return editors, nil
}

func (t *Tracer) storeClosedConnections(connections []network.ConnectionStats) {
	var rejected int
	for i := range connections {
		cs := &connections[i]
		if t.shouldSkipConnection(cs) {
			connections[rejected], connections[i] = connections[i], connections[rejected]
			rejected++
			continue
		}

		cs.IPTranslation = t.conntracker.GetTranslationForConn(*cs)
		t.connVia(cs)
		if cs.IPTranslation != nil {
			t.conntracker.DeleteTranslation(*cs)
		}
	}

	connections = connections[rejected:]
	t.closedConns.Add(int64(len(connections)))
	t.skippedConns.Add(int64(rejected))
	t.state.StoreClosedConnections(connections)
}

// Stop stops the tracer
func (t *Tracer) Stop() {
	if t.gwLookup != nil {
		t.gwLookup.Close()
	}
	t.reverseDNS.Close()
	t.ebpfTracer.Stop()
	t.httpMonitor.Stop()
	t.conntracker.Close()
}

// GetActiveConnections returns the delta for connection info from the last time it was called with the same clientID
func (t *Tracer) GetActiveConnections(clientID string) (*network.Connections, error) {
	t.bufferLock.Lock()
	defer t.bufferLock.Unlock()
	log.Tracef("GetActiveConnections clientID=%s", clientID)

	t.ebpfTracer.FlushPending()
	latestTime, err := t.getConnections(t.activeBuffer)
	if err != nil {
		return nil, fmt.Errorf("error retrieving connections: %s", err)
	}
	active := t.activeBuffer.Connections()

	delta := t.state.GetDelta(clientID, latestTime, active, t.reverseDNS.GetDNSStats(), t.httpMonitor.GetHTTPStats())
	t.activeBuffer.Reset()

	ips := make([]util.Address, 0, len(delta.Conns)*2)
	for _, conn := range delta.Conns {
		ips = append(ips, conn.Source, conn.Dest)
	}
	names := t.reverseDNS.Resolve(ips)
	ctm := t.state.GetTelemetryDelta(clientID, t.getConnTelemetry(len(active)))
	rctm := t.getRuntimeCompilationTelemetry()
	khfr := int32(kernel.HeaderProvider.GetResult())
	coretm := ddebpf.GetCORETelemetryByAsset()
	t.lastCheck.Store(time.Now().Unix())

	return &network.Connections{
		BufferedData:                delta.BufferedData,
		DNS:                         names,
		DNSStats:                    delta.DNSStats,
		HTTP:                        delta.HTTP,
		ConnTelemetry:               ctm,
		KernelHeaderFetchResult:     khfr,
		CompilationTelemetryByAsset: rctm,
		CORETelemetryByAsset:        coretm,
	}, nil
}

// RegisterClient registers a clientID with the tracer
func (t *Tracer) RegisterClient(clientID string) error {
	t.state.RegisterClient(clientID)
	return nil
}

func (t *Tracer) getConnTelemetry(mapSize int) map[network.ConnTelemetryType]int64 {
	kprobeStats := ddebpf.GetProbeTotals()
	tm := map[network.ConnTelemetryType]int64{
		network.MonotonicKprobesTriggered: kprobeStats.Hits,
		network.MonotonicKprobesMissed:    kprobeStats.Misses,
		network.ConnsBpfMapSize:           int64(mapSize),
		network.MonotonicConnsClosed:      t.closedConns.Load(),
	}

	stats, err := t.getStats(conntrackStats, dnsStats, epbfStats, httpStats, stateStats)
	if err != nil {
		return nil
	}

	conntrackStats := stats["conntrack"].(map[string]int64)
	if rt, ok := conntrackStats["registers_total"]; ok {
		tm[network.MonotonicConntrackRegisters] = rt
	}
	if sp, ok := conntrackStats["sampling_pct"]; ok {
		tm[network.ConntrackSamplingPercent] = sp
	}

	dnsStats := stats["dns"].(map[string]int64)
	if pp, ok := dnsStats["packets_processed"]; ok {
		tm[network.MonotonicDNSPacketsProcessed] = pp
	}

	if ds, ok := dnsStats["dropped_stats"]; ok {
		tm[network.DNSStatsDropped] = ds
	}

	ebpfStats := stats["ebpf"].(map[string]int64)
	if usp, ok := ebpfStats["udp_sends_processed"]; ok {
		tm[network.MonotonicUDPSendsProcessed] = usp
	}
	if usm, ok := ebpfStats["udp_sends_missed"]; ok {
		tm[network.MonotonicUDPSendsMissed] = usm
	}
	if pl, ok := ebpfStats["closed_conn_polling_lost"]; ok {
		tm[network.MonotonicPerfLost] = pl
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
		"http":            runtime.Http.GetTelemetry(),
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
func (t *Tracer) getConnections(activeBuffer *network.ConnectionBuffer) (latestUint uint64, err error) {
	cachedConntrack := newCachedConntrack(t.config.ProcRoot, netlink.NewConntrack, 128)
	defer func() { _ = cachedConntrack.Close() }()

	latestTime, err := ddebpf.NowNanoseconds()
	if err != nil {
		return 0, fmt.Errorf("error retrieving latest timestamp: %s", err)
	}

	var expired []network.ConnectionStats
	err = t.ebpfTracer.GetConnections(activeBuffer, func(c *network.ConnectionStats) bool {
		if t.connectionExpired(c, uint64(latestTime), cachedConntrack) {
			expired = append(expired, *c)
			if c.Type == network.TCP {
				t.expiredTCPConns.Inc()
			}
			t.closedConns.Inc()
			return false
		}

		if t.shouldSkipConnection(c) {
			t.skippedConns.Inc()
			return false
		}
		return true
	})
	if err != nil {
		return 0, err
	}

	active := activeBuffer.Connections()
	for i := range active {
		active[i].IPTranslation = t.conntracker.GetTranslationForConn(active[i])
		// do gateway resolution only on active connections outside
		// the map iteration loop to not add to connections while
		// iterating (leads to ever-increasing connections in the map,
		// since gateway resolution connects to the ec2 metadata
		// endpoint)
		t.connVia(&active[i])
	}

	entryCount := len(active)
	if entryCount >= int(t.config.MaxTrackedConnections) {
		log.Errorf("connection tracking map size has reached the limit of %d. Accurate connection count and data volume metrics will be affected. Increase config value `system_probe_config.max_tracked_connections` to correct this.", t.config.MaxTrackedConnections)
	} else if (float64(entryCount) / float64(t.config.MaxTrackedConnections)) >= 0.9 {
		log.Warnf("connection tracking map size of %d is approaching the limit of %d. The config value `system_probe_config.max_tracked_connections` may be increased to avoid any accuracy problems.", entryCount, t.config.MaxTrackedConnections)
	}
	t.connStatsMapSize.Store(int64(entryCount))

	// Remove expired entries
	t.removeEntries(expired)

	// check for expired clients in the state
	t.state.RemoveExpiredClients(time.Now())

	latestTime, err = ddebpf.NowNanoseconds()
	if err != nil {
		return 0, fmt.Errorf("error retrieving latest timestamp: %s", err)
	}
	return uint64(latestTime), nil
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

	log.Debugf("Removed %d connection entries in %s", len(toRemove), time.Now().Sub(now))
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
	bpfMapStats
	bpfHelperStats
)

var allStats = []statsComp{
	conntrackStats,
	dnsStats,
	epbfStats,
	gatewayLookupStats,
	kprobesStats,
	stateStats,
	tracerStats,
	bpfHelperStats,
	bpfMapStats,
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
		case conntrackStats:
			ret["conntrack"] = t.conntracker.GetStats()
		case dnsStats:
			ret["dns"] = t.reverseDNS.GetStats()
		case epbfStats:
			ret["ebpf"] = t.ebpfTracer.GetTelemetry()
		case gatewayLookupStats:
			ret["gateway_lookup"] = t.gwLookup.GetStats()
		case kprobesStats:
			ret["kprobes"] = ddebpf.GetProbeStats()
		case stateStats:
			ret["state"] = t.state.GetStats()["telemetry"]
		case tracerStats:
			tracerStats := atomicstats.Report(t)
			tracerStats["runtime"] = runtime.Tracer.GetTelemetry()
			ret["tracer"] = tracerStats
		case bpfMapStats:
			ret["map_ops"] = t.bpfTelemetry.GetMapsTelemetry()
		case bpfHelperStats:
			ret["ebpf_helpers"] = t.bpfTelemetry.GetHelperTelemetry()
		}
	}

	// merge with components already migrated to `network/telemetry`
	for k, v := range telemetry.ReportExpvar() {
		ret[k] = v
	}

	return ret, nil
}

// GetStats returns a map of statistics about the current tracer's internal state
func (t *Tracer) GetStats() (map[string]interface{}, error) {
	return t.getStats()
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
	_, err := t.getConnections(activeBuffer)
	if err != nil {
		return nil, fmt.Errorf("error retrieving connections: %s", err)
	}
	return &network.Connections{
		BufferedData: network.BufferedData{
			Conns: activeBuffer.Connections(),
		},
	}, nil

}

// DebugEBPFMaps returns all maps registred in the eBPF manager
func (t *Tracer) DebugEBPFMaps(maps ...string) (string, error) {
	tracerMaps, err := t.ebpfTracer.DumpMaps(maps...)
	if err != nil {
		return "", err
	}
	if t.httpMonitor == nil {
		return "tracer:\n" + tracerMaps, nil
	}

	httpMaps, err := t.httpMonitor.DumpMaps(maps...)
	if err != nil {
		return "", err
	}
	return "tracer:\n" + tracerMaps + "\nhttp_monitor:\n" + httpMaps, nil
}

// connectionExpired returns true if the passed in connection has expired
//
// expiry is handled differently for UDP and TCP. For TCP where conntrack TTL is very long, we use a short expiry for userspace tracking
// but use conntrack as a source of truth to keep long-lived idle TCP conns in the userspace state, while evicting closed TCP connections.
// for UDP, the conntrack TTL is lower (two minutes), so the userspace and conntrack expiry are synced to avoid touching conntrack for
// UDP expiries
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

	rootNS, err := util.GetInoForNs(ns)
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

	rootNS, err := util.GetInoForNs(ns)
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

func newHTTPMonitor(c *config.Config, tracer connection.Tracer, bpfTelemetry *telemetry.EBPFTelemetry, offsets []manager.ConstantEditor) *http.Monitor {
	if !c.EnableHTTPMonitoring {
		return nil
	}

	// Shared with the HTTP program
	sockFDMap := tracer.GetMap(string(probes.SockByPidFDMap))

	monitor, err := http.NewMonitor(c, offsets, sockFDMap, bpfTelemetry)
	if err != nil {
		log.Errorf("could not instantiate http monitor: %s", err)
		return nil
	}

	err = monitor.Start()
	if errors.Is(err, syscall.ENOMEM) {
		log.Error("could not enable http monitoring: not enough memory to attach http ebpf socket filter. please consider raising the limit via sysctl -w net.core.optmem_max=<LIMIT>")
		return nil
	}

	if err != nil {
		log.Errorf("could not enable http monitoring: %s", err)
		return nil
	}

	log.Info("http monitoring enabled")
	return monitor
}
