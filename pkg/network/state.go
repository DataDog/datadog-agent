// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/network/http/transaction"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	_ State = &networkState{}
)

const (
	// DEBUGCLIENT is the ClientID for debugging
	DEBUGCLIENT = "-1"

	// DNSResponseCodeNoError is the value that indicates that the DNS reply contains no errors.
	// We could have used layers.DNSResponseCodeNoErr here. But importing the gopacket library only for this
	// constant is not worth the increased memory cost.
	DNSResponseCodeNoError = 0

	// ConnectionByteKeyMaxLen represents the maximum size in bytes of a connection byte key
	ConnectionByteKeyMaxLen = 41
)

// State takes care of handling the logic for:
// - closed connections
// - sent and received bytes per connection
type State interface {
	// GetDelta returns a Delta object for the given client when provided the latest set of active connections
	GetDelta(
		clientID string,
		latestTime uint64,
		active []ConnectionStats,
		dns dns.StatsByKeyByNameByType,
		http map[transaction.Key]*http.RequestStats,
	) Delta

	// GetTelemetryDelta returns the telemetry delta since last time the given client requested telemetry data.
	GetTelemetryDelta(
		id string,
		telemetry map[ConnTelemetryType]int64,
	) map[ConnTelemetryType]int64

	// RegisterClient starts tracking stateful data for the given client
	// If the client is already registered, it does nothing.
	RegisterClient(clientID string)

	// RemoveClient stops tracking stateful data for a given client
	RemoveClient(clientID string)

	// RemoveExpiredClients removes expired clients from the state
	RemoveExpiredClients(now time.Time)

	// RemoveConnections removes the given keys from the state
	RemoveConnections(conns []*ConnectionStats)

	// StoreClosedConnections stores a batch of closed connections
	StoreClosedConnections(connections []ConnectionStats)

	// GetStats returns a map of statistics about the current network state
	GetStats() map[string]interface{}

	// DumpState returns a map with the current network state for a client ID
	DumpState(clientID string) map[string]interface{}
}

// Delta represents a delta of network data compared to the last call to State.
type Delta struct {
	BufferedData
	HTTP     map[transaction.Key]*http.RequestStats
	DNSStats dns.StatsByKeyByNameByType
}

type telemetry struct {
	closedConnDropped  int64
	connDropped        int64
	statsUnderflows    int64
	timeSyncCollisions int64
	dnsStatsDropped    int64
	httpStatsDropped   int64
	dnsPidCollisions   int64
}

const minClosedCapacity = 1024

type client struct {
	lastFetch time.Time

	// generated via `ByteKey` and used exclusively to roll up closed connections
	closedConnectionsKeys map[string]int

	closedConnections []ConnectionStats
	stats             map[string]StatCountersByCookie
	// maps by dns key the domain (string) to stats structure
	dnsStats        dns.StatsByKeyByNameByType
	httpStatsDelta  map[transaction.Key]*http.RequestStats
	lastTelemetries map[ConnTelemetryType]int64
}

func (c *client) Reset(active map[string]*ConnectionStats) {
	half := cap(c.closedConnections) / 2
	if closedLen := len(c.closedConnections); closedLen > minClosedCapacity && closedLen < half {
		c.closedConnections = make([]ConnectionStats, half)
	}

	c.closedConnections = c.closedConnections[:0]
	c.closedConnectionsKeys = make(map[string]int)
	c.dnsStats = make(dns.StatsByKeyByNameByType)
	c.httpStatsDelta = make(map[transaction.Key]*http.RequestStats)

	// XXX: we should change the way we clean this map once
	// https://github.com/golang/go/issues/20135 is solved
	newStats := make(map[string]StatCountersByCookie, len(c.stats))
	for key, st := range c.stats {
		// Only keep active connections stats
		if _, isActive := active[key]; isActive {
			newStats[key] = st
		}
	}
	c.stats = newStats
}

