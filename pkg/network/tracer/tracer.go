// +build linux_bpf

package tracer

import (
	"expvar"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"golang.org/x/sys/unix"
)

var (
	expvarEndpoints map[string]*expvar.Map
	expvarTypes     = []string{"conntrack", "state", "tracer", "ebpf", "kprobes", "dns"}
)

func init() {
	expvarEndpoints = make(map[string]*expvar.Map, len(expvarTypes))
	for _, name := range expvarTypes {
		expvarEndpoints[name] = expvar.NewMap(name)
	}
}

type Tracer struct {
	m *manager.Manager

	config *config.Config

	state          network.State
	portMapping    *network.PortMapping
	udpPortMapping *network.PortMapping

	conntracker netlink.Conntracker

	reverseDNS network.ReverseDNS

	httpMonitor *http.Monitor

	perfMap      *manager.PerfMap
	perfHandler  *ddebpf.PerfHandler
	batchManager *PerfBatchManager
	flushIdle    chan chan struct{}
	stop         chan struct{}

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
	expiredTCPConns int64
	closedConns     int64

	buffer     []network.ConnectionStats
	bufferLock sync.Mutex

	// Internal buffer used to compute bytekeys
	buf []byte

	// Connections for the tracer to blacklist
	sourceExcludes []*network.ConnectionFilter
	destExcludes   []*network.ConnectionFilter
}

const (
	defaultClosedChannelSize = 500
)

