// +build linux_bpf

package tracer

/*
#include "../ebpf/c/tracer.h"
*/
import "C"
import (
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/config/sysctl"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	filterpkg "github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"golang.org/x/sys/unix"
)

const defaultUDPConnTimeoutNanoSeconds uint64 = uint64(time.Duration(120) * time.Second)

type Tracer struct {
	m *manager.Manager

	config *config.Config

	state network.State

	conntracker netlink.Conntracker

	reverseDNS network.ReverseDNS

	httpMonitor *http.Monitor

	perfHandler   *ddebpf.PerfHandler
	batchManager  *PerfBatchManager
	flushIdle     chan chan struct{}
	stop          chan struct{}
	runtimeTracer bool

	// Telemetry
	perfReceived  int64
	perfLost      int64
	skippedConns  int64
	pidCollisions int64
	// Will track the count of expired TCP connections
	// We are manually expiring TCP connections because it seems that we are losing some of the TCP close events
	// For now we are only tracking the `tcp_close` probe but we should also track the `tcp_set_state` probe when
	// states are set to `TCP_CLOSE_WAIT`, `TCP_CLOSE` and `TCP_FIN_WAIT1` we should probably also track `tcp_time_wait`
	// However there are some caveats by doing that:
	// - `tcp_set_state` does not have access to the PID of the connection => we have to remove the PID from the connection tuple (which can lead to issues)
	// - We will have multiple probes that can trigger for the same connection close event => we would have to add something to dedupe those
	// - - Using the timestamp does not seem to be reliable (we are already seeing unordered connections)
	// - - Having IDs for those events would need to have an internal monotonic counter and this is tricky to manage (race conditions, cleaning)
	//
	// If we want to have a way to track the # of active TCP connections in the future we could use the procfs like here: https://github.com/DataDog/datadog-agent/pull/3728
	// to determine whether a connection is truly closed or not
	expiredTCPConns  int64
	closedConns      int64
	connStatsMapSize int64

	buffer     []network.ConnectionStats
	bufferLock sync.Mutex

	// Internal buffer used to compute bytekeys
	buf []byte

	// Connections for the tracer to blacklist
	sourceExcludes []*network.ConnectionFilter
	destExcludes   []*network.ConnectionFilter

	gwLookup *gatewayLookup

	sysctlUDPConnTimeout       *sysctl.Int
	sysctlUDPConnStreamTimeout *sysctl.Int
}

const (
	defaultClosedChannelSize = 500
)