type networkState struct {
	sync.Mutex

	// clients is a map of the connection id string to the client structure
	clients       map[string]*client
	telemetry     telemetry // Monotonic state telemetry
	lastTelemetry telemetry // Old telemetry state; used for logging

	buf             []byte // Shared buffer
	latestTimeEpoch uint64

	// Network state configuration
	clientExpiry   time.Duration
	maxClosedConns int
	maxClientStats int
	maxDNSStats    int
	maxHTTPStats   int
}

// NewState creates a new network state
func NewState(clientExpiry time.Duration, maxClosedConns, maxClientStats int, maxDNSStats int, maxHTTPStats int) State {
	return &networkState{
		clients:        map[string]*client{},
		telemetry:      telemetry{},
		clientExpiry:   clientExpiry,
		maxClosedConns: maxClosedConns,
		maxClientStats: maxClientStats,
		maxDNSStats:    maxDNSStats,
		maxHTTPStats:   maxHTTPStats,
		buf:            make([]byte, ConnectionByteKeyMaxLen),
	}
}

func (ns *networkState) getClients() []string {
	ns.Lock()
	defer ns.Unlock()
	clients := make([]string, 0, len(ns.clients))

	for id := range ns.clients {
		clients = append(clients, id)
	}

	return clients
}

// GetTelemetryDelta returns the telemetry delta for a given client.
// As for now, this only keeps track of monotonic telemetry, as the
// other ones are already relative to the last time they were fetched.
func (ns *networkState) GetTelemetryDelta(
	id string,
	telemetry map[ConnTelemetryType]int64,
) map[ConnTelemetryType]int64 {
	ns.Lock()
	defer ns.Unlock()

	if len(telemetry) > 0 {
		return ns.getTelemetryDelta(id, telemetry)
	}
	return nil
}

// GetDelta returns the connections for the given client
// If the client is not registered yet, we register it and return the connections we have in the global state
// Otherwise we return both the connections with last stats and the closed connections for this client
func (ns *networkState) GetDelta(
	id string,
	latestTime uint64,
	active []ConnectionStats,
	dnsStats dns.StatsByKeyByNameByType,
	httpStats map[transaction.Key]*http.RequestStats,
) Delta {
	ns.Lock()
	defer ns.Unlock()

	// Update the latest known time
	ns.latestTimeEpoch = latestTime
	connsByKey := getConnsByKey(active, ns.buf)

	clientBuffer := clientPool.Get(id)
	client := ns.getClient(id)
	defer client.Reset(connsByKey)

	// Update all connections with relevant up-to-date stats for client
	ns.mergeConnections(id, connsByKey, clientBuffer)

	conns := clientBuffer.Connections()
	ns.determineConnectionIntraHost(conns)
	if len(dnsStats) > 0 {
		ns.storeDNSStats(dnsStats)
	}
	if len(httpStats) > 0 {
		ns.storeHTTPStats(httpStats)
	}

	return Delta{
		BufferedData: BufferedData{
			Conns:  conns,
			buffer: clientBuffer,
		},
		HTTP:     client.httpStatsDelta,
		DNSStats: client.dnsStats,
	}
}

// saveTelemetry saves the non-monotonic telemetry data for each registered clients.
// It does so by accumulating values per telemetry point.
func (ns *networkState) saveTelemetry(telemetry map[ConnTelemetryType]int64) {
	for _, cl := range ns.clients {
		for _, telType := range ConnTelemetryTypes {
			if val, ok := telemetry[telType]; ok {
				cl.lastTelemetries[telType] += val
			}
		}
	}
}

func (ns *networkState) getTelemetryDelta(id string, telemetry map[ConnTelemetryType]int64) map[ConnTelemetryType]int64 {
	ns.logTelemetry()

	var res = make(map[ConnTelemetryType]int64)
	client := ns.getClient(id)
	ns.saveTelemetry(telemetry)

	for _, telType := range MonotonicConnTelemetryTypes {
		if val, ok := telemetry[telType]; ok {
			res[telType] = val
			if prev, ok := client.lastTelemetries[telType]; ok {
				res[telType] -= prev
			}
			client.lastTelemetries[telType] = val
		}
	}

	for _, telType := range ConnTelemetryTypes {
		if _, ok := client.lastTelemetries[telType]; ok {
			res[telType] = client.lastTelemetries[telType]
			client.lastTelemetries[telType] = 0
		}
	}

	return res
}

