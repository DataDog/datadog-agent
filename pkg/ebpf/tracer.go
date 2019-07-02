// +build linux_bpf

package ebpf

import (
	"bytes"
	"expvar"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/ebpf/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	bpflib "github.com/iovisor/gobpf/elf"
	"github.com/mailru/easyjson/buffer"
)

var (
	probeExpvar *expvar.Map
)

func init() {
	probeExpvar = expvar.NewMap("systemprobe")

	// Configure the easyjson buffer pool to allow for bigger buffers
	// A connection is ~ 200 B JSON encoded
	// so setting the max size to 524 KB should allow for buffers that could hold
	// up to ~ 2600 JSON encoded connections
	buffer.Init(buffer.PoolConfig{
		StartSize:  128,
		PooledSize: 512,
		MaxSize:    524288,
	})
}

type Tracer struct {
	m *bpflib.Module

	config *Config

	state          NetworkState
	portMapping    *PortMapping
	localAddresses map[util.Address]struct{}

	conntracker netlink.Conntracker

	perfMap *bpflib.PerfMap

	// Telemetry
	perfReceived int64
	perfLost     int64
	skippedConns int64
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

	buffer     []ConnectionStats
	bufferLock sync.Mutex

	// Internal buffer used to compute bytekeys
	buf *bytes.Buffer
}

// maxActive configures the maximum number of instances of the kretprobe-probed functions handled simultaneously.
// This value should be enough for typical workloads (e.g. some amount of processes blocked on the accept syscall).
const (
	maxActive = 128
)

// CurrentKernelVersion exposes calculated kernel version - exposed in LINUX_VERSION_CODE format
// That is, for kernel "a.b.c", the version number will be (a<<16 + b<<8 + c)
func CurrentKernelVersion() (uint32, error) {
	return bpflib.CurrentKernelVersion()
}

func NewTracer(config *Config) (*Tracer, error) {
	m, err := readBPFModule(config.BPFDebug)
	if err != nil {
		return nil, fmt.Errorf("could not read bpf module: %s", err)
	}

	err = m.Load(SectionsFromConfig(config))
	if err != nil {
		return nil, fmt.Errorf("could not load bpf module: %s", err)
	}

	// Use the config to determine what kernel probes should be enabled
	enabledProbes := config.EnabledKProbes()
	for k := range m.IterKprobes() {
		if _, ok := enabledProbes[KProbeName(k.Name)]; ok {
			if err = m.EnableKprobe(k.Name, maxActive); err != nil {
				return nil, fmt.Errorf("could not enable kprobe(%s): %s", k.Name, err)
			}
		}
	}

	// TODO: Disable TCPv{4,6} connect kernel probes once offsets have been figured out.
	if err := guess(m, config); err != nil {
		return nil, fmt.Errorf("failed to init module: error guessing offsets: %v", err)
	}

	portMapping := NewPortMapping(config.ProcRoot, config)
	if err := portMapping.ReadInitialState(); err != nil {
		return nil, fmt.Errorf("failed to read initial pid->port mapping: %s", err)
	}

	conntracker := netlink.NewNoOpConntracker()
	if config.EnableConntrack {
		if c, err := netlink.NewConntracker(config.ProcRoot, config.ConntrackShortTermBufferSize, int(config.MaxTrackedConnections)); err != nil {
			log.Warnf("could not initialize conntrack, tracer will continue without NAT tracking: %s", err)
		} else {
			conntracker = c
		}
	}

	state := NewNetworkState(config.ClientStateExpiry, config.MaxClosedConnectionsBuffered, config.MaxConnectionsStateBuffered)

	tr := &Tracer{
		m:              m,
		config:         config,
		state:          state,
		portMapping:    portMapping,
		localAddresses: readLocalAddresses(),
		buffer:         make([]ConnectionStats, 0, 512),
		buf:            &bytes.Buffer{},
		conntracker:    conntracker,
	}

	tr.perfMap, err = tr.initPerfPolling()
	if err != nil {
		return nil, fmt.Errorf("could not start polling bpf events: %s", err)
	}

	go tr.expvarStats()

	return tr, nil
}

