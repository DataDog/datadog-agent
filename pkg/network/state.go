// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"bytes"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"github.com/DataDog/datadog-agent/pkg/network/slice"
	nettelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	_ State = &networkState{}
)

// Telemetry
var stateTelemetry = struct {
	closedConnDropped     *nettelemetry.StatCounterWrapper
	connDropped           *nettelemetry.StatCounterWrapper
	statsUnderflows       *nettelemetry.StatCounterWrapper
	statsCookieCollisions *nettelemetry.StatCounterWrapper
	timeSyncCollisions    *nettelemetry.StatCounterWrapper
	dnsStatsDropped       *nettelemetry.StatCounterWrapper
	httpStatsDropped      *nettelemetry.StatCounterWrapper
	http2StatsDropped     *nettelemetry.StatCounterWrapper
	kafkaStatsDropped     *nettelemetry.StatCounterWrapper
	dnsPidCollisions      *nettelemetry.StatCounterWrapper
	udpDirectionFixes     telemetry.Counter
}{
	nettelemetry.NewStatCounterWrapper(stateModuleName, "closed_conn_dropped", []string{"ip_proto"}, "Counter measuring the number of dropped closed connections"),
	nettelemetry.NewStatCounterWrapper(stateModuleName, "conn_dropped", []string{}, "Counter measuring the number of closed connections"),
	nettelemetry.NewStatCounterWrapper(stateModuleName, "stats_underflows", []string{}, "Counter measuring the number of stats underflows"),
	nettelemetry.NewStatCounterWrapper(stateModuleName, "stats_cookie_collisions", []string{}, "Counter measuring the number of stats cookie collisions"),
	nettelemetry.NewStatCounterWrapper(stateModuleName, "time_sync_collisions", []string{}, "Counter measuring the number of time sync collisions"),
	nettelemetry.NewStatCounterWrapper(stateModuleName, "dns_stats_dropped", []string{}, "Counter measuring the number of DNS stats dropped"),
	nettelemetry.NewStatCounterWrapper(stateModuleName, "http_stats_dropped", []string{}, "Counter measuring the number of http stats dropped"),
	nettelemetry.NewStatCounterWrapper(stateModuleName, "http2_stats_dropped", []string{}, "Counter measuring the number of http2 stats dropped"),
	nettelemetry.NewStatCounterWrapper(stateModuleName, "kafka_stats_dropped", []string{}, "Counter measuring the number of kafka stats dropped"),
	nettelemetry.NewStatCounterWrapper(stateModuleName, "dns_pid_collisions", []string{}, "Counter measuring the number of DNS PID collisions"),
	telemetry.NewCounter(stateModuleName, "udp_direction_fixes", []string{}, "Counter measuring the number of udp direction fixes"),
}

const (
	// DEBUGCLIENT is the ClientID for debugging
	DEBUGCLIENT = "-1"

	// DNSResponseCodeNoError is the value that indicates that the DNS reply contains no errors.
	// We could have used layers.DNSResponseCodeNoErr here. But importing the gopacket library only for this
	// constant is not worth the increased memory cost.
	DNSResponseCodeNoError = 0

	// ConnectionByteKeyMaxLen represents the maximum size in bytes of a connection byte key
	ConnectionByteKeyMaxLen = 41

	stateModuleName = "network_tracer__state"
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
		usmStats map[protocols.ProtocolType]interface{},
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
	Conns    []ConnectionStats
	HTTP     map[http.Key]*http.RequestStats
	HTTP2    map[http.Key]*http.RequestStats
	Kafka    map[kafka.Key]*kafka.RequestStat
	DNSStats dns.StatsByKeyByNameByType
}

type lastStateTelemetry struct {
	closedConnDropped     int64
	connDropped           int64
	statsUnderflows       int64
	statsCookieCollisions int64
	timeSyncCollisions    int64
	dnsStatsDropped       int64
	httpStatsDropped      int64
	http2StatsDropped     int64
	kafkaStatsDropped     int64
	dnsPidCollisions      int64
}