func (ns *networkState) logTelemetry() {
	delta := telemetry{
		closedConnDropped:  ns.telemetry.closedConnDropped - ns.lastTelemetry.closedConnDropped,
		connDropped:        ns.telemetry.connDropped - ns.lastTelemetry.connDropped,
		statsUnderflows:    ns.telemetry.statsUnderflows - ns.lastTelemetry.statsUnderflows,
		timeSyncCollisions: ns.telemetry.timeSyncCollisions - ns.lastTelemetry.timeSyncCollisions,
		dnsStatsDropped:    ns.telemetry.dnsStatsDropped - ns.lastTelemetry.dnsStatsDropped,
		httpStatsDropped:   ns.telemetry.httpStatsDropped - ns.lastTelemetry.httpStatsDropped,
		dnsPidCollisions:   ns.telemetry.dnsPidCollisions - ns.lastTelemetry.dnsPidCollisions,
	}

	// Flush log line if any metric is non-zero
	if delta.statsUnderflows > 0 || delta.closedConnDropped > 0 || delta.connDropped > 0 || delta.timeSyncCollisions > 0 ||
		delta.dnsStatsDropped > 0 || delta.httpStatsDropped > 0 || delta.dnsPidCollisions > 0 {
		s := "state telemetry: "
		s += " [%d stats stats_underflows]"
		s += " [%d connections dropped due to stats]"
		s += " [%d closed connections dropped]"
		s += " [%d dns stats dropped]"
		s += " [%d HTTP stats dropped]"
		s += " [%d DNS pid collisions]"
		s += " [%d time sync collisions]"
		log.Warnf(s,
			delta.statsUnderflows,
			delta.connDropped,
			delta.closedConnDropped,
			delta.dnsStatsDropped,
			delta.httpStatsDropped,
			delta.dnsPidCollisions,
			delta.timeSyncCollisions)
	}

	ns.lastTelemetry = ns.telemetry
}

// RegisterClient registers a client before it first gets stream of data.
// This call is not strictly mandatory, although it is useful when users
// want to first register and then start getting data at regular intervals.
// If the client is already registered, this call simply does nothing.
// The purpose of this new method is to start registering closed connections
// for the given client once this call has been made.
func (ns *networkState) RegisterClient(id string) {
	ns.Lock()
	defer ns.Unlock()

	_ = ns.getClient(id)
}

// getConnsByKey returns a mapping of byte-key -> connection for easier access + manipulation
func getConnsByKey(conns []ConnectionStats, buf []byte) map[string]*ConnectionStats {
	connsByKey := make(map[string]*ConnectionStats, len(conns))
	for i := range conns {
		key := string(conns[i].ByteKey(buf))
		var c *ConnectionStats
		if c = connsByKey[key]; c == nil {
			connsByKey[key] = &conns[i]
			continue
		}

		log.Tracef("duplicate connection in collection: key: %s, c1: %+v, c2: %+v", BeautifyKey(key), *c, conns[i])
		mergeConnectionStats(c, &conns[i])
	}

	return connsByKey
}

func (ns *networkState) StoreClosedConnections(closed []ConnectionStats) {
	ns.Lock()
	defer ns.Unlock()

	ns.storeClosedConnections(closed)
}

// StoreClosedConnection stores the given connection for every client
func (ns *networkState) storeClosedConnections(conns []ConnectionStats) {
	for _, client := range ns.clients {
		for _, c := range conns {
			key := string(c.ByteKeyNAT(ns.buf))

			if i, ok := client.closedConnectionsKeys[key]; ok {
				mergeConnectionStats(&client.closedConnections[i], &c)
				continue
			}

			if len(client.closedConnections) >= ns.maxClosedConns {
				ns.telemetry.closedConnDropped++
				continue
			}

			client.closedConnections = append(client.closedConnections, c)
			client.closedConnectionsKeys[key] = len(client.closedConnections) - 1
		}
	}
}