// snakeToCapInitialCamel converts a snake case to Camel case with capital initial
func snakeToCapInitialCamel(s string) string {
	n := ""
	capNext := true
	for _, v := range s {
		if v >= 'A' && v <= 'Z' {
			n += string(v)
		}
		if v >= 'a' && v <= 'z' {
			if capNext {
				n += strings.ToUpper(string(v))
			} else {
				n += string(v)
			}
		}
		if v == '_' {
			capNext = true
		} else {
			capNext = false
		}
	}
	return n
}

func (t *Tracer) expvarStats() {
	ticker := time.NewTicker(5 * time.Second)
	// starts running the body immediately instead waiting for the first tick
	for ; true; <-ticker.C {
		stats, err := t.GetStats()
		if err != nil {
			continue
		}

		if tracerStats, ok := stats["tracer"]; ok {
			for metric, val := range tracerStats.(map[string]int64) {
				currVal := &expvar.Int{}
				currVal.Set(val)
				probeExpvar.Set(snakeToCapInitialCamel(metric), currVal)
			}
		}

		if probeStats, ok := stats["kprobes"]; ok {
			for metric, val := range probeStats.(map[string]int64) {
				currVal := &expvar.Int{}
				currVal.Set(val)
				probeExpvar.Set(fmt.Sprintf("Kprobe%s", snakeToCapInitialCamel(metric)), currVal)
			}
		}

		if ebpfStats, ok := stats["ebpf"]; ok {
			for metric, val := range ebpfStats.(map[string]int64) {
				currVal := &expvar.Int{}
				currVal.Set(val)
				probeExpvar.Set(fmt.Sprintf("Ebpf%s", snakeToCapInitialCamel(metric)), currVal)
			}
		}

		if states, ok := stats["state"]; ok {
			if telemetry, ok := states.(map[string]interface{})["telemetry"]; ok {
				for metric, val := range telemetry.(map[string]int64) {
					currVal := &expvar.Int{}
					currVal.Set(val)
					probeExpvar.Set(snakeToCapInitialCamel(metric), currVal)
				}
			}
		}

		if conntrackStats, ok := stats["conntrack"]; ok {
			for metric, val := range conntrackStats.(map[string]int64) {
				currVal := &expvar.Int{}
				currVal.Set(val)
				probeExpvar.Set(fmt.Sprintf("Conntrack%s", snakeToCapInitialCamel(metric)), currVal)
			}
		}
	}
}