const minClosedCapacity = 1024

type client struct {
	lastFetch time.Time

	closedConnectionsKeys map[StatCookie]int

	closedConnections []ConnectionStats
	stats             map[StatCookie]StatCounters
	// maps by dns key the domain (string) to stats structure
	dnsStats        dns.StatsByKeyByNameByType
	httpStatsDelta  map[http.Key]*http.RequestStats
	http2StatsDelta map[http.Key]*http.RequestStats
	kafkaStatsDelta map[kafka.Key]*kafka.RequestStat
	lastTelemetries map[ConnTelemetryType]int64
}

func (c *client) Reset() {
	half := cap(c.closedConnections) / 2
	if closedLen := len(c.closedConnections); closedLen > minClosedCapacity && closedLen < half {
		c.closedConnections = make([]ConnectionStats, half)
	}

	c.closedConnections = c.closedConnections[:0]
	c.closedConnectionsKeys = make(map[StatCookie]int)
	c.dnsStats = make(dns.StatsByKeyByNameByType)
	c.httpStatsDelta = make(map[http.Key]*http.RequestStats)
	c.http2StatsDelta = make(map[http.Key]*http.RequestStats)
	c.kafkaStatsDelta = make(map[kafka.Key]*kafka.RequestStat)
}

type networkState struct {
	sync.Mutex

	// clients is a map of the connection id string to the client structure
	clients       map[string]*client
	lastTelemetry lastStateTelemetry // Old telemetry state; used for logging

	latestTimeEpoch uint64

	// Network state configuration
	clientExpiry   time.Duration
	maxClosedConns uint32
	maxClientStats int
	maxDNSStats    int
	maxHTTPStats   int
	maxKafkaStats  int

	mergeStatsBuffers [2][]byte
}