func getDeepDNSStatsCount(stats dns.StatsByKeyByNameByType) int {
	var count int
	for _, bykey := range stats {
		for _, bydomain := range bykey {
			count += len(bydomain)
		}
	}
	return count
}

// storeDNSStats stores latest DNS stats for all clients
func (ns *networkState) storeDNSStats(stats dns.StatsByKeyByNameByType) {
	// Fast-path for common case (one client registered)
	if len(ns.clients) == 1 {
		for _, c := range ns.clients {
			if len(c.dnsStats) == 0 {
				c.dnsStats = stats
			}
			return
		}
	}

	for _, client := range ns.clients {
		dnsStatsThisClient := getDeepDNSStatsCount(client.dnsStats)
		for key, statsByDomain := range stats {
			for domain, statsByQtype := range statsByDomain {
				for qtype, dnsStats := range statsByQtype {

					if _, ok := client.dnsStats[key]; !ok {
						if dnsStatsThisClient >= ns.maxDNSStats {
							ns.telemetry.dnsStatsDropped++
							continue
						}
						client.dnsStats[key] = make(map[dns.Hostname]map[dns.QueryType]dns.Stats)
					}
					if _, ok := client.dnsStats[key][domain]; !ok {
						if dnsStatsThisClient >= ns.maxDNSStats {
							ns.telemetry.dnsStatsDropped++
							continue
						}
						client.dnsStats[key][domain] = make(map[dns.QueryType]dns.Stats)
					}

					// If we've seen DNS stats for this key already, let's combine the two
					if prev, ok := client.dnsStats[key][domain][qtype]; ok {
						prev.Timeouts += dnsStats.Timeouts
						prev.SuccessLatencySum += dnsStats.SuccessLatencySum
						prev.FailureLatencySum += dnsStats.FailureLatencySum
						for rcode, count := range dnsStats.CountByRcode {
							prev.CountByRcode[rcode] += count
						}
						client.dnsStats[key][domain][qtype] = prev
					} else {
						if dnsStatsThisClient >= ns.maxDNSStats {
							ns.telemetry.dnsStatsDropped++
							continue
						}
						client.dnsStats[key][domain][qtype] = dnsStats
						dnsStatsThisClient++
					}
				}
			}
		}
	}
}

// storeHTTPStats stores the latest HTTP stats for all clients
func (ns *networkState) storeHTTPStats(allStats map[transaction.Key]*http.RequestStats) {
	if len(ns.clients) == 1 {
		for _, client := range ns.clients {
			if len(client.httpStatsDelta) == 0 {
				// optimization for the common case:
				// if there is only one client and no previous state, no memory allocation is needed
				client.httpStatsDelta = allStats
				return
			}
		}
	}

	for key, stats := range allStats {
		for _, client := range ns.clients {
			prevStats, ok := client.httpStatsDelta[key]
			if !ok && len(client.httpStatsDelta) >= ns.maxHTTPStats {
				ns.telemetry.httpStatsDropped++
				continue
			}

			if prevStats != nil {
				prevStats.CombineWith(stats)
				client.httpStatsDelta[key] = prevStats
			} else {
				client.httpStatsDelta[key] = stats
			}
		}
	}
}

func (ns *networkState) getClient(clientID string) *client {
	if c, ok := ns.clients[clientID]; ok {
		return c
	}

	c := &client{
		lastFetch:             time.Now(),
		stats:                 make(map[string]StatCountersByCookie),
		closedConnections:     make([]ConnectionStats, 0, minClosedCapacity),
		closedConnectionsKeys: make(map[string]int),
		dnsStats:              dns.StatsByKeyByNameByType{},
		httpStatsDelta:        map[transaction.Key]*http.RequestStats{},
		lastTelemetries:       make(map[ConnTelemetryType]int64),
	}
	ns.clients[clientID] = c
	return c
}