func NewTracer(config *config.Config) (*Tracer, error) {
	// make sure debugfs is mounted
	if mounted, err := kernel.IsDebugFSMounted(); !mounted {
		return nil, fmt.Errorf("%s: %s", "system-probe unsupported", err)
	}

	// check if current platform is using old kernel API because it affects what kprobe are we going to enable
	currKernelVersion, err := kernel.HostVersion()
	if err != nil {
		// if the platform couldn't be determined, treat it as new kernel case
		log.Warn("could not detect the platform, will use kprobes from kernel version >= 4.1.0")
	}

	// check to see if current kernel is earlier than version 4.1.0
	pre410Kernel := currKernelVersion < kernel.VersionCode(4, 1, 0)
	if pre410Kernel {
		log.Infof("detected platform %s, switch to use kprobes from kernel version < 4.1.0", currKernelVersion)
	}

	runtimeTracer := false
	var buf bytecode.AssetReader
	if config.EnableRuntimeCompiler {
		runtime.RuntimeCompilationEnabled = true
		buf, err = getRuntimeCompiledTracer(config)
		if err != nil {
			if !config.AllowPrecompiledFallback {
				return nil, fmt.Errorf("error compiling network tracer: %s", err)
			}
			log.Warnf("error compiling network tracer, falling back to pre-compiled: %s", err)
		} else {
			runtimeTracer = true
			defer buf.Close()
		}
	}

	// Use the config to determine what kernel probes should be enabled
	enabledProbes, err := config.EnabledProbes(runtimeTracer)
	if err != nil {
		return nil, fmt.Errorf("invalid probe configuration: %v", err)
	}

	enableSocketFilter := config.DNSInspection && !pre410Kernel
	if enableSocketFilter {
		enabledProbes[probes.SocketDnsFilter] = struct{}{}
	}

	mgrOptions := manager.Options{
		// Extend RLIMIT_MEMLOCK (8) size
		// On some systems, the default for RLIMIT_MEMLOCK may be as low as 64 bytes.
		// This will result in an EPERM (Operation not permitted) error, when trying to create an eBPF map
		// using bpf(2) with BPF_MAP_CREATE.
		//
		// We are setting the limit to infinity until we have a better handle on the true requirements.
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		MapSpecEditors: map[string]manager.MapSpecEditor{
			string(probes.ConnMap):            {Type: ebpf.Hash, MaxEntries: uint32(config.MaxTrackedConnections), EditorFlag: manager.EditMaxEntries},
			string(probes.TcpStatsMap):        {Type: ebpf.Hash, MaxEntries: uint32(config.MaxTrackedConnections), EditorFlag: manager.EditMaxEntries},
			string(probes.PortBindingsMap):    {Type: ebpf.Hash, MaxEntries: uint32(config.MaxTrackedConnections), EditorFlag: manager.EditMaxEntries},
			string(probes.UdpPortBindingsMap): {Type: ebpf.Hash, MaxEntries: uint32(config.MaxTrackedConnections), EditorFlag: manager.EditMaxEntries},
		},
	}

	if buf == nil {
		buf, err = netebpf.ReadBPFModule(config.BPFDir, config.BPFDebug)
		if err != nil {
			return nil, fmt.Errorf("could not read bpf module: %s", err)
		}
		defer buf.Close()

		offsetBuf, err := netebpf.ReadOffsetBPFModule(config.BPFDir, config.BPFDebug)
		if err != nil {
			return nil, fmt.Errorf("could not read offset bpf module: %s", err)
		}
		defer offsetBuf.Close()

		// Offset guessing has been flaky for some customers, so if it fails we'll retry it up to 5 times
		for i := 0; i < 5; i++ {
			mgrOptions.ConstantEditors, err = runOffsetGuessing(config, offsetBuf)
			if err == nil {
				break
			}
			time.Sleep(1 * time.Second)
		}
		if err != nil {
			return nil, fmt.Errorf("error guessing offsets: %s", err)
		}

		if enableSocketFilter && config.CollectDNSStats {
			mgrOptions.ConstantEditors = append(mgrOptions.ConstantEditors, manager.ConstantEditor{
				Name:  "dns_stats_enabled",
				Value: uint64(1),
			})
		}
	}

	closedChannelSize := defaultClosedChannelSize
	if config.ClosedChannelSize > 0 {
		closedChannelSize = config.ClosedChannelSize
	}
	perfHandlerTCP := ddebpf.NewPerfHandler(closedChannelSize)
	m := netebpf.NewManager(perfHandlerTCP, runtimeTracer)

	// exclude all non-enabled probes to ensure we don't run into problems with unsupported probe types
	for _, p := range m.Probes {
		if _, enabled := enabledProbes[probes.ProbeName(p.Section)]; !enabled {
			mgrOptions.ExcludedSections = append(mgrOptions.ExcludedSections, p.Section)
		}
	}
	for probeName := range enabledProbes {
		mgrOptions.ActivatedProbes = append(
			mgrOptions.ActivatedProbes,
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					Section: string(probeName),
				},
			})
	}
	err = m.InitWithOptions(buf, mgrOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to init ebpf manager: %v", err)
	}

	reverseDNS, err := newReverseDNS(config, m, pre410Kernel)
	if err != nil {
		return nil, fmt.Errorf("error enabling DNS traffic inspection: %s", err)
	}

	err = initializePortBindingMaps(config, m)
	if err != nil {
		return nil, fmt.Errorf("error initializing port binding maps: %s", err)
	}

	creator := netlink.NewConntracker
	if runtimeTracer {
		creator = NewEBPFConntracker
	}
	conntracker, err := newConntracker(config, creator)
	if err != nil {
		return nil, err
	}

	state := network.NewState(
		config.ClientStateExpiry,
		config.MaxClosedConnectionsBuffered,
		config.MaxConnectionsStateBuffered,
		config.MaxDNSStatsBuffered,
		config.MaxHTTPStatsBuffered,
		config.CollectDNSDomains,
	)

	tr := &Tracer{
		m:                          m,
		config:                     config,
		state:                      state,
		reverseDNS:                 reverseDNS,
		httpMonitor:                newHTTPMonitor(!pre410Kernel, config),
		buffer:                     make([]network.ConnectionStats, 0, 512),
		conntracker:                conntracker,
		sourceExcludes:             network.ParseConnectionFilters(config.ExcludedSourceConnections),
		destExcludes:               network.ParseConnectionFilters(config.ExcludedDestinationConnections),
		perfHandler:                perfHandlerTCP,
		flushIdle:                  make(chan chan struct{}),
		stop:                       make(chan struct{}),
		buf:                        make([]byte, network.ConnectionByteKeyMaxLen),
		runtimeTracer:              runtimeTracer,
		sysctlUDPConnTimeout:       sysctl.NewInt(config.ProcRoot, "net/netfilter/nf_conntrack_udp_timeout", time.Minute),
		sysctlUDPConnStreamTimeout: sysctl.NewInt(config.ProcRoot, "net/netfilter/nf_conntrack_udp_timeout_stream", time.Minute),
		gwLookup:                   newGatewayLookup(config, m),
	}

	tr.batchManager, err = tr.initPerfPolling(perfHandlerTCP)
	if err != nil {
		return nil, fmt.Errorf("could not start polling bpf events: %s", err)
	}

	if err = m.Start(); err != nil {
		return nil, fmt.Errorf("could not start ebpf manager: %s", err)
	}

	return tr, nil
}

