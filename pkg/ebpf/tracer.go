// +build linux_bpf

package ebpf

import (
	"bytes"
	"expvar"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	bpflib "github.com/iovisor/gobpf/elf"
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
	m *bpflib.Module

	config *Config

	state          network.State
	portMapping    *network.PortMapping
	udpPortMapping *network.PortMapping

	conntracker netlink.Conntracker

	reverseDNS network.ReverseDNS

	perfMap      *bpflib.PerfMap
	batchManager *PerfBatchManager

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
	buf *bytes.Buffer

	// Connections for the tracer to blacklist
	sourceExcludes []*network.ConnectionFilter
	destExcludes   []*network.ConnectionFilter
}

const (
	// maxActive configures the maximum number of instances of the kretprobe-probed functions handled simultaneously.
	// This value should be enough for typical workloads (e.g. some amount of processes blocked on the accept syscall).
	maxActive                = 128
	defaultClosedChannelSize = 500
)

func NewTracer(config *Config) (*Tracer, error) {
	// make sure debugfs is mounted
	if mounted, msg := util.IsDebugfsMounted(); !mounted {
		return nil, fmt.Errorf("%s: %s", "system-probe unsupported", msg)
	}

	m, err := bytecode.ReadBPFModule(config.BPFDebug)
	if err != nil {
		return nil, fmt.Errorf("could not read bpf module: %s", err)
	}

	// check if current platform is using old kernel API because it affects what kprobe are we going to enable
	currKernelVersion, err := CurrentKernelVersion()
	if err != nil {
		// if the platform couldn't be determined, treat it as new kernel case
		log.Warn("could not detect the platform, will use kprobes from kernel version >= 4.1.0")
	}

	// check to see if current kernel is earlier than version 4.1.0
	pre410Kernel := isPre410Kernel(currKernelVersion)
	if pre410Kernel {
		log.Infof("detected platform %s, switch to use kprobes from kernel version < 4.1.0", kernelCodeToString(currKernelVersion))
	}

	enableSocketFilter := config.DNSInspection && !pre410Kernel
	err = m.Load(SectionsFromConfig(config, enableSocketFilter))
	if err != nil {
		return nil, fmt.Errorf("could not load bpf module: %s", err)
	}

	// Enable kernel probes used for offset guessing.
	// TODO: Disable them once offsets have been figured out.
	for _, probeName := range offsetGuessProbes(config) {
		if err := m.EnableKprobe(string(probeName), maxActive); err != nil {
			return nil, fmt.Errorf(
				"could not enable kprobe(%s) used for offset guessing: %s",
				probeName,
				err,
			)
		}
	}

	start := time.Now()
	if err := guessOffsets(m, config); err != nil {
		return nil, fmt.Errorf("failed to init module: error guessing offsets: %v", err)
	}
	log.Infof("socket struct offset guessing complete (took %v)", time.Since(start))

	// Use the config to determine what kernel probes should be enabled
	enabledProbes := config.EnabledKProbes(pre410Kernel)

	prefix, err := getSyscallPrefix()
	if err != nil {
		return nil, fmt.Errorf("cannot get syscall prefix: %v", err)
	}

	for k := range m.IterKprobes() {
		probeName := KProbeName(k.Name)
		if _, ok := enabledProbes[probeName]; ok {
			// check if we should override kprobe name
			if override, ok := kprobeOverrides[probeName]; ok {
				if err = m.SetKprobeForSection(string(probeName), string(override)); err != nil {
					return nil, fmt.Errorf("could not update kprobe \"%s\" to \"%s\" : %s", k.Name, string(override), err)
				}
			}

			if isSysCall(probeName) {
				fixedName := fixSyscallName(prefix, probeName)
				log.Debugf("attaching section %s to %s", string(probeName), fixedName)
				if err = m.SetKprobeForSection(string(probeName), fixedName); err != nil {
					return nil, fmt.Errorf("could not update kprobe \"%s\" to \"%s\" : %s", k.Name, fixedName, err)
				}
			}

			if err = m.EnableKprobe(string(probeName), maxActive); err != nil {
				return nil, fmt.Errorf("could not enable kprobe(%s): %s", k.Name, err)
			}
		}
	}

	var reverseDNS network.ReverseDNS = network.NewNullReverseDNS()
	if enableSocketFilter {

		if config.CollectDNSStats {
			mp := m.Map(string(configMap))
			if mp == nil {
				return nil, fmt.Errorf("error retrieving the bpf %s map: %s", configMap, err)
			}
			if err := m.UpdateElement(mp, unsafe.Pointer(&zero), unsafe.Pointer(&zero), 0); err != nil {
				return nil, fmt.Errorf("error updating element of bpf %s map: %s", configMap, err)
			}
		}

		filter := m.SocketFilter("socket/dns_filter")
		if filter == nil {
			return nil, fmt.Errorf("error retrieving socket filter")
		}

		if snooper, err := network.NewSocketFilterSnooper(
			config.ProcRoot,
			filter,
			config.CollectDNSStats,
			config.CollectLocalDNS,
			config.DNSTimeout,
		); err == nil {
			reverseDNS = snooper
		} else {
			return nil, fmt.Errorf("error enabling DNS traffic inspection: %s", err)
		}
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
		if c, err := netlink.NewConntracker(config.ProcRoot, config.ConntrackMaxStateSize, config.ConntrackRateLimit); err != nil {
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
	)

	tr := &Tracer{
		m:              m,
		config:         config,
		state:          state,
		portMapping:    portMapping,
		udpPortMapping: udpPortMapping,
		reverseDNS:     reverseDNS,
		buffer:         make([]network.ConnectionStats, 0, 512),
		buf:            &bytes.Buffer{},
		conntracker:    conntracker,
		sourceExcludes: network.ParseConnectionFilters(config.ExcludedSourceConnections),
		destExcludes:   network.ParseConnectionFilters(config.ExcludedDestinationConnections),
	}

	tcpCloseMap, _ := tr.getMap(tcpCloseBatchMap)
	batchManager, err := NewPerfBatchManager(m, tcpCloseMap, config.TCPClosedTimeout)
	if err != nil {
		return nil, err
	}
	tr.batchManager = batchManager

	tr.perfMap, err = tr.initPerfPolling()
	if err != nil {
		return nil, fmt.Errorf("could not start polling bpf events: %s", err)
	}

	go tr.expvarStats()

	return tr, nil
}

func (t *Tracer) expvarStats() {
	ticker := time.NewTicker(5 * time.Second)
	// starts running the body immediately instead waiting for the first tick
	for ; true; <-ticker.C {
		stats, err := t.getTelemetry()
		if err != nil {
			continue
		}

		for name, stat := range stats {
			for metric, val := range stat.(map[string]int64) {
				currVal := &expvar.Int{}
				currVal.Set(val)
				expvarEndpoints[name].Set(snakeToCapInitialCamel(metric), currVal)
			}
		}
	}
}

// initPerfPolling starts the listening on perf buffer events to grab closed connections
func (t *Tracer) initPerfPolling() (*bpflib.PerfMap, error) {
	closedChannelSize := defaultClosedChannelSize
	if t.config.ClosedChannelSize > 0 {
		closedChannelSize = t.config.ClosedChannelSize
	}
	closedChannel := make(chan []byte, closedChannelSize)
	lostChannel := make(chan uint64, 10)

	pm, err := bpflib.InitPerfMap(t.m, string(tcpCloseEventMap), closedChannel, lostChannel)
	if err != nil {
		return nil, fmt.Errorf("error initializing perf map: %s", err)
	}

	pm.PollStart()

	go func() {
		// Stats about how much connections have been closed / lost
		ticker := time.NewTicker(5 * time.Minute)
		flushIdle := time.NewTicker(t.config.TCPClosedTimeout)

		for {
			select {
			case batchData, ok := <-closedChannel:
				if !ok {
					return
				}
				atomic.AddInt64(&t.perfReceived, 1)

				batch := toBatch(batchData)
				conns := t.batchManager.Extract(batch, time.Now())
				for _, c := range conns {
					t.storeClosedConn(c)
				}
			case lostCount, ok := <-lostChannel:
				if !ok {
					return
				}
				atomic.AddInt64(&t.perfLost, int64(lostCount))
			case now := <-flushIdle.C:
				idleConns := t.batchManager.GetIdleConns(now)
				for _, c := range idleConns {
					t.storeClosedConn(c)
				}
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

	return pm, nil
}

// shouldSkipConnection returns whether or not the tracer should ignore a given connection:
//  • Local DNS (*:53) requests if configured (default: true)
func (t *Tracer) shouldSkipConnection(conn *network.ConnectionStats) bool {
	isDNSConnection := conn.DPort == 53 || conn.SPort == 53
	if !t.config.CollectLocalDNS && isDNSConnection && conn.Dest.IsLoopback() {
		return true
	} else if network.IsBlacklistedConnection(t.sourceExcludes, t.destExcludes, conn) {
		return true
	}
	return false
}

func (t *Tracer) storeClosedConn(cs network.ConnectionStats) {
	cs.Direction = t.determineConnectionDirection(&cs)
	if t.shouldSkipConnection(&cs) {
		atomic.AddInt64(&t.skippedConns, 1)
		return
	}

	atomic.AddInt64(&t.closedConns, 1)
	cs.IPTranslation = t.conntracker.GetTranslationForConn(cs)
	t.state.StoreClosedConnection(cs)
	if cs.IPTranslation != nil {
		t.conntracker.DeleteTranslation(cs)
	}
}

func (t *Tracer) Stop() {
	t.reverseDNS.Close()
	_ = t.m.Close()
	t.perfMap.PollStop()
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

	conns := t.state.Connections(clientID, latestTime, latestConns, t.reverseDNS.GetDNSStats())
	names := t.reverseDNS.Resolve(conns)
	tm := t.getConnTelemetry(len(latestConns))

	return &network.Connections{Conns: conns, DNS: names, Telemetry: tm}, nil
}

func (t *Tracer) getConnTelemetry(mapSize int) *network.ConnectionsTelemetry {
	kprobeStats := getProbeTotals()
	tm := &network.ConnectionsTelemetry{
		MonotonicKprobesTriggered: kprobeStats.hits,
		MonotonicKprobesMissed:    kprobeStats.miss,
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

	dnsStats := t.reverseDNS.GetStats()
	if pp, ok := dnsStats["packets_processed"]; ok {
		tm.MonotonicDNSPacketsProcessed = pp
	}
	return tm
}

// getConnections returns all of the active connections in the ebpf maps along with the latest timestamp.  It takes
// a reusable buffer for appending the active connections so that this doesn't continuously allocate
func (t *Tracer) getConnections(active []network.ConnectionStats) ([]network.ConnectionStats, uint64, error) {
	mp, err := t.getMap(connMap)
	if err != nil {
		return nil, 0, fmt.Errorf("error retrieving the bpf %s map: %s", connMap, err)
	}

	tcpMp, err := t.getMap(tcpStatsMap)
	if err != nil {
		return nil, 0, fmt.Errorf("error retrieving the bpf %s map: %s", tcpStatsMap, err)
	}

	portMp, err := t.getMap(portBindingsMap)
	if err != nil {
		return nil, 0, fmt.Errorf("error retrieving the bpf %s map: %s", portBindingsMap, err)
	}

	udpPortMp, err := t.getMap(udpPortBindingsMap)
	if err != nil {
		return nil, 0, fmt.Errorf("error retrieving the bpf %s map: %s", udpPortBindingsMap, err)
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

	// Iterate through all key-value pairs in map
	key, nextKey, stats := &ConnTuple{}, &ConnTuple{}, &ConnStatsWithTimestamp{}
	seen := make(map[ConnTuple]struct{})
	var expired []*ConnTuple
	for {
		hasNext, _ := t.m.LookupNextElement(mp, unsafe.Pointer(key), unsafe.Pointer(nextKey), unsafe.Pointer(stats))
		if !hasNext {
			break
		} else if stats.isExpired(latestTime, t.timeoutForConn(nextKey)) {
			expired = append(expired, nextKey.copy())
			if nextKey.isTCP() {
				atomic.AddInt64(&t.expiredTCPConns, 1)
			}
			atomic.AddInt64(&t.closedConns, 1)
		} else {
			conn := connStats(nextKey, stats, t.getTCPStats(tcpMp, nextKey, seen))
			conn.Direction = t.determineConnectionDirection(&conn)

			if t.shouldSkipConnection(&conn) {
				atomic.AddInt64(&t.skippedConns, 1)
			} else {
				// lookup conntrack in for active
				conn.IPTranslation = t.conntracker.GetTranslationForConn(conn)
				active = append(active, conn)
			}
		}
		key = nextKey
	}

	// Remove expired entries
	t.removeEntries(mp, tcpMp, expired)

	// check for expired clients in the state
	t.state.RemoveExpiredClients(time.Now())

	for _, key := range closedPortBindings {
		t.portMapping.RemoveMapping(key)
		_ = t.m.DeleteElement(portMp, unsafe.Pointer(&key))
	}

	for _, key := range closedUDPPortBindings {
		t.udpPortMapping.RemoveMapping(key)
		_ = t.m.DeleteElement(udpPortMp, unsafe.Pointer(&key))
	}

	// Get the latest time a second time because it could have changed while we were reading the eBPF map
	latestTime, _, err = t.getLatestTimestamp()
	if err != nil {
		return nil, 0, fmt.Errorf("error retrieving latest timestamp: %s", err)
	}

	return active, latestTime, nil
}

func (t *Tracer) removeEntries(mp, tcpMp *bpflib.Map, entries []*ConnTuple) {
	now := time.Now()
	// Byte keys of the connections to remove
	keys := make([]string, 0, len(entries))
	// Used to create the keys
	statsWithTs, tcpStats := &ConnStatsWithTimestamp{}, &TCPStats{}
	// Remove the entries from the eBPF Map
	for i := range entries {
		err := t.m.DeleteElement(mp, unsafe.Pointer(entries[i]))
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
		_ = t.m.DeleteElement(tcpMp, unsafe.Pointer(entries[i]))
	}

	t.state.RemoveConnections(keys)

	log.Debugf("Removed %d entries in %s", len(keys), time.Now().Sub(now))
}

// getTCPStats reads tcp related stats for the given ConnTuple
func (t *Tracer) getTCPStats(mp *bpflib.Map, tuple *ConnTuple, seen map[ConnTuple]struct{}) *TCPStats {
	stats := new(TCPStats)

	if !tuple.isTCP() {
		return stats
	}

	// The PID isn't used as a key in the stats map, we will temporarily set it to 0 here and reset it when we're done
	pid := tuple.pid
	tuple.pid = 0

	_ = t.m.LookupElement(mp, unsafe.Pointer(tuple), unsafe.Pointer(stats))

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

// getLatestTimestamp reads the most recent timestamp captured by the eBPF
// module.  if the eBFP module has not yet captured a timestamp (as will be the
// case if the eBPF module has just started), the second return value will be
// false.
func (t *Tracer) getLatestTimestamp() (uint64, bool, error) {
	tsMp, err := t.getMap(latestTimestampMap)
	if err != nil {
		return 0, false, fmt.Errorf("error retrieving latest timestamp map: %s", err)
	}

	var latestTime uint64
	if err := t.m.LookupElement(tsMp, unsafe.Pointer(&zero), unsafe.Pointer(&latestTime)); err != nil {
		// If we can't find latest timestamp, there probably haven't been any messages yet
		return 0, false, nil
	}

	return latestTime, true, nil
}

// getEbpfTelemetry reads the telemetry map from the kernelspace and returns a map of key -> count
func (t *Tracer) getEbpfTelemetry() map[string]int64 {
	mp, err := t.getMap(telemetryMap)
	if err != nil {
		log.Warnf("error retrieving telemetry map: %s", err)
		return map[string]int64{}
	}

	telemetry := &kernelTelemetry{}
	if err := t.m.LookupElement(mp, unsafe.Pointer(&zero), unsafe.Pointer(telemetry)); err != nil {
		// This can happen if we haven't initialized the telemetry object yet
		// so let's just use a trace log
		log.Tracef("error retrieving the telemetry struct: %s", err)
	}

	return map[string]int64{
		"tcp_sent_miscounts": int64(telemetry.tcp_sent_miscounts),
		"missed_tcp_close":   int64(telemetry.missed_tcp_close),
	}
}

func (t *Tracer) getMap(name bpfMapName) (*bpflib.Map, error) {
	mp := t.m.Map(string(name))
	if mp == nil {
		return nil, fmt.Errorf("no map with name %s", name)
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
		"kprobes": GetProbeStats(),
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
func (t *Tracer) populatePortMapping(mp *bpflib.Map, mapping *network.PortMapping) ([]uint16, error) {
	var key, nextKey uint16
	var state uint8

	closedPortBindings := make([]uint16, 0)

	for {
		hasNext, _ := t.m.LookupNextElement(mp, unsafe.Pointer(&key), unsafe.Pointer(&nextKey), unsafe.Pointer(&state))
		if !hasNext {
			break
		}

		port := nextKey

		mapping.AddMapping(port)

		if isPortClosed(state) {
			closedPortBindings = append(closedPortBindings, port)
		}

		key = nextKey
	}

	return closedPortBindings, nil
}

func (t *Tracer) determineConnectionDirection(conn *network.ConnectionStats) network.ConnectionDirection {
	if conn.Type == network.UDP {
		if t.udpPortMapping.IsListening(conn.SPort) {
			return network.INCOMING
		}
		return network.OUTGOING
	}

	if t.portMapping.IsListening(conn.SPort) {
		return network.INCOMING
	}

	return network.OUTGOING
}

// SectionsFromConfig returns a map of string -> gobpf.SectionParams used to configure the way we load the BPF program (bpf map sizes)
func SectionsFromConfig(c *Config, enableSocketFilter bool) map[string]bpflib.SectionParams {
	return map[string]bpflib.SectionParams{
		connMap.sectionName(): {
			MapMaxEntries: int(c.MaxTrackedConnections),
		},
		tcpStatsMap.sectionName(): {
			MapMaxEntries: int(c.MaxTrackedConnections),
		},
		portBindingsMap.sectionName(): {
			MapMaxEntries: int(c.MaxTrackedConnections),
		},
		udpPortBindingsMap.sectionName(): {
			MapMaxEntries: int(c.MaxTrackedConnections),
		},
		tcpCloseEventMap.sectionName(): {
			MapMaxEntries: 1024,
		},
		"socket/dns_filter": {
			Disabled: !enableSocketFilter,
		},
	}
}

// CurrentKernelVersion exposes calculated kernel version - exposed in LINUX_VERSION_CODE format
// That is, for kernel "a.b.c", the version number will be (a<<16 + b<<8 + c)
func CurrentKernelVersion() (uint32, error) {
	return bpflib.CurrentKernelVersion()
}

func isSysCall(name KProbeName) bool {
	parts := strings.Split(string(name), "/")
	if len(parts) != 2 {
		return false
	}
	return strings.HasPrefix(parts[1], "sys_")
}