// mergeConnections return the connections and takes care of updating their last stat counters
func (ns *networkState) mergeConnections(id string, active map[string]*ConnectionStats, buffer *clientBuffer) {
	now := time.Now()

	client := ns.clients[id]
	client.lastFetch = now

	closed := client.closedConnections
	closedKeys := make(map[string]struct{}, len(closed))
	for i := range closed {
		closedConn := &closed[i]
		key := string(closedConn.ByteKey(ns.buf))
		closedKeys[key] = struct{}{}

		var activeConn *ConnectionStats
		if activeConn = active[key]; activeConn != nil {
			mergeConnectionStats(closedConn, activeConn)
			ns.createStatsForKey(client, key)
		}

		ns.updateConnWithStats(client, key, closedConn)

		if closedConn.Last.IsZero() {
			continue
		}
		*buffer.Next() = *closedConn
	}

	// Active connections
	for key, c := range active {
		// If the connection was closed, it has already been processed so skip it
		if _, ok := closedKeys[key]; ok {
			continue
		}

		ns.createStatsForKey(client, key)
		ns.updateConnWithStats(client, key, c)

		if c.Last.IsZero() {
			continue
		}
		*buffer.Next() = *c
	}
}

func (ns *networkState) updateConnWithStats(client *client, key string, c *ConnectionStats) {
	c.Last = StatCounters{}
	if sts, ok := client.stats[key]; ok {
		for _, cm := range c.Monotonic {
			cookie := cm.Cookie
			counters := cm.StatCounters

			st, ok := sts.Get(cookie)
			if !ok {
				c.Last = c.Last.Add(counters)
				sts.Put(cookie, counters)
				client.stats[key] = sts
				continue
			}

			var last StatCounters
			var underflow bool
			if last, underflow = counters.Sub(st); underflow {
				ns.telemetry.statsUnderflows++
				log.Debugf("Stats underflow for key:%s, stats:%+v, connection:%+v", BeautifyKey(key), st, *c)

				counters = counters.Max(st)
				last, _ = counters.Sub(st)
			}

			c.Last = c.Last.Add(last)

			sts.Put(cookie, counters)
			client.stats[key] = sts
		}
	} else {
		for _, counters := range c.Monotonic {
			c.Last = c.Last.Add(counters.StatCounters)
		}
	}
}

// createStatsForKey will create a new stats object for a key if it doesn't already exist.
func (ns *networkState) createStatsForKey(client *client, key string) {
	if _, ok := client.stats[key]; !ok {
		if len(client.stats) >= ns.maxClientStats {
			ns.telemetry.connDropped++
			return
		}
		client.stats[key] = make(StatCountersByCookie, 0, 3)
	}
}

func (ns *networkState) RemoveClient(clientID string) {
	ns.Lock()
	defer ns.Unlock()
	delete(ns.clients, clientID)
	clientPool.RemoveExpiredClient(clientID)
}

func (ns *networkState) RemoveExpiredClients(now time.Time) {
	ns.Lock()
	defer ns.Unlock()

	for id, c := range ns.clients {
		if c.lastFetch.Add(ns.clientExpiry).Before(now) {
			log.Debugf("expiring client: %s, had %d stats and %d closed connections", id, len(c.stats), len(c.closedConnections))
			delete(ns.clients, id)
			clientPool.RemoveExpiredClient(id)
		}
	}
}

func (ns *networkState) RemoveConnections(conns []*ConnectionStats) {
	ns.Lock()
	defer ns.Unlock()

	for _, cl := range ns.clients {
		for _, c := range conns {
			key := c.ByteKey(ns.buf)
			delete(cl.stats, string(key))
		}
	}
}