func newConntracker(cfg *config.Config, conntrackerCreator func(*config.Config) (netlink.Conntracker, error)) (netlink.Conntracker, error) {
	conntracker := netlink.NewNoOpConntracker()
	if !cfg.EnableConntrack {
		return conntracker, nil
	}

	if c, err := conntrackerCreator(cfg); err != nil {
		if cfg.IgnoreConntrackInitFailure {
			log.Warnf("could not initialize conntrack, tracer will continue without NAT tracking: %s", err)
		} else {
			return nil, fmt.Errorf("could not initialize conntrack: %s. set network_config.ignore_conntrack_init_failure to true to ignore conntrack failures on startup", err)
		}
	} else {
		conntracker = c
	}

	return conntracker, nil
}

func initializePortBindingMaps(config *config.Config, m *manager.Manager) error {
	if tcpPorts, err := network.ReadInitialState(config.ProcRoot, network.TCP, config.CollectIPv6Conns); err != nil {
		return fmt.Errorf("failed to read initial TCP pid->port mapping: %s", err)
	} else {
		tcpPortMap, _, err := m.GetMap(string(probes.PortBindingsMap))
		if err != nil {
			return fmt.Errorf("failed to get TCP port binding map: %w", err)
		}
		for p := range tcpPorts {
			log.Debugf("adding initial TCP port binding: netns: %d port: %d", p.Ino, p.Port)
			pb := portBindingTuple{netns: C.__u32(p.Ino), port: C.__u16(p.Port)}
			state := uint8(C.PORT_LISTENING)
			err = tcpPortMap.Update(unsafe.Pointer(&pb), unsafe.Pointer(&state), ebpf.UpdateNoExist)
			if err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
				return fmt.Errorf("failed to update TCP port binding map: %w", err)
			}
		}
	}

	if udpPorts, err := network.ReadInitialState(config.ProcRoot, network.UDP, config.CollectIPv6Conns); err != nil {
		return fmt.Errorf("failed to read initial UDP pid->port mapping: %s", err)
	} else {
		udpPortMap, _, err := m.GetMap(string(probes.UdpPortBindingsMap))
		if err != nil {
			return fmt.Errorf("failed to get UDP port binding map: %w", err)
		}
		for p := range udpPorts {
			log.Debugf("adding initial UDP port binding: netns: %d port: %d", p.Ino, p.Port)
			// UDP port bindings currently do not have network namespace numbers
			pb := portBindingTuple{netns: 0, port: C.__u16(p.Port)}
			state := uint8(C.PORT_LISTENING)
			err = udpPortMap.Update(unsafe.Pointer(&pb), unsafe.Pointer(&state), ebpf.UpdateNoExist)
			if err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
				return fmt.Errorf("failed to update UDP port binding map: %w", err)
			}
		}
	}
	return nil
}