// NewState creates a new network state
func NewState(clientExpiry time.Duration, maxClosedConns uint32, maxClientStats int, maxDNSStats int, maxHTTPStats int, maxKafkaStats int) State {
	return &networkState{
		clients:        map[string]*client{},
		clientExpiry:   clientExpiry,
		maxClosedConns: maxClosedConns,
		maxClientStats: maxClientStats,
		maxDNSStats:    maxDNSStats,
		maxHTTPStats:   maxHTTPStats,
		maxKafkaStats:  maxKafkaStats,
		mergeStatsBuffers: [2][]byte{
			make([]byte, ConnectionByteKeyMaxLen),
			make([]byte, ConnectionByteKeyMaxLen),
		},
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

func filterConnections(conns []ConnectionStats, keep func(c *ConnectionStats) bool) []ConnectionStats {
	p := 0
	for i := range conns {
		// swap first so that the connection pointer
		// passed to keep will be stable
		conns[p], conns[i] = conns[i], conns[p]
		if keep(&conns[p]) {
			p++
		}
	}

	return conns[:p]
}

// GetDelta returns the connections for the given client
// If the client is not registered yet, we register it and return the connections we have in the global state
// Otherwise we return both the connections with last stats and the closed connections for this client
func (ns *networkState) GetDelta(
	id string,
	latestTime uint64,
	active []ConnectionStats,
	dnsStats dns.StatsByKeyByNameByType,
	usmStats map[protocols.ProtocolType]interface{},
) Delta {
	ns.Lock()
	defer ns.Unlock()

	// Update the latest known time
	ns.latestTimeEpoch = latestTime

	client := ns.getClient(id)
	defer client.Reset()

	// Update all connections with relevant up-to-date stats for client
	active, closed := ns.mergeConnections(id, active)

	aggr := newConnectionAggregator((len(closed) + len(active)) / 2)
	active = filterConnections(active, func(c *ConnectionStats) bool {
		return !aggr.Aggregate(c)
	})

	closed = filterConnections(closed, func(c *ConnectionStats) bool {
		return !aggr.Aggregate(c)
	})

	aggr.finalize()

	ns.determineConnectionIntraHost(slice.NewChain(active, closed))

	if len(dnsStats) > 0 {
		ns.storeDNSStats(dnsStats)
	}

	for protocolType, protocolStats := range usmStats {
		switch protocolType {
		case protocols.HTTP:
			stats := protocolStats.(map[http.Key]*http.RequestStats)
			ns.storeHTTPStats(stats)
		case protocols.Kafka:
			stats := protocolStats.(map[kafka.Key]*kafka.RequestStat)
			ns.storeKafkaStats(stats)
		case protocols.HTTP2:
			stats := protocolStats.(map[http.Key]*http.RequestStats)
			ns.storeHTTP2Stats(stats)
		}
	}

	return Delta{
		Conns:    append(active, closed...),
		HTTP:     client.httpStatsDelta,
		HTTP2:    client.http2StatsDelta,
		DNSStats: client.dnsStats,
		Kafka:    client.kafkaStatsDelta,
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
	closedConnDroppedDelta := stateTelemetry.closedConnDropped.Load() - ns.lastTelemetry.closedConnDropped
	connDroppedDelta := stateTelemetry.connDropped.Load() - ns.lastTelemetry.connDropped
	statsUnderflowsDelta := stateTelemetry.statsUnderflows.Load() - ns.lastTelemetry.statsUnderflows
	statsCookieCollisionsDelta := stateTelemetry.statsCookieCollisions.Load() - ns.lastTelemetry.statsCookieCollisions
	timeSyncCollisionsDelta := stateTelemetry.timeSyncCollisions.Load() - ns.lastTelemetry.timeSyncCollisions
	dnsStatsDroppedDelta := stateTelemetry.dnsStatsDropped.Load() - ns.lastTelemetry.dnsStatsDropped
	httpStatsDroppedDelta := stateTelemetry.httpStatsDropped.Load() - ns.lastTelemetry.httpStatsDropped
	http2StatsDroppedDelta := stateTelemetry.http2StatsDropped.Load() - ns.lastTelemetry.http2StatsDropped
	kafkaStatsDroppedDelta := stateTelemetry.kafkaStatsDropped.Load() - ns.lastTelemetry.kafkaStatsDropped
	dnsPidCollisionsDelta := stateTelemetry.dnsPidCollisions.Load() - ns.lastTelemetry.dnsPidCollisions

	// Flush log line if any metric is non-zero
	if connDroppedDelta > 0 || closedConnDroppedDelta > 0 || dnsStatsDroppedDelta > 0 ||
		httpStatsDroppedDelta > 0 || http2StatsDroppedDelta > 0 || kafkaStatsDroppedDelta > 0 {
		s := "State telemetry: "
		s += " [%d connections dropped due to stats]"
		s += " [%d closed connections dropped]"
		s += " [%d DNS stats dropped]"
		s += " [%d HTTP stats dropped]"
		s += " [%d HTTP2 stats dropped]"
		s += " [%d Kafka stats dropped]"
		log.Warnf(s,
			connDroppedDelta,
			closedConnDroppedDelta,
			dnsStatsDroppedDelta,
			httpStatsDroppedDelta,
			http2StatsDroppedDelta,
			kafkaStatsDroppedDelta,
		)
	}

	// debug metrics that aren't useful for customers to see
	if statsCookieCollisionsDelta > 0 || statsUnderflowsDelta > 0 ||
		timeSyncCollisionsDelta > 0 || dnsPidCollisionsDelta > 0 {
		s := "State telemetry debug: "
		s += " [%d stats cookie collisions]"
		s += " [%d stats underflows]"
		s += " [%d time sync collisions]"
		s += " [%d DNS pid collisions]"
		log.Debugf(s,
			statsCookieCollisionsDelta,
			statsUnderflowsDelta,
			timeSyncCollisionsDelta,
			dnsPidCollisionsDelta,
		)
	}

	ns.lastTelemetry.closedConnDropped = stateTelemetry.closedConnDropped.Load()
	ns.lastTelemetry.connDropped = stateTelemetry.connDropped.Load()
	ns.lastTelemetry.statsUnderflows = stateTelemetry.statsUnderflows.Load()
	ns.lastTelemetry.statsCookieCollisions = stateTelemetry.statsCookieCollisions.Load()
	ns.lastTelemetry.timeSyncCollisions = stateTelemetry.timeSyncCollisions.Load()
	ns.lastTelemetry.dnsStatsDropped = stateTelemetry.dnsStatsDropped.Load()
	ns.lastTelemetry.httpStatsDropped = stateTelemetry.httpStatsDropped.Load()
	ns.lastTelemetry.http2StatsDropped = stateTelemetry.http2StatsDropped.Load()
	ns.lastTelemetry.kafkaStatsDropped = stateTelemetry.kafkaStatsDropped.Load()
	ns.lastTelemetry.dnsPidCollisions = stateTelemetry.dnsPidCollisions.Load()
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

// mergeByCookie merges connections with the same cookie and returns an index by cookie
//
// The passed connections slice is modified to remove duplicate connections that have
// the same cookie, returning a subset of connections. The returned map has pointers
// into this returned slice. The removed connections are merged with the one connection
// (per cookie) that is retained.
func (ns *networkState) mergeByCookie(conns []ConnectionStats) ([]ConnectionStats, map[StatCookie]*ConnectionStats) {
	connsByKey := make(map[StatCookie]*ConnectionStats, len(conns))
	conns = filterConnections(conns, func(c *ConnectionStats) bool {
		ck := connsByKey[c.Cookie]
		if ck == nil {
			connsByKey[c.Cookie] = c
			return true
		}

		if log.ShouldLog(seelog.TraceLvl) {
			log.Tracef("duplicate connection in collection: cookie: %d, c1: %+v, c2: %+v", c.Cookie, *ck, *c)
		}

		if ns.mergeConnectionStats(ck, c) {
			// cookie collision
			stateTelemetry.statsCookieCollisions.Inc()
			// pick the latest one
			if c.LastUpdateEpoch > ck.LastUpdateEpoch {
				// we overwrite the value here without
				// updating the pointer in the map
				// since keeping `c` would mean discarding
				// `ck`, which is not possible here since
				// we have already signaled to `filterConnections`
				// we want to keep `ck`
				*ck = *c
			}
		}

		return false
	})

	return conns, connsByKey
}

func (ns *networkState) StoreClosedConnections(closed []ConnectionStats) {
	ns.Lock()
	defer ns.Unlock()

	ns.storeClosedConnections(closed)
}

// storeClosedConnection stores the given connection for every client
func (ns *networkState) storeClosedConnections(conns []ConnectionStats) {
	for _, client := range ns.clients {
		for _, c := range conns {
			if i, ok := client.closedConnectionsKeys[c.Cookie]; ok {
				if ns.mergeConnectionStats(&client.closedConnections[i], &c) {
					stateTelemetry.statsCookieCollisions.Inc()
					// pick the latest one
					if c.LastUpdateEpoch > client.closedConnections[i].LastUpdateEpoch {
						client.closedConnections[i] = c
					}
				}
				continue
			}

			if uint32(len(client.closedConnections)) >= ns.maxClosedConns {
				stateTelemetry.closedConnDropped.Inc(c.Type.String())
				continue
			}

			client.closedConnections = append(client.closedConnections, c)
			client.closedConnectionsKeys[c.Cookie] = len(client.closedConnections) - 1
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
			if len(c.dnsStats) == 0 && getDeepDNSStatsCount(stats) <= ns.maxDNSStats {
				c.dnsStats = stats
				return
			}
		}
	}

	for _, client := range ns.clients {
		dnsStatsThisClient := getDeepDNSStatsCount(client.dnsStats)
		for key, statsByDomain := range stats {
			for domain, statsByQtype := range statsByDomain {
				for qtype, dnsStats := range statsByQtype {
					if _, ok := client.dnsStats[key]; !ok {
						if dnsStatsThisClient >= ns.maxDNSStats {
							stateTelemetry.dnsStatsDropped.Inc()
							continue
						}
						client.dnsStats[key] = make(map[dns.Hostname]map[dns.QueryType]dns.Stats)
					}

					if _, ok := client.dnsStats[key][domain]; !ok {
						if dnsStatsThisClient >= ns.maxDNSStats {
							stateTelemetry.dnsStatsDropped.Inc()
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
						continue
					}

					// new stat
					if dnsStatsThisClient >= ns.maxDNSStats {
						stateTelemetry.dnsStatsDropped.Inc()
						continue
					}

					client.dnsStats[key][domain][qtype] = dnsStats
					dnsStatsThisClient++
				}
			}
		}
	}
}

// storeHTTPStats stores the latest HTTP stats for all clients
func (ns *networkState) storeHTTPStats(allStats map[http.Key]*http.RequestStats) {
	if len(ns.clients) == 1 {
		for _, client := range ns.clients {
			if len(client.httpStatsDelta) == 0 && len(allStats) <= ns.maxHTTPStats {
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
				stateTelemetry.httpStatsDropped.Inc()
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

func (ns *networkState) storeHTTP2Stats(allStats map[http.Key]*http.RequestStats) {
	if len(ns.clients) == 1 {
		for _, client := range ns.clients {
			if len(client.http2StatsDelta) == 0 && len(allStats) <= ns.maxHTTPStats {
				// optimization for the common case:
				// if there is only one client and no previous state, no memory allocation is needed
				client.http2StatsDelta = allStats
				return
			}
		}
	}

	for key, stats := range allStats {
		for _, client := range ns.clients {
			prevStats, ok := client.http2StatsDelta[key]
			// Currently, we are using maxHTTPStats for HTTP2.
			if !ok && len(client.http2StatsDelta) >= ns.maxHTTPStats {
				stateTelemetry.http2StatsDropped.Inc()
				continue
			}

			if prevStats != nil {
				prevStats.CombineWith(stats)
				client.http2StatsDelta[key] = prevStats
			} else {
				client.http2StatsDelta[key] = stats
			}
		}
	}
}

// storeKafkaStats stores the latest Kafka stats for all clients
func (ns *networkState) storeKafkaStats(allStats map[kafka.Key]*kafka.RequestStat) {
	if len(ns.clients) == 1 {
		for _, client := range ns.clients {
			if len(client.kafkaStatsDelta) == 0 && len(allStats) <= ns.maxKafkaStats {
				// optimization for the common case:
				// if there is only one client and no previous state, no memory allocation is needed
				client.kafkaStatsDelta = allStats
				return
			}
		}
	}

	for key, stats := range allStats {
		for _, client := range ns.clients {
			prevStats, ok := client.kafkaStatsDelta[key]
			if !ok && len(client.kafkaStatsDelta) >= ns.maxKafkaStats {
				stateTelemetry.kafkaStatsDropped.Inc()
				continue
			}

			if prevStats != nil {
				prevStats.CombineWith(stats)
				client.kafkaStatsDelta[key] = prevStats
			} else {
				client.kafkaStatsDelta[key] = stats
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
		stats:                 make(map[StatCookie]StatCounters),
		closedConnections:     make([]ConnectionStats, 0, minClosedCapacity),
		closedConnectionsKeys: make(map[StatCookie]int),
		dnsStats:              dns.StatsByKeyByNameByType{},
		httpStatsDelta:        map[http.Key]*http.RequestStats{},
		http2StatsDelta:       map[http.Key]*http.RequestStats{},
		kafkaStatsDelta:       map[kafka.Key]*kafka.RequestStat{},
		lastTelemetries:       make(map[ConnTelemetryType]int64),
	}
	ns.clients[clientID] = c
	return c
}

// mergeConnections return the connections and takes care of updating their last stat counters
func (ns *networkState) mergeConnections(id string, active []ConnectionStats) (_, closed []ConnectionStats) {
	now := time.Now()

	client := ns.clients[id]
	client.lastFetch = now

	// index active connection by cookie, merging
	// connections with the same cookie
	active, activeByCookie := ns.mergeByCookie(active)

	// filter closed connections, keeping those that have changed or have not
	// been aggregated into another connection
	closed = filterConnections(client.closedConnections, func(closedConn *ConnectionStats) bool {
		cookie := closedConn.Cookie
		if activeConn := activeByCookie[cookie]; activeConn != nil {
			if ns.mergeConnectionStats(closedConn, activeConn) {
				stateTelemetry.statsCookieCollisions.Inc()
				// remove any previous stats since we
				// can't distinguish between the two sets of stats
				delete(client.stats, cookie)
				if activeConn.LastUpdateEpoch > closedConn.LastUpdateEpoch {
					// keep active connection
					return false
				}

				// keep closed connection
			}
			// not an active connection
			delete(activeByCookie, cookie)
		}

		ns.updateConnWithStats(client, cookie, closedConn)

		if closedConn.Last.IsZero() {
			// not reporting an "empty" connection
			return false
		}

		return true
	})

	// do the same for active connections
	// keep stats for only active connections
	newStats := make(map[StatCookie]StatCounters, len(activeByCookie))
	active = filterConnections(active, func(c *ConnectionStats) bool {
		if _, isActive := activeByCookie[c.Cookie]; !isActive {
			return false
		}

		ns.createStatsForCookie(client, c.Cookie)
		ns.updateConnWithStats(client, c.Cookie, c)

		newStats[c.Cookie] = client.stats[c.Cookie]

		if c.Last.IsZero() {
			// not reporting an "empty" connection
			return false
		}

		return true
	})

	client.stats = newStats

	return active, closed
}

func (ns *networkState) updateConnWithStats(client *client, cookie StatCookie, c *ConnectionStats) {
	c.Last = StatCounters{}
	if sts, ok := client.stats[cookie]; ok {
		var last StatCounters
		var underflow bool
		if last, underflow = c.Monotonic.Sub(sts); underflow {
			stateTelemetry.statsUnderflows.Inc()
			log.DebugFunc(func() string {
				return fmt.Sprintf("Stats underflow for cookie:%d, stats counters:%+v, connection counters:%+v", c.Cookie, sts, c.Monotonic)
			})

			c.Monotonic = c.Monotonic.Max(sts)
			last, _ = c.Monotonic.Sub(sts)
		}

		c.Last = c.Last.Add(last)
		client.stats[cookie] = c.Monotonic
	} else {
		c.Last = c.Last.Add(c.Monotonic)
	}
}

// createStatsForCookie will create a new stats object for a key if it doesn't already exist.
func (ns *networkState) createStatsForCookie(client *client, cookie StatCookie) {
	if _, ok := client.stats[cookie]; !ok {
		if len(client.stats) >= ns.maxClientStats {
			stateTelemetry.connDropped.Inc()
			return
		}

		client.stats[cookie] = StatCounters{}
	}
}

func (ns *networkState) RemoveClient(clientID string) {
	ns.Lock()
	defer ns.Unlock()
	delete(ns.clients, clientID)
	ClientPool.RemoveExpiredClient(clientID)
}

func (ns *networkState) RemoveExpiredClients(now time.Time) {
	ns.Lock()
	defer ns.Unlock()

	for id, c := range ns.clients {
		if c.lastFetch.Add(ns.clientExpiry).Before(now) {
			log.Debugf("expiring client: %s, had %d stats and %d closed connections", id, len(c.stats), len(c.closedConnections))
			delete(ns.clients, id)
			ClientPool.RemoveExpiredClient(id)
		}
	}
}

func (ns *networkState) RemoveConnections(conns []*ConnectionStats) {
	ns.Lock()
	defer ns.Unlock()

	for _, cl := range ns.clients {
		for _, c := range conns {
			delete(cl.stats, c.Cookie)
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
			"closed_conn_dropped": stateTelemetry.closedConnDropped.Load(),
			"conn_dropped":        stateTelemetry.connDropped.Load(),
			"dns_stats_dropped":   stateTelemetry.dnsStatsDropped.Load(),
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
		for cookie, s := range client.stats {
			data[strconv.Itoa(int(cookie))] = map[string]uint64{
				"total_sent":            s.SentBytes,
				"total_recv":            s.RecvBytes,
				"total_retransmits":     uint64(s.Retransmits),
				"total_tcp_established": uint64(s.TCPEstablished),
				"total_tcp_closed":      uint64(s.TCPClosed),
			}
		}
	}
	return data
}

func isDNAT(c *ConnectionStats) bool {
	return c.Direction == OUTGOING &&
		c.IPTranslation != nil &&
		(c.IPTranslation.ReplSrcIP.Compare(c.Dest.Addr) != 0 ||
			c.IPTranslation.ReplSrcPort != c.DPort)
}

func (ns *networkState) determineConnectionIntraHost(connections slice.Chain[ConnectionStats]) {
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

	type dnatKey struct {
		src, dst     util.Address
		sport, dport uint16
		_type        ConnectionType
	}

	dnats := make(map[dnatKey]struct{}, connections.Len()/2)
	lAddrs := make(map[connKey]struct{}, connections.Len())
	connections.Iterate(func(_ int, conn *ConnectionStats) {
		k := newConnKey(conn, false)
		lAddrs[k] = struct{}{}

		if isDNAT(conn) {
			dnats[dnatKey{
				src:   conn.Source,
				sport: conn.SPort,
				dst:   conn.IPTranslation.ReplSrcIP,
				dport: conn.IPTranslation.ReplSrcPort,
				_type: conn.Type,
			}] = struct{}{}
		}
	})

	// do not use range value here since it will create a copy of the ConnectionStats object
	connections.Iterate(func(_ int, conn *ConnectionStats) {
		if conn.Source == conn.Dest ||
			(conn.Source.IsLoopback() && conn.Dest.IsLoopback()) ||
			(conn.IPTranslation != nil && conn.IPTranslation.ReplSrcIP.IsLoopback()) {
			conn.IntraHost = true
		} else {
			keyWithRAddr := newConnKey(conn, true)
			_, conn.IntraHost = lAddrs[keyWithRAddr]
		}

		fixConnectionDirection(conn)

		if conn.IntraHost &&
			conn.Direction == INCOMING &&
			conn.IPTranslation != nil {
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

			// check if this connection is also dnat'ed before
			// zero'ing out the ip translation
			// note: src/dst address/port are reversed since
			// we are looking for the outgoing side of this
			// incoming connection
			if _, ok := dnats[dnatKey{
				src:   conn.Dest,
				sport: conn.DPort,
				dst:   conn.Source,
				dport: conn.SPort,
				_type: conn.Type,
			}]; ok {
				conn.IPTranslation = nil
			}
		}
	})
}

// fixConnectionDirection fixes connection direction
// for UDP incoming connections.
//
// Some UDP connections can be assigned an incoming
// direction incorrectly since we cannot reliably
// distinguish between a server and client for UDP
// in eBPF. Both clients and servers can call
// the system call bind() for source ports, but
// UDP servers don't call listen() or accept()
// like TCP.
//
// This function fixes only a very specific case:
// incoming UDP connections, when the source
// port is ephemeral but the destination port is not.
// This is the only case where we can be sure the
// connection has the incorrect direction of
// incoming. For remote connections, only
// destination ports < 1024 are considered
// non-ephemeral.
func fixConnectionDirection(c *ConnectionStats) {
	// fix only incoming UDP connections
	if c.Direction != INCOMING || c.Type != UDP {
		return
	}

	sourceEphemeral := IsPortInEphemeralRange(c.Family, c.Type, c.SPort) == EphemeralTrue
	var destNotEphemeral bool
	if c.IntraHost {
		destNotEphemeral = IsPortInEphemeralRange(c.Family, c.Type, c.DPort) != EphemeralTrue
	} else {
		// use a much more restrictive range
		// for non-ephemeral ports if the
		// connection is not local
		destNotEphemeral = c.DPort < 1024
	}
	if sourceEphemeral && destNotEphemeral {
		c.Direction = OUTGOING
		stateTelemetry.udpDirectionFixes.Inc()
	}
}

type aggregateConnection struct {
	*ConnectionStats
	rttSum, rttVarSum uint64
	count             uint32
}

type connectionAggregator struct {
	conns map[string][]*aggregateConnection
	buf   []byte
}

func newConnectionAggregator(size int) *connectionAggregator {
	return &connectionAggregator{
		conns: make(map[string][]*aggregateConnection, size),
		buf:   make([]byte, ConnectionByteKeyMaxLen),
	}
}

func (a *connectionAggregator) canAggregateIPTranslation(t1, t2 *IPTranslation) bool {
	return t1 == t2 || t1 == nil || t2 == nil || *t1 == *t2
}

func (a *connectionAggregator) canAggregateProtocolStack(p1, p2 protocols.Stack) bool {
	return p1.IsUnknown() || p2.IsUnknown() || p1 == p2
}

// Aggregate aggregates a connection. The connection is only
// aggregated if:
// - it is not in the collection
// - it is in the collection and:
//   - the ip translation is nil OR
//   - the other connection's ip translation is nil OR
//   - the other connection's ip translation is not nil AND the nat info is the same
//   - the protocol stack is all unknown OR
//   - the other connection's protocol stack is unknown
//   - the other connection's protocol stack is not unknown AND equal
func (a *connectionAggregator) Aggregate(c *ConnectionStats) bool {
	key := string(c.ByteKey(a.buf))
	aggrConns, ok := a.conns[key]
	if !ok {
		a.conns[key] = []*aggregateConnection{
			{
				ConnectionStats: c,
				rttSum:          uint64(c.RTT),
				rttVarSum:       uint64(c.RTTVar),
				count:           1,
			}}

		return false
	}

	for _, aggrConn := range aggrConns {
		if !a.canAggregateIPTranslation(aggrConn.IPTranslation, c.IPTranslation) ||
			!a.canAggregateProtocolStack(aggrConn.ProtocolStack, c.ProtocolStack) {
			continue
		}

		aggrConn.Monotonic = aggrConn.Monotonic.Add(c.Monotonic)
		aggrConn.Last = aggrConn.Last.Add(c.Last)
		aggrConn.rttSum += uint64(c.RTT)
		aggrConn.rttVarSum += uint64(c.RTTVar)
		aggrConn.count++
		if aggrConn.LastUpdateEpoch < c.LastUpdateEpoch {
			aggrConn.LastUpdateEpoch = c.LastUpdateEpoch
		}
		if aggrConn.IPTranslation == nil {
			aggrConn.IPTranslation = c.IPTranslation
		}
		aggrConn.ProtocolStack.MergeWith(c.ProtocolStack)

		return true
	}

	a.conns[key] = append(aggrConns, &aggregateConnection{
		ConnectionStats: c,
		rttSum:          uint64(c.RTT),
		rttVarSum:       uint64(c.RTTVar),
		count:           1,
	})

	return false
}

func (a *connectionAggregator) finalize() {
	for _, aggrConns := range a.conns {
		for _, c := range aggrConns {
			c.RTT = uint32(c.rttSum / uint64(c.count))
			c.RTTVar = uint32(c.rttVarSum / uint64(c.count))
		}
	}
}

func (ns *networkState) mergeConnectionStats(a, b *ConnectionStats) (collision bool) {
	if a.Cookie != b.Cookie {
		return false
	}

	if !bytes.Equal(a.ByteKey(ns.mergeStatsBuffers[0]), b.ByteKey(ns.mergeStatsBuffers[1])) {
		log.Debugf("cookie collision for connections %+v and %+v", a, b)
		// cookie collision
		return true
	}

	a.Monotonic = a.Monotonic.Max(b.Monotonic)

	if b.LastUpdateEpoch > a.LastUpdateEpoch {
		a.LastUpdateEpoch = b.LastUpdateEpoch
	}

	if a.IPTranslation == nil {
		a.IPTranslation = b.IPTranslation
	}

	a.ProtocolStack.MergeWith(b.ProtocolStack)

	return false
}