func NewTracer(config *config.Config) (*Tracer, error) {
	// make sure debugfs is mounted
	if mounted, err := kernel.IsDebugFSMounted(); !mounted {
		return nil, fmt.Errorf("%s: %s", "system-probe unsupported", err)
	}

	buf, err := netebpf.ReadBPFModule(config.BPFDir, config.BPFDebug)
	if err != nil {
		return nil, fmt.Errorf("could not read bpf module: %s", err)
	}
	offsetBuf, err := netebpf.ReadOffsetBPFModule(config.BPFDir, config.BPFDebug)
	if err != nil {
		return nil, fmt.Errorf("could not read offset bpf module: %s", err)
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
			string(probes.HttpInFlightMap):    {Type: ebpf.Hash, MaxEntries: uint32(config.MaxTrackedConnections), EditorFlag: manager.EditMaxEntries},
		},
	}
	mgrOptions.ConstantEditors, err = runOffsetGuessing(config, offsetBuf)
	if err != nil {
		return nil, fmt.Errorf("error guessing offsets: %s", err)
	}

	closedChannelSize := defaultClosedChannelSize
	if config.ClosedChannelSize > 0 {
		closedChannelSize = config.ClosedChannelSize
	}
	perfHandlerTCP := ddebpf.NewPerfHandler(closedChannelSize)
	perfHandlerHTTP := ddebpf.NewPerfHandler(closedChannelSize)
	m := netebpf.NewManager(perfHandlerTCP, perfHandlerHTTP)

	// Use the config to determine what kernel probes should be enabled
	enabledProbes, err := config.EnabledProbes(pre410Kernel)
	if err != nil {
		return nil, fmt.Errorf("invalid probe configuration: %v", err)
	}

	enableSocketFilter := config.DNSInspection && !pre410Kernel
	if enableSocketFilter {
		enabledProbes[probes.SocketDnsFilter] = struct{}{}
		if config.CollectDNSStats {
			mgrOptions.ConstantEditors = append(mgrOptions.ConstantEditors, manager.ConstantEditor{
				Name:  "dns_stats_enabled",
				Value: uint64(1),
			})
		}
	}

	if config.EnableHTTPMonitoring && !pre410Kernel {
		enabledProbes[probes.SocketHTTPFilter] = struct{}{}
	}

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
	portMapping := network.NewPortMapping(config.ProcRoot, config.CollectTCPConns, config.CollectIPv6Conns)
	udpPortMapping := network.NewPortMapping(config.ProcRoot, config.CollectTCPConns, config.CollectIPv6Conns)
	if err := portMapping.ReadInitialState(); err != nil {
		return nil, fmt.Errorf("failed to read initial TCP pid->port mapping: %s", err)
	}

	if err := udpPortMapping.ReadInitialUDPState(); err != nil {
		return nil, fmt.Errorf("failed to read initial UDP pid->port mapping: %s", err)
	}

	conntracker := netlink.NewNoOpConntracker()
	if config.EnableConntrack {
		if c, err := netlink.NewConntracker(config.ProcRoot, config.ConntrackMaxStateSize, config.ConntrackRateLimit, config.EnableConntrackAllNamespaces); err != nil {
			log.Warnf("could not initialize conntrack, tracer will continue without NAT tracking: %s", err)
		} else {
			conntracker = c
		}
	}

	state := network.NewState(
		config.ClientStateExpiry,
		config.MaxClosedConnectionsBuffered,
		config.MaxConnectionsStateBuffered,
		config.MaxDNSStatsBufferred,
		config.CollectDNSDomains,
	)

	tr := &Tracer{
		m:              m,
		config:         config,
		state:          state,
		portMapping:    portMapping,
		udpPortMapping: udpPortMapping,
		reverseDNS:     reverseDNS,
		httpMonitor:    newHTTPMonitor(!pre410Kernel, config, m, perfHandlerHTTP),
		buffer:         make([]network.ConnectionStats, 0, 512),
		conntracker:    conntracker,
		sourceExcludes: network.ParseConnectionFilters(config.ExcludedSourceConnections),
		destExcludes:   network.ParseConnectionFilters(config.ExcludedDestinationConnections),
		perfHandler:    perfHandlerTCP,
		flushIdle:      make(chan chan struct{}),
		stop:           make(chan struct{}),
		buf:            make([]byte, network.ConnectionByteKeyMaxLen),
	}

	tr.perfMap, tr.batchManager, err = tr.initPerfPolling(perfHandlerTCP)
	if err != nil {
		return nil, fmt.Errorf("could not start polling bpf events: %s", err)
	}

	if err := tr.httpMonitor.Start(); err != nil {
		log.Errorf("failed to initialize http monitor: %s", err)
	}

	if err = m.Start(); err != nil {
		return nil, fmt.Errorf("could not start ebpf manager: %s", err)
	}

	go tr.expvarStats()

	return tr, nil
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
		packetSrc network.PacketSource
		srcErr    error
	)
	err := util.WithRootNS(cfg.ProcRoot, func() error {
		packetSrc, srcErr = network.NewPacketSource(filter)
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
	enabledProbes := offsetGuessProbes(config)
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

func (t *Tracer) expvarStats() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// trigger first get immediately
	_ = t.populateExpvarStats()
	for {
		select {
		case <-t.stop:
			return
		case <-ticker.C:
			_ = t.populateExpvarStats()
		}
	}
}

func (t *Tracer) populateExpvarStats() error {
	stats, err := t.getTelemetry()
	if err != nil {
		return err
	}

	for name, stat := range stats {
		for metric, val := range stat.(map[string]int64) {
			currVal := &expvar.Int{}
			currVal.Set(val)
			expvarEndpoints[name].Set(snakeToCapInitialCamel(metric), currVal)
		}
	}
	return nil
}

// initPerfPolling starts the listening on perf buffer events to grab closed connections
func (t *Tracer) initPerfPolling(perf *ddebpf.PerfHandler) (*manager.PerfMap, *PerfBatchManager, error) {
	pm, found := t.m.GetPerfMap(string(probes.TcpCloseEventMap))
	if !found {
		return nil, nil, fmt.Errorf("unable to find perf map %s", probes.TcpCloseEventMap)
	}

	tcpCloseEventMap, _ := t.getMap(probes.TcpCloseEventMap)
	tcpCloseMap, _ := t.getMap(probes.TcpCloseBatchMap)
	numCPUs := int(tcpCloseEventMap.ABI().MaxEntries)
	batchManager, err := NewPerfBatchManager(tcpCloseMap, t.config.TCPClosedTimeout, numCPUs)
	if err != nil {
		return nil, nil, err
	}

	if err := pm.Start(); err != nil {
		return nil, nil, fmt.Errorf("error starting perf map: %s", err)
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

				batch := toBatch(batchData)
				conns := t.batchManager.Extract(batch, time.Now())
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
				idleConns := t.batchManager.GetIdleConns(time.Now())
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

	return pm, batchManager, nil
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
	cs.Direction = t.determineConnectionDirection(cs)
	if t.shouldSkipConnection(cs) {
		atomic.AddInt64(&t.skippedConns, 1)
		return
	}

	atomic.AddInt64(&t.closedConns, 1)
	cs.IPTranslation = t.conntracker.GetTranslationForConn(*cs)
	t.state.StoreClosedConnection(cs)
	if cs.IPTranslation != nil {
		t.conntracker.DeleteTranslation(*cs)
	}
}

func (t *Tracer) Stop() {
	close(t.stop)
	t.reverseDNS.Close()
	_ = t.m.Stop(manager.CleanAll)
	_ = t.perfMap.Stop(manager.CleanAll)
	t.perfHandler.Stop()
	t.httpMonitor.Stop()
	close(t.flushIdle)
	t.conntracker.Close()
}

func (t *Tracer) GetActiveConnections(clientID string) (*network.Connections, error) {
	t.bufferLock.Lock()
	defer t.bufferLock.Unlock()

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

	conns := t.state.Connections(clientID, latestTime, latestConns, t.reverseDNS.GetDNSStats())
	names := t.reverseDNS.Resolve(conns)
	tm := t.getConnTelemetry(len(latestConns))

	return &network.Connections{Conns: conns, DNS: names, Telemetry: tm}, nil
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

	ebpfStats := t.getEbpfTelemetry()
	if usp, ok := ebpfStats["udp_sends_processed"]; ok {
		tm.MonotonicUDPSendsProcessed = usp
	}
	if usm, ok := ebpfStats["udp_sends_missed"]; ok {
		tm.MonotonicUDPSendsMissed = usm
	}

	return tm
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

	portMp, err := t.getMap(probes.PortBindingsMap)
	if err != nil {
		return nil, 0, fmt.Errorf("error retrieving the bpf %s map: %s", probes.PortBindingsMap, err)
	}

	udpPortMp, err := t.getMap(probes.UdpPortBindingsMap)
	if err != nil {
		return nil, 0, fmt.Errorf("error retrieving the bpf %s map: %s", probes.UdpPortBindingsMap, err)
	}

	latestTime, ok, err := t.getLatestTimestamp()
	if err != nil {
		return nil, 0, fmt.Errorf("error retrieving latest timestamp: %s", err)
	}

	if !ok { // if no timestamps have been captured, there can be no packets
		return nil, 0, nil
	}

	closedPortBindings, err := t.populatePortMapping(portMp, t.portMapping)
	if err != nil {
		return nil, 0, fmt.Errorf("error populating port mapping: %s", err)
	}

	closedUDPPortBindings, err := t.populatePortMapping(udpPortMp, t.udpPortMapping)
	if err != nil {
		return nil, 0, fmt.Errorf("error populating UDP port mapping: %s", err)
	}

	cachedConntrack := newCachedConntrack(t.config.ProcRoot, netlink.NewConntrack, 128)
	defer cachedConntrack.Close()

	// Iterate through all key-value pairs in map
	key, stats := &ConnTuple{}, &ConnStatsWithTimestamp{}
	seen := make(map[ConnTuple]struct{})
	var expired []*ConnTuple
	entries := mp.IterateFrom(unsafe.Pointer(&ConnTuple{}))
	for entries.Next(unsafe.Pointer(key), unsafe.Pointer(stats)) {

		// expiry is handled differently for UDP and TCP. For TCP where conntrack TTL is very long, we use a short expiry for userspace tracking
		// but use conntrack as a source of truth to keep long lived idle TCP conns in the userspace state, while evicting closed TCP connections.
		// for UDP, the conntrack TTL is lower (two minutes), so the userspace and conntrack expiry are synced to avoid touching conntrack for
		// UDP expiries
		if stats.isExpired(latestTime, t.timeoutForConn(key)) && (key.isUDP() || !t.conntrackExists(cachedConntrack, key)) {
			expired = append(expired, key.copy())
			if key.isTCP() {
				atomic.AddInt64(&t.expiredTCPConns, 1)
			}
			atomic.AddInt64(&t.closedConns, 1)
		} else {
			conn := connStats(key, stats, t.getTCPStats(tcpMp, key, seen))
			conn.Direction = t.determineConnectionDirection(&conn)

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

	// Remove expired entries
	t.removeEntries(mp, tcpMp, expired)

	// check for expired clients in the state
	t.state.RemoveExpiredClients(time.Now())

	for _, key := range closedPortBindings {
		port := uint16(key.port)
		t.portMapping.RemoveMapping(uint64(key.net_ns), port)
		_ = portMp.Delete(unsafe.Pointer(&key))
	}

	for _, key := range closedUDPPortBindings {
		port := uint16(key.port)
		t.udpPortMapping.RemoveMapping(uint64(key.net_ns), port)
		_ = udpPortMp.Delete(unsafe.Pointer(&key))
	}

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

	log.Debugf("Removed %d entries in %s", len(keys), time.Now().Sub(now))
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
		"tcp_sent_miscounts":  int64(telemetry.tcp_sent_miscounts),
		"missed_tcp_close":    int64(telemetry.missed_tcp_close),
		"udp_sends_processed": int64(telemetry.udp_sends_processed),
		"udp_sends_missed":    int64(telemetry.udp_sends_missed),
	}
}

func (t *Tracer) getMap(name probes.BPFMapName) (*ebpf.Map, error) {
	mp, _, err := t.m.GetMap(string(name))
	if mp == nil {
		return nil, fmt.Errorf("no map with name %s: %s", name, err)
	}
	return mp, nil
}

func (t *Tracer) timeoutForConn(c *ConnTuple) uint64 {
	if c.isTCP() {
		return uint64(t.config.TCPConnTimeout.Nanoseconds())
	}
	return uint64(t.config.UDPConnTimeout.Nanoseconds())
}

// getTelemetry calls GetStats and extract telemetry from the state structure
func (t *Tracer) getTelemetry() (map[string]interface{}, error) {
	stats, err := t.GetStats()
	if err != nil {
		return nil, err
	}

	if states, ok := stats["state"]; ok {
		if telemetry, ok := states.(map[string]interface{})["telemetry"]; ok {
			stats["state"] = telemetry
		}
	}
	return stats, nil
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

	stateStats := t.state.GetStats()
	conntrackStats := t.conntracker.GetStats()

	return map[string]interface{}{
		"conntrack": conntrackStats,
		"state":     stateStats,
		"tracer": map[string]int64{
			"closed_conn_polling_lost":     lost,
			"closed_conn_polling_received": received,
			"conn_valid_skipped":           skipped, // Skipped connections (e.g. Local DNS requests)
			"expired_tcp_conns":            expiredTCP,
			"pid_collisions":               pidCollisions,
		},
		"ebpf":    t.getEbpfTelemetry(),
		"kprobes": ddebpf.GetProbeStats(),
		"dns":     t.reverseDNS.GetStats(),
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
	latestConns, _, err := t.getConnections(make([]network.ConnectionStats, 0))
	if err != nil {
		return nil, fmt.Errorf("error retrieving connections: %s", err)
	}
	return &network.Connections{Conns: latestConns}, nil
}

// populatePortMapping reads an entire portBinding bpf map and populates the userspace  port map.  A list of
// closed ports will be returned.
// the map will be one of port_bindings  or udp_port_bindings, and the mapping will be one of tracer#portMapping
// tracer#udpPortMapping respectively.
func (t *Tracer) populatePortMapping(mp *ebpf.Map, mapping *network.PortMapping) (closed []portBindingTuple, err error) {
	var key, emptyKey portBindingTuple
	var state uint8

	entries := mp.IterateFrom(unsafe.Pointer(&emptyKey))
	for entries.Next(unsafe.Pointer(&key), unsafe.Pointer(&state)) {
		log.Tracef("port mapping added port=%d net_ns=%d", key.port, key.net_ns)
		mapping.AddMapping(uint64(key.net_ns), uint16(key.port))
		if isPortClosed(state) {
			log.Tracef("port mapping closed port=%d net_ns=%d", key.port, key.net_ns)
			closed = append(closed, key)
		}
	}

	return closed, nil
}

func (t *Tracer) determineConnectionDirection(conn *network.ConnectionStats) network.ConnectionDirection {
	pm := t.portMapping
	netNs := uint64(conn.NetNS)
	if conn.Type == network.UDP {
		pm = t.udpPortMapping
		netNs = 0 // namespace is always 0 for udp since we can't get namespace info from ebpf
	}
	if pm.IsListening(netNs, conn.SPort) {
		return network.INCOMING
	}
	return network.OUTGOING
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

func (t *Tracer) conntrackExists(ctr *cachedConntrack, conn *ConnTuple) bool {
	ok, err := ctr.Exists(conn)
	if err != nil {
		log.Errorf("error checking conntrack for connection %+v", *conn)
	}

	return ok
}

func newHTTPMonitor(supported bool, c *config.Config, m *manager.Manager, h *ddebpf.PerfHandler) *http.Monitor {
	if !c.EnableHTTPMonitoring {
		return nil
	}

	if !supported {
		log.Warnf("http monitoring is not supported by this kernel version. please refer to system-probe's documentation")
		return nil
	}

	monitor, err := http.NewMonitor(c.ProcRoot, m, h)
	if err != nil {
		log.Errorf("could not enable http monitoring: %s", err)
		return nil
	}

	log.Info("http monitoring enabled")
	return monitor
}