// GetStats returns a map of statistics about the current network state
func (ns *networkState) GetStats() map[string]interface{} {
	ns.Lock()
	defer ns.Unlock()

	clientInfo := map[string]interface{}{}
	for id, c := range ns.clients {
		clientInfo[id] = map[string]int{
			"stats":              len(c.stats),
			"closed_connections": len(c.closedConnections),
			"last_fetch":         int(c.lastFetch.Unix()),
		}
	}

	return map[string]interface{}{
		"clients": clientInfo,
		"telemetry": map[string]int64{
			"stats_underflows":     ns.telemetry.statsUnderflows,
			"closed_conn_dropped":  ns.telemetry.closedConnDropped,
			"conn_dropped":         ns.telemetry.connDropped,
			"time_sync_collisions": ns.telemetry.timeSyncCollisions,
			"dns_stats_dropped":    ns.telemetry.dnsStatsDropped,
			"http_stats_dropped":   ns.telemetry.httpStatsDropped,
			"dns_pid_collisions":   ns.telemetry.dnsPidCollisions,
		},
		"current_time":       time.Now().Unix(),
		"latest_bpf_time_ns": ns.latestTimeEpoch,
	}
}

// DumpState returns the entirety of the network state in memory at the moment for a particular clientID, for debugging
func (ns *networkState) DumpState(clientID string) map[string]interface{} {
	ns.Lock()
	defer ns.Unlock()

	data := map[string]interface{}{}
	if client, ok := ns.clients[clientID]; ok {
		for connKey, s := range client.stats {
			byCookie := map[uint32]interface{}{}
			for _, st := range s {
				byCookie[st.Cookie] = map[string]uint64{
					"total_sent":            st.SentBytes,
					"total_recv":            st.RecvBytes,
					"total_retransmits":     uint64(st.Retransmits),
					"total_tcp_established": uint64(st.TCPEstablished),
					"total_tcp_closed":      uint64(st.TCPClosed),
				}
			}

			data[BeautifyKey(connKey)] = byCookie
		}
	}
	return data
}

func (ns *networkState) determineConnectionIntraHost(connections []ConnectionStats) {
	type connKey struct {
		Address util.Address
		Port    uint16
		Type    ConnectionType
	}

	newConnKey := func(connStat *ConnectionStats, useRAddrAsKey bool) connKey {
		key := connKey{Type: connStat.Type}
		if useRAddrAsKey {
			if connStat.IPTranslation == nil {
				key.Address = connStat.Dest
				key.Port = connStat.DPort
			} else {
				key.Address = connStat.IPTranslation.ReplSrcIP
				key.Port = connStat.IPTranslation.ReplSrcPort
			}
		} else {
			key.Address = connStat.Source
			key.Port = connStat.SPort
		}
		return key
	}

	lAddrs := make(map[connKey]struct{}, len(connections))
	for _, conn := range connections {
		k := newConnKey(&conn, false)
		lAddrs[k] = struct{}{}
	}

	// do not use range value here since it will create a copy of the ConnectionStats object
	for i := range connections {
		conn := &connections[i]
		if conn.Source == conn.Dest ||
			(conn.Source.IsLoopback() && conn.Dest.IsLoopback()) ||
			(conn.IPTranslation != nil && conn.IPTranslation.ReplSrcIP.IsLoopback()) {
			conn.IntraHost = true
		} else {
			keyWithRAddr := newConnKey(conn, true)
			_, conn.IntraHost = lAddrs[keyWithRAddr]
		}

		if conn.IntraHost && conn.Direction == INCOMING {
			// Remove ip translation from incoming local connections
			// this is necessary for local connections because of
			// the way we store conntrack entries in the conntrack
			// cache in the system-probe. For local connections
			// that are DNAT'ed, system-probe will tack on the
			// translation on the incoming source side as well,
			// even though there is no SNAT on the incoming side.
			// This is because we store both the origin and reply
			// (and map them to each other) in the conntrack cache
			// in system-probe.
			conn.IPTranslation = nil
		}
	}
}

func mergeConnectionStats(a, b *ConnectionStats) {
	for _, bm := range b.Monotonic {
		if ams, ok := a.Monotonic.Get(bm.Cookie); ok {
			a.Monotonic.Put(bm.Cookie, ams.Max(bm.StatCounters))
			continue
		}

		a.Monotonic.Put(bm.Cookie, bm.StatCounters)
	}

	if b.LastUpdateEpoch > a.LastUpdateEpoch {
		a.LastUpdateEpoch = b.LastUpdateEpoch
	}

	if a.IPTranslation == nil {
		a.IPTranslation = b.IPTranslation
	}
}