// initPerfPolling starts the listening on perf buffer events to grab closed connections
func (t *Tracer) initPerfPolling() (*bpflib.PerfMap, error) {
	closedChannel := make(chan []byte, 100)
	lostChannel := make(chan uint64, 10)

	pm, err := bpflib.InitPerfMap(t.m, string(tcpCloseEventMap), closedChannel, lostChannel)
	if err != nil {
		return nil, fmt.Errorf("error initializing perf map: %s", err)
	}

	pm.PollStart()

	go func() {
		// Stats about how much connections have been closed / lost
		ticker := time.NewTicker(5 * time.Minute)

		for {
			select {
			case conn, ok := <-closedChannel:
				if !ok {
					log.Infof("Exiting closed connections polling")
					return
				}
				atomic.AddInt64(&t.perfReceived, 1)
				cs := decodeRawTCPConn(conn)
				cs.Direction = t.determineConnectionDirection(&cs)
				if t.shouldSkipConnection(&cs) {
					atomic.AddInt64(&t.skippedConns, 1)
				} else {
					cs.IPTranslation = t.conntracker.GetTranslationForConn(cs.SourceAddr(), cs.SPort)
					t.state.StoreClosedConnection(cs)
				}
			case lostCount, ok := <-lostChannel:
				if !ok {
					return
				}
				atomic.AddInt64(&t.perfLost, int64(lostCount))
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
func (t *Tracer) shouldSkipConnection(conn *ConnectionStats) bool {
	isDNSConnection := conn.DPort == 53 || conn.SPort == 53
	return !t.config.CollectLocalDNS && isDNSConnection && conn.Direction == LOCAL
}

func (t *Tracer) Stop() {
	_ = t.m.Close()
	t.perfMap.PollStop()
	t.conntracker.Close()
}

func (t *Tracer) GetActiveConnections(clientID string) (*Connections, error) {
	t.bufferLock.Lock()
	defer t.bufferLock.Unlock()

	latestConns, latestTime, err := t.getConnections(t.buffer[:0])
	if err != nil {
		return nil, fmt.Errorf("error retrieving connections: %s", err)
	}

	// Grow or shrink buffer depending on the usage
	if len(latestConns) >= cap(t.buffer)*2 {
		t.buffer = make([]ConnectionStats, 0, cap(t.buffer)*2)
	} else if len(latestConns) <= cap(t.buffer)/2 {
		t.buffer = make([]ConnectionStats, 0, cap(t.buffer)/2)
	}

	return &Connections{Conns: t.state.Connections(clientID, latestTime, latestConns)}, nil
}

// getConnections returns all of the active connections in the ebpf maps along with the latest timestamp.  It takes
// a reusable buffer for appending the active connections so that this doesn't continuously allocate
func (t *Tracer) getConnections(active []ConnectionStats) ([]ConnectionStats, uint64, error) {
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

	latestTime, ok, err := t.getLatestTimestamp()
	if err != nil {
		return nil, 0, fmt.Errorf("error retrieving latest timestamp: %s", err)
	}

	if !ok { // if no timestamps have been captured, there can be no packets
		return nil, 0, nil
	}

	closedPortBindings, err := t.populatePortMapping(portMp)
	if err != nil {
		return nil, 0, fmt.Errorf("error populating port mapping: %s", err)
	}

	// Iterate through all key-value pairs in map
	key, nextKey, stats := &ConnTuple{}, &ConnTuple{}, &ConnStatsWithTimestamp{}
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
		} else {
			conn := connStats(nextKey, stats, t.getTCPStats(tcpMp, nextKey))
			conn.Direction = t.determineConnectionDirection(&conn)

			if t.shouldSkipConnection(&conn) {
				atomic.AddInt64(&t.skippedConns, 1)
			} else {
				// lookup conntrack in for active
				conn.IPTranslation = t.conntracker.GetTranslationForConn(conn.SourceAddr(), conn.SPort)
				active = append(active, conn)
			}
		}
		key = nextKey
	}

	// Remove expired entries
	t.removeEntries(mp, tcpMp, expired)

	// check for expired clients in the state
	t.state.RemoveExpiredClients(time.Now())

	t.conntracker.ClearShortLived()

	for _, key := range closedPortBindings {
		t.portMapping.RemoveMapping(key)
		_ = t.m.DeleteElement(portMp, unsafe.Pointer(&key))
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
			// It's possible some other process deleted this entry already (e.g. tcp_close)
			_ = log.Warnf("failed to remove entry from connections map: %s", err)
		}

		// Append the connection key to the keys to remove from the userspace state
		bk, err := connStats(entries[i], statsWithTs, tcpStats).ByteKey(t.buf)
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
func (t *Tracer) getTCPStats(mp *bpflib.Map, tuple *ConnTuple) *TCPStats {
	// The PID isn't used as a key in the stats map, we will temporarily set it to 0 here and reset it when we're done
	pid := tuple.pid
	tuple.pid = 0

	stats := &TCPStats{retransmits: 0}

	// Don't bother looking in the map if the connection is UDP, there will never be data for that and we will avoid
	// the overhead of the syscall and creating the resultant error
	if tuple.isTCP() {
		_ = t.m.LookupElement(mp, unsafe.Pointer(tuple), unsafe.Pointer(stats))
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
	}
}

func (t *Tracer) getMap(name bpfMapName) (*bpflib.Map, error) {
	mp := t.m.Map(string(name))
	if mp == nil {
		return nil, fmt.Errorf("no map with name %s", name)
	}
	return mp, nil
}

func readBPFModule(debug bool) (*bpflib.Module, error) {
	file := "tracer-ebpf.o"
	if debug {
		file = "tracer-ebpf-debug.o"
	}

	buf, err := Asset(file)
	if err != nil {
		return nil, fmt.Errorf("couldn't find asset: %s", err)
	}

	m := bpflib.NewModuleFromReader(bytes.NewReader(buf))
	if m == nil {
		return nil, fmt.Errorf("BPF not supported")
	}
	return m, nil
}

func (t *Tracer) timeoutForConn(c *ConnTuple) uint64 {
	if c.isTCP() {
		return uint64(t.config.TCPConnTimeout.Nanoseconds())
	}
	return uint64(t.config.UDPConnTimeout.Nanoseconds())
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
		},
		"ebpf":    t.getEbpfTelemetry(),
		"kprobes": GetProbeStats(),
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
func (t *Tracer) DebugNetworkMaps() (*Connections, error) {
	latestConns, _, err := t.getConnections(make([]ConnectionStats, 0))
	if err != nil {
		return nil, fmt.Errorf("error retrieving connections: %s", err)
	}
	return &Connections{Conns: latestConns}, nil
}

// populatePortMapping reads the entire portBinding bpf map and populates the local port/address map.  A list of
// closed ports will be returned
func (t *Tracer) populatePortMapping(mp *bpflib.Map) ([]uint16, error) {
	var key, nextKey uint16
	var state uint8

	closedPortBindings := make([]uint16, 0)

	for {
		hasNext, _ := t.m.LookupNextElement(mp, unsafe.Pointer(&key), unsafe.Pointer(&nextKey), unsafe.Pointer(&state))
		if !hasNext {
			break
		}

		port := nextKey

		t.portMapping.AddMapping(port)

		if isPortClosed(state) {
			closedPortBindings = append(closedPortBindings, port)
		}

		key = nextKey
	}

	return closedPortBindings, nil
}

func (t *Tracer) determineConnectionDirection(conn *ConnectionStats) ConnectionDirection {
	sourceLocal := t.isLocalAddress(conn.SourceAddr())
	destLocal := t.isLocalAddress(conn.DestAddr())

	if sourceLocal && destLocal {
		return LOCAL
	}

	if sourceLocal && t.portMapping.IsListening(conn.SPort) {
		return INCOMING
	}

	return OUTGOING
}

func (t *Tracer) isLocalAddress(address util.Address) bool {
	_, ok := t.localAddresses[address]
	return ok
}

func readLocalAddresses() map[util.Address]struct{} {
	addresses := make(map[util.Address]struct{}, 0)

	interfaces, err := net.Interfaces()
	if err != nil {
		_ = log.Errorf("error reading network interfaces: %s", err)
		return addresses
	}

	for _, intf := range interfaces {
		addrs, err := intf.Addrs()

		if err != nil {
			_ = log.Errorf("error reading interface %s addresses: %s", intf.Name, err)
			continue
		}

		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				addresses[util.AddressFromNetIP(v.IP)] = struct{}{}
			case *net.IPAddr:
				addresses[util.AddressFromNetIP(v.IP)] = struct{}{}
			}
		}

	}

	return addresses
}

// SectionsFromConfig returns a map of string -> gobpf.SectionParams used to configure the way we load the BPF program (bpf map sizes)
func SectionsFromConfig(c *Config) map[string]bpflib.SectionParams {
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
		tcpCloseEventMap.sectionName(): {
			MapMaxEntries: 1024,
		},
	}
}