func newReverseDNS(cfg *config.Config, m *manager.Manager, pre410Kernel bool) (network.ReverseDNS, error) {
	if !cfg.DNSInspection {
		return network.NewNullReverseDNS(), nil
	}

	if pre410Kernel {
		log.Warn("DNS inspection not supported by kernel versions < 4.1.0. Please see https://docs.datadoghq.com/network_performance_monitoring/installation for more details.")
		return network.NewNullReverseDNS(), nil
	}

	filter, _ := m.GetProbe(manager.ProbeIdentificationPair{Section: string(probes.SocketDnsFilter)})
	if filter == nil {
		return nil, fmt.Errorf("error retrieving socket filter")
	}

	// Create the RAW_SOCKET inside the root network namespace
	var (
		packetSrc *filterpkg.AFPacketSource
		srcErr    error
	)
	err := util.WithRootNS(cfg.ProcRoot, func() error {
		packetSrc, srcErr = filterpkg.NewPacketSource(filter)
		return srcErr
	})

	if err != nil {
		return nil, err
	}

	return network.NewSocketFilterSnooper(cfg, packetSrc)
}

func runOffsetGuessing(config *config.Config, buf bytecode.AssetReader) ([]manager.ConstantEditor, error) {
	// Enable kernel probes used for offset guessing.
	offsetMgr := netebpf.NewOffsetManager()
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
		if _, enabled := enabledProbes[probes.ProbeName(p.Section)]; !enabled {
			offsetOptions.ExcludedSections = append(offsetOptions.ExcludedSections, p.Section)
		}
	}
	for probeName := range enabledProbes {
		offsetOptions.ActivatedProbes = append(
			offsetOptions.ActivatedProbes,
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					Section: string(probeName),
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

// initPerfPolling starts the listening on perf buffer events to grab closed connections
func (t *Tracer) initPerfPolling(perf *ddebpf.PerfHandler) (*PerfBatchManager, error) {
	connCloseEventMap, _ := t.getMap(probes.ConnCloseEventMap)
	connCloseMap, _ := t.getMap(probes.ConnCloseBatchMap)
	numCPUs := int(connCloseEventMap.ABI().MaxEntries)
	batchManager, err := NewPerfBatchManager(connCloseMap, numCPUs)
	if err != nil {
		return nil, err
	}

	go func() {
		// Stats about how much connections have been closed / lost
		ticker := time.NewTicker(5 * time.Minute)
		for {
			select {
			case batchData, ok := <-perf.DataChannel:
				if !ok {
					return
				}
				atomic.AddInt64(&t.perfReceived, 1)

				batch := toBatch(batchData.Data)
				conns := t.batchManager.Extract(batch, batchData.CPU)
				for _, c := range conns {
					t.storeClosedConn(&c)
				}
			case lostCount, ok := <-perf.LostChannel:
				if !ok {
					return
				}
				atomic.AddInt64(&t.perfLost, int64(lostCount))
			case done, ok := <-t.flushIdle:
				if !ok {
					return
				}
				idleConns := t.batchManager.GetIdleConns()
				for _, c := range idleConns {
					t.storeClosedConn(&c)
				}
				close(done)
			case <-ticker.C:
				recv := atomic.SwapInt64(&t.perfReceived, 0)
				lost := atomic.SwapInt64(&t.perfLost, 0)
				skip := atomic.SwapInt64(&t.skippedConns, 0)
				tcpExpired := atomic.SwapInt64(&t.expiredTCPConns, 0)
				if lost > 0 {
					log.Warnf("closed connection polling: %d received, %d lost, %d skipped, %d expired TCP", recv, lost, skip, tcpExpired)
				}
			}
		}
	}()

	return batchManager, nil
}

// shouldSkipConnection returns whether or not the tracer should ignore a given connection:
//  • Local DNS (*:53) requests if configured (default: true)
func (t *Tracer) shouldSkipConnection(conn *network.ConnectionStats) bool {
	isDNSConnection := conn.DPort == 53 || conn.SPort == 53
	if !t.config.CollectLocalDNS && isDNSConnection && conn.Dest.IsLoopback() {
		return true
	} else if network.IsExcludedConnection(t.sourceExcludes, t.destExcludes, conn) {
		return true
	}
	return false
}

func (t *Tracer) storeClosedConn(cs *network.ConnectionStats) {
	if t.shouldSkipConnection(cs) {
		atomic.AddInt64(&t.skippedConns, 1)
		return
	}

	atomic.AddInt64(&t.closedConns, 1)
	cs.IPTranslation = t.conntracker.GetTranslationForConn(*cs)
	t.connVia(cs)
	t.state.StoreClosedConnection(cs)
	if cs.IPTranslation != nil {
		t.conntracker.DeleteTranslation(*cs)
	}
}

func (t *Tracer) Stop() {
	close(t.stop)
	t.reverseDNS.Close()
	_ = t.m.Stop(manager.CleanAll)
	t.perfHandler.Stop()
	t.httpMonitor.Stop()
	close(t.flushIdle)
	t.conntracker.Close()
}

func (t *Tracer) GetActiveConnections(clientID string) (*network.Connections, error) {
	t.bufferLock.Lock()
	defer t.bufferLock.Unlock()
	log.Tracef("GetActiveConnections clientID=%s", clientID)

	latestConns, latestTime, err := t.getConnections(t.buffer[:0])
	if err != nil {
		return nil, fmt.Errorf("error retrieving connections: %s", err)
	}

	// Grow or shrink buffer depending on the usage
	if len(latestConns) >= cap(t.buffer)*2 {
		t.buffer = make([]network.ConnectionStats, 0, cap(t.buffer)*2)
	} else if len(latestConns) <= cap(t.buffer)/2 {
		t.buffer = make([]network.ConnectionStats, 0, cap(t.buffer)/2)
	}

	// Ensure that TCP closed connections are flushed
	done := make(chan struct{})
	t.flushIdle <- done
	<-done

	delta := t.state.GetDelta(clientID, latestTime, latestConns, t.reverseDNS.GetDNSStats(), t.httpMonitor.GetHTTPStats())
	names := t.reverseDNS.Resolve(delta.Connections)
	ctm := t.getConnTelemetry(len(latestConns))
	rctm := t.getRuntimeCompilationTelemetry()

	return &network.Connections{
		Conns:                       delta.Connections,
		DNS:                         names,
		HTTP:                        delta.HTTP,
		ConnTelemetry:               ctm,
		CompilationTelemetryByAsset: rctm,
	}, nil
}

func (t *Tracer) getConnTelemetry(mapSize int) *network.ConnectionsTelemetry {
	kprobeStats := ddebpf.GetProbeTotals()
	tm := &network.ConnectionsTelemetry{
		MonotonicKprobesTriggered: kprobeStats.Hits,
		MonotonicKprobesMissed:    kprobeStats.Misses,
		ConnsBpfMapSize:           int64(mapSize),
		MonotonicConnsClosed:      atomic.LoadInt64(&t.closedConns),
	}

	conntrackStats := t.conntracker.GetStats()
	if rt, ok := conntrackStats["registers_total"]; ok {
		tm.MonotonicConntrackRegisters = rt
	}
	if rtd, ok := conntrackStats["registers_dropped"]; ok {
		tm.MonotonicConntrackRegistersDropped = rtd
	}
	if sp, ok := conntrackStats["sampling_pct"]; ok {
		tm.ConntrackSamplingPercent = sp
	}

	dnsStats := t.reverseDNS.GetStats()
	if pp, ok := dnsStats["packets_processed"]; ok {
		tm.MonotonicDNSPacketsProcessed = pp
	}

	if ds, ok := dnsStats["dropped_stats"]; ok {
		tm.DNSStatsDropped = ds
	}

	ebpfStats := t.getEbpfTelemetry()
	if usp, ok := ebpfStats["udp_sends_processed"]; ok {
		tm.MonotonicUDPSendsProcessed = usp
	}
	if usm, ok := ebpfStats["udp_sends_missed"]; ok {
		tm.MonotonicUDPSendsMissed = usm
	}

	return tm
}

func (t *Tracer) getRuntimeCompilationTelemetry() map[string]network.RuntimeCompilationTelemetry {
	telemetryByAsset := map[string]map[string]int64{
		"tracer":    runtime.Tracer.GetTelemetry(),
		"conntrack": runtime.Conntrack.GetTelemetry(),
	}

	result := make(map[string]network.RuntimeCompilationTelemetry)
	for assetName, telemetry := range telemetryByAsset {
		tm := network.RuntimeCompilationTelemetry{}
		if enabled, ok := telemetry["runtime_compilation_enabled"]; ok {
			tm.RuntimeCompilationEnabled = enabled == 1
		}
		if result, ok := telemetry["runtime_compilation_result"]; ok {
			tm.RuntimeCompilationResult = int32(result)
		}
		if duration, ok := telemetry["runtime_compilation_duration"]; ok {
			tm.RuntimeCompilationDuration = duration
		}
		result[assetName] = tm
	}

	return result
}

// getConnections returns all of the active connections in the ebpf maps along with the latest timestamp.  It takes
// a reusable buffer for appending the active connections so that this doesn't continuously allocate
func (t *Tracer) getConnections(active []network.ConnectionStats) ([]network.ConnectionStats, uint64, error) {
	mp, err := t.getMap(probes.ConnMap)
	if err != nil {
		return nil, 0, fmt.Errorf("error retrieving the bpf %s map: %s", probes.ConnMap, err)
	}

	tcpMp, err := t.getMap(probes.TcpStatsMap)
	if err != nil {
		return nil, 0, fmt.Errorf("error retrieving the bpf %s map: %s", probes.TcpStatsMap, err)
	}

	latestTime, ok, err := t.getLatestTimestamp()
	if err != nil {
		return nil, 0, fmt.Errorf("error retrieving latest timestamp: %s", err)
	}

	if !ok { // if no timestamps have been captured, there can be no packets
		return nil, 0, nil
	}

	cachedConntrack := newCachedConntrack(t.config.ProcRoot, netlink.NewConntrack, 128)
	defer cachedConntrack.Close()

	// Iterate through all key-value pairs in map
	key, stats := &ConnTuple{}, &ConnStatsWithTimestamp{}
	seen := make(map[ConnTuple]struct{})
	var expired []*ConnTuple
	var entryCount uint
	entries := mp.IterateFrom(unsafe.Pointer(&ConnTuple{}))
	for entries.Next(unsafe.Pointer(key), unsafe.Pointer(stats)) {
		entryCount++
		if t.connectionExpired(key, latestTime, stats, cachedConntrack) {
			expired = append(expired, key.copy())
			if key.isTCP() {
				atomic.AddInt64(&t.expiredTCPConns, 1)
			}
			atomic.AddInt64(&t.closedConns, 1)
		} else {
			conn := connStats(key, stats, t.getTCPStats(tcpMp, key, seen))
			if t.shouldSkipConnection(&conn) {
				atomic.AddInt64(&t.skippedConns, 1)
			} else {
				// lookup conntrack in for active
				conn.IPTranslation = t.conntracker.GetTranslationForConn(conn)
				active = append(active, conn)
			}
		}
	}

	if err := entries.Err(); err != nil {
		return nil, 0, fmt.Errorf("unable to iterate connection map: %s", err)
	}

	if entryCount >= t.config.MaxTrackedConnections {
		log.Errorf("connection tracking map size has reached the limit of %d. Accurate connection count and data volume metrics will be affected. Increase config value `system_probe_config.max_tracked_connections` to correct this.", t.config.MaxTrackedConnections)
	} else if (float64(entryCount) / float64(t.config.MaxTrackedConnections)) >= 0.9 {
		log.Warnf("connection tracking map size of %d is approaching the limit of %d. The config value `system_probe_config.max_tracked_connections` may be increased to avoid any accuracy problems.", entryCount, t.config.MaxTrackedConnections)
	}
	atomic.SwapInt64(&t.connStatsMapSize, int64(entryCount))

	// do gateway resolution only on active connections outside
	// the map iteration loop to not add to connections while
	// iterating (leads to ever increasing connections in the map,
	// since gateway resolution connects to the ec2 metadata
	// endpoint)
	for i := range active {
		t.connVia(&active[i])
	}

	// Remove expired entries
	t.removeEntries(mp, tcpMp, expired)

	// check for expired clients in the state
	t.state.RemoveExpiredClients(time.Now())

	// Get the latest time a second time because it could have changed while we were reading the eBPF map
	latestTime, _, err = t.getLatestTimestamp()
	if err != nil {
		return nil, 0, fmt.Errorf("error retrieving latest timestamp: %s", err)
	}

	return active, latestTime, nil
}

func (t *Tracer) removeEntries(mp, tcpMp *ebpf.Map, entries []*ConnTuple) {
	now := time.Now()
	// Byte keys of the connections to remove
	keys := make([]string, 0, len(entries))
	// Used to create the keys
	statsWithTs, tcpStats := &ConnStatsWithTimestamp{}, &TCPStats{}
	// Remove the entries from the eBPF Map
	for i := range entries {
		err := mp.Delete(unsafe.Pointer(entries[i]))
		if err != nil {
			// If this entry no longer exists in the eBPF map it means `tcp_close` has executed
			// during this function call. In that case state.StoreClosedConnection() was already called for this connection
			// and we can't delete the corresponding client state or we'll likely over-report the metric values.
			// By skipping to the next iteration and not calling state.RemoveConnections() we'll let
			// this connection expire "naturally" when either next connection check runs or the client itself expires.
			_ = log.Warnf("failed to remove entry from connections map: %s", err)
			continue
		}

		// Delete conntrack entry for this connection
		connStats := connStats(entries[i], statsWithTs, tcpStats)
		t.conntracker.DeleteTranslation(connStats)

		// Append the connection key to the keys to remove from the userspace state
		bk, err := connStats.ByteKey(t.buf)
		if err != nil {
			log.Warnf("failed to create connection byte_key: %s", err)
		} else {
			keys = append(keys, string(bk))
		}

		// We have to remove the PID to remove the element from the TCP Map since we don't use the pid there
		entries[i].pid = 0
		// We can ignore the error for this map since it will not always contain the entry
		_ = tcpMp.Delete(unsafe.Pointer(entries[i]))
	}

	t.state.RemoveConnections(keys)

	log.Debugf("Removed %d connection entries in %s", len(keys), time.Now().Sub(now))
}

// getTCPStats reads tcp related stats for the given ConnTuple
func (t *Tracer) getTCPStats(mp *ebpf.Map, tuple *ConnTuple, seen map[ConnTuple]struct{}) *TCPStats {
	stats := new(TCPStats)

	if !tuple.isTCP() {
		return stats
	}

	// The PID isn't used as a key in the stats map, we will temporarily set it to 0 here and reset it when we're done
	pid := tuple.pid
	tuple.pid = 0

	_ = mp.Lookup(unsafe.Pointer(tuple), unsafe.Pointer(stats))

	// This is required to avoid (over)reporting retransmits for connections sharing the same socket.
	if _, reported := seen[*tuple]; reported {
		atomic.AddInt64(&t.pidCollisions, 1)
		stats.retransmits = 0
	} else {
		seen[*tuple] = struct{}{}
	}

	tuple.pid = pid
	return stats
}

func (t *Tracer) getLatestTimestamp() (uint64, bool, error) {
	latestTime, err := ddebpf.NowNanoseconds()
	if err != nil {
		return 0, false, err
	}
	return uint64(latestTime), true, nil
}

// getEbpfTelemetry reads the telemetry map from the kernelspace and returns a map of key -> count
func (t *Tracer) getEbpfTelemetry() map[string]int64 {
	mp, err := t.getMap(probes.TelemetryMap)
	if err != nil {
		log.Warnf("error retrieving telemetry map: %s", err)
		return map[string]int64{}
	}

	telemetry := &kernelTelemetry{}
	if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(telemetry)); err != nil {
		// This can happen if we haven't initialized the telemetry object yet
		// so let's just use a trace log
		log.Tracef("error retrieving the telemetry struct: %s", err)
	}

	return map[string]int64{
		"tcp_sent_miscounts":         int64(telemetry.tcp_sent_miscounts),
		"missed_tcp_close":           int64(telemetry.missed_tcp_close),
		"missed_udp_close":           int64(telemetry.missed_udp_close),
		"udp_sends_processed":        int64(telemetry.udp_sends_processed),
		"udp_sends_missed":           int64(telemetry.udp_sends_missed),
		"conn_stats_max_entries_hit": int64(telemetry.conn_stats_max_entries_hit),
	}
}

func (t *Tracer) getMap(name probes.BPFMapName) (*ebpf.Map, error) {
	mp, _, err := t.m.GetMap(string(name))
	if mp == nil {
		return nil, fmt.Errorf("no map with name %s: %s", name, err)
	}
	return mp, nil
}

func (t *Tracer) timeoutForConn(c *ConnTuple, isAssured bool) uint64 {
	if c.isTCP() {
		return uint64(t.config.TCPConnTimeout.Nanoseconds())
	}

	return t.udpConnTimeout(isAssured)
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

// GetStats returns a map of statistics about the current tracer's internal state
func (t *Tracer) GetStats() (map[string]interface{}, error) {
	if t.state == nil {
		return nil, fmt.Errorf("internal state not yet initialized")
	}

	lost := atomic.LoadInt64(&t.perfLost)
	received := atomic.LoadInt64(&t.perfReceived)
	skipped := atomic.LoadInt64(&t.skippedConns)
	expiredTCP := atomic.LoadInt64(&t.expiredTCPConns)
	pidCollisions := atomic.LoadInt64(&t.pidCollisions)
	connStatsMapSize := atomic.LoadInt64(&t.connStatsMapSize)

	tracerStats := map[string]int64{
		"closed_conn_polling_lost":     lost,
		"closed_conn_polling_received": received,
		"conn_valid_skipped":           skipped, // Skipped connections (e.g. Local DNS requests)
		"expired_tcp_conns":            expiredTCP,
		"pid_collisions":               pidCollisions,
		"conn_stats_map_size":          connStatsMapSize,
	}
	for k, v := range runtime.Tracer.GetTelemetry() {
		tracerStats[k] = v
	}

	stateStats := t.state.GetStats()
	conntrackStats := t.conntracker.GetStats()

	ret := map[string]interface{}{
		"conntrack": conntrackStats,
		"state":     stateStats["telemetry"],
		"tracer":    tracerStats,
		"ebpf":      t.getEbpfTelemetry(),
		"kprobes":   ddebpf.GetProbeStats(),
		"dns":       t.reverseDNS.GetStats(),
	}

	return ret, nil
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
	latestConns, _, err := t.getConnections(make([]network.ConnectionStats, 0))
	if err != nil {
		return nil, fmt.Errorf("error retrieving connections: %s", err)
	}
	return &network.Connections{Conns: latestConns}, nil
}

func (t *Tracer) getProbeProgramIDs() (map[string]uint32, error) {
	fds := make(map[string]uint32, 0)
	for _, p := range t.m.Probes {
		if !p.Enabled {
			continue
		}
		prog := p.Program()
		if prog == nil {
			log.Debugf("unable to find program for %s\n", p.Section)
			continue
		}
		id, err := prog.ID()
		if err != nil {
			return nil, err
		}
		fds[p.Section] = uint32(id)
	}
	return fds, nil
}

// connectionExpired returns true if the passed in connection has expired
//
// expiry is handled differently for UDP and TCP. For TCP where conntrack TTL is very long, we use a short expiry for userspace tracking
// but use conntrack as a source of truth to keep long lived idle TCP conns in the userspace state, while evicting closed TCP connections.
// for UDP, the conntrack TTL is lower (two minutes), so the userspace and conntrack expiry are synced to avoid touching conntrack for
// UDP expiries
func (t *Tracer) connectionExpired(conn *ConnTuple, latestTime uint64, stats *ConnStatsWithTimestamp, ctr *cachedConntrack) bool {
	if !stats.isExpired(latestTime, t.timeoutForConn(conn, stats.isAssured())) {
		return false
	}

	// skip connection check for udp connections or if
	// the pid for the connection is dead
	if conn.isUDP() || !procutil.PidExists(int(conn.Pid())) {
		return true
	}

	exists, err := ctr.Exists(conn)
	if err != nil {
		log.Warnf("error checking conntrack for connection %s: %s", conn, err)
	}
	if !exists {
		exists, err = ctr.ExistsInRootNS(conn)
		if err != nil {
			log.Warnf("error checking conntrack for connection in root ns %s: %s", conn, err)
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

func newHTTPMonitor(supported bool, c *config.Config) *http.Monitor {
	if !c.EnableHTTPMonitoring {
		return nil
	}

	if !supported {
		log.Warnf("http monitoring is not supported by this kernel version. please refer to system-probe's documentation")
		return nil
	}

	monitor, err := http.NewMonitor(c)
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
