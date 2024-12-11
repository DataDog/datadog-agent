// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/cihub/seelog"
	"go4.org/intern"

	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/postgres"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/redis"
	"github.com/DataDog/datadog-agent/pkg/network/slice"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	_ State = &networkState{}
)

// Telemetry
var stateTelemetry = struct {
	closedConnDropped      *telemetry.StatCounterWrapper
	connDropped            *telemetry.StatCounterWrapper
	statsUnderflows        *telemetry.StatCounterWrapper
	statsCookieCollisions  *telemetry.StatCounterWrapper
	timeSyncCollisions     *telemetry.StatCounterWrapper
	dnsStatsDropped        *telemetry.StatCounterWrapper
	httpStatsDropped       *telemetry.StatCounterWrapper
	http2StatsDropped      *telemetry.StatCounterWrapper
	kafkaStatsDropped      *telemetry.StatCounterWrapper
	postgresStatsDropped   *telemetry.StatCounterWrapper
	redisStatsDropped      *telemetry.StatCounterWrapper
	dnsPidCollisions       *telemetry.StatCounterWrapper
	incomingDirectionFixes telemetry.Counter
	outgoingDirectionFixes telemetry.Counter
}{
	telemetry.NewStatCounterWrapper(stateModuleName, "closed_conn_dropped", []string{"ip_proto"}, "Counter measuring the number of dropped closed connections"),
	telemetry.NewStatCounterWrapper(stateModuleName, "conn_dropped", []string{}, "Counter measuring the number of closed connections"),
	telemetry.NewStatCounterWrapper(stateModuleName, "stats_underflows", []string{}, "Counter measuring the number of stats underflows"),
	telemetry.NewStatCounterWrapper(stateModuleName, "stats_cookie_collisions", []string{}, "Counter measuring the number of stats cookie collisions"),
	telemetry.NewStatCounterWrapper(stateModuleName, "time_sync_collisions", []string{}, "Counter measuring the number of time sync collisions"),
	telemetry.NewStatCounterWrapper(stateModuleName, "dns_stats_dropped", []string{}, "Counter measuring the number of DNS stats dropped"),
	telemetry.NewStatCounterWrapper(stateModuleName, "http_stats_dropped", []string{}, "Counter measuring the number of http stats dropped"),
	telemetry.NewStatCounterWrapper(stateModuleName, "http2_stats_dropped", []string{}, "Counter measuring the number of http2 stats dropped"),
	telemetry.NewStatCounterWrapper(stateModuleName, "kafka_stats_dropped", []string{}, "Counter measuring the number of kafka stats dropped"),
	telemetry.NewStatCounterWrapper(stateModuleName, "postgres_stats_dropped", []string{}, "Counter measuring the number of postgres stats dropped"),
	telemetry.NewStatCounterWrapper(stateModuleName, "redis_stats_dropped", []string{}, "Counter measuring the number of redis stats dropped"),
	telemetry.NewStatCounterWrapper(stateModuleName, "dns_pid_collisions", []string{}, "Counter measuring the number of DNS PID collisions"),
	telemetry.NewCounter(stateModuleName, "incoming_direction_fixes", []string{}, "Counter measuring the number of udp direction fixes for incoming connections"),
	telemetry.NewCounter(stateModuleName, "outgoing_direction_fixes", []string{}, "Counter measuring the number of udp/tcp direction fixes for outgoing connections"),
}

const (
	// DEBUGCLIENT is the ClientID for debugging
	DEBUGCLIENT = "-1"

	// DNSResponseCodeNoError is the value that indicates that the DNS reply contains no errors.
	// We could have used layers.DNSResponseCodeNoErr here. But importing the gopacket library only for this
	// constant is not worth the increased memory cost.
	DNSResponseCodeNoError = 0

	stateModuleName = "network_tracer__state"

	shortLivedConnectionThreshold = 2 * time.Minute
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

	// StoreClosedConnection stores a batch of closed connections
	StoreClosedConnection(connection *ConnectionStats)

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
	Kafka    map[kafka.Key]*kafka.RequestStats
	Postgres map[postgres.Key]*postgres.RequestStat
	Redis    map[redis.Key]*redis.RequestStat
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
	postgresStatsDropped  int64
	redisStatsDropped     int64
	dnsPidCollisions      int64
}

const minClosedCapacity = 1024

type closedConnections struct {
	// conns are ordered by placing all the empty connections at the end of the slice
	conns []ConnectionStats
	// byCookie is used to search for the index of a ConnectionStats in conns
	byCookie map[StatCookie]int
	// the index of first empty connection in conns
	emptyStart int
}

// Inserts a connection into conns and byCookie:
// This function checks whether conns has reached the maxClosedConns limit. If it has, it drops an empty connection.
// If the limit has not been reached, it places the connection in conns.
// All empty connections are placed at the end. If it is not empty, it will be placed
// at the index of the first empty connection, and the first empty connection will be placed at the end.
// If there are no empty connections, it will be appended at the end.
func (cc *closedConnections) insert(c *ConnectionStats, maxClosedConns uint32) {
	// If we have reached the limit, drop an empty connection
	if uint32(len(cc.conns)) >= maxClosedConns {
		stateTelemetry.closedConnDropped.IncWithTags(c.Type.Tags())
		cc.dropEmpty(c)
		return
	}
	// If the connection is empty append at the end
	if c.IsEmpty() {
		cc.conns = append(cc.conns, *c)
		cc.byCookie[c.Cookie] = len(cc.conns) - 1
		return
	}

	// Insert the connection before empty connections
	if cc.emptyStart < len(cc.conns) {
		emptyConn := cc.conns[cc.emptyStart]
		cc.conns[cc.emptyStart] = *c
		cc.conns = append(cc.conns, emptyConn)
		cc.byCookie[c.Cookie] = cc.emptyStart
		cc.byCookie[emptyConn.Cookie] = len(cc.conns) - 1
		cc.emptyStart++
		return
	}
	// If there are no empty connections, append at the end
	cc.conns = append(cc.conns, *c)
	cc.byCookie[c.Cookie] = len(cc.conns) - 1
	cc.emptyStart = len(cc.conns)
}

// Drops the first empty connection:
// This method drops the incoming connection if it's empty or there are no empty connections in conns.
// If neither of these conditions are true, it will drop the first empty connection and replace it with
// the incoming connection.
func (cc *closedConnections) dropEmpty(c *ConnectionStats) {
	if c.IsEmpty() || cc.emptyStart == len(cc.conns) {
		return
	}
	delete(cc.byCookie, cc.conns[cc.emptyStart].Cookie)
	cc.conns[cc.emptyStart] = *c
	cc.byCookie[c.Cookie] = cc.emptyStart
	cc.emptyStart++
}

// Replaces connection c with the connection at index i:
// If the conn at i is the latest, or c is empty and the connection at i is not,
// it will not complete the replacement.
// Otherwise it checks if the connection at i is empty and will be replaced with a non-empty conn.
// If this is true, it will replace the connection and move it to where the first empty conn is.
// If there isn't a change of state (both are empty or non-empty) it will simply replace the conn.
func (cc *closedConnections) replaceAt(i int, c *ConnectionStats) {
	// pick the latest one
	if c.LastUpdateEpoch <= cc.conns[i].LastUpdateEpoch {
		return
	}
	// If c is empty and connn[i] is not, do not replace
	if c.IsEmpty() && i < cc.emptyStart {
		return
	}
	// If conn[i] is empty and c is not, replace with the first empty connection
	if !c.IsEmpty() && i >= cc.emptyStart {
		cc.conns[cc.emptyStart], cc.conns[i] = cc.conns[i], cc.conns[cc.emptyStart]
		cc.byCookie[cc.conns[i].Cookie] = i
		cc.conns[cc.emptyStart] = *c
		cc.byCookie[c.Cookie] = cc.emptyStart
		cc.emptyStart++
		return
	}
	cc.conns[i] = *c
}

type client struct {
	lastFetch time.Time
	closed    *closedConnections
	stats     map[StatCookie]StatCounters
	// maps by dns key the domain (string) to stats structure
	dnsStats           dns.StatsByKeyByNameByType
	httpStatsDelta     map[http.Key]*http.RequestStats
	http2StatsDelta    map[http.Key]*http.RequestStats
	kafkaStatsDelta    map[kafka.Key]*kafka.RequestStats
	postgresStatsDelta map[postgres.Key]*postgres.RequestStat
	redisStatsDelta    map[redis.Key]*redis.RequestStat
	lastTelemetries    map[ConnTelemetryType]int64
}

func (c *client) Reset() {
	half := cap(c.closed.conns) / 2
	if closedLen := len(c.closed.conns); closedLen > minClosedCapacity && closedLen < half {
		c.closed.conns = make([]ConnectionStats, half)
	}

	c.closed.conns = c.closed.conns[:0]
	c.closed.byCookie = make(map[StatCookie]int)
	c.dnsStats = make(dns.StatsByKeyByNameByType)
	c.httpStatsDelta = make(map[http.Key]*http.RequestStats)
	c.http2StatsDelta = make(map[http.Key]*http.RequestStats)
	c.kafkaStatsDelta = make(map[kafka.Key]*kafka.RequestStats)
	c.postgresStatsDelta = make(map[postgres.Key]*postgres.RequestStat)
	c.redisStatsDelta = make(map[redis.Key]*redis.RequestStat)
}

type networkState struct {
	sync.Mutex

	// clients is a map of the connection id string to the client structure
	clients       map[string]*client
	lastTelemetry lastStateTelemetry // Old telemetry state; used for logging

	latestTimeEpoch uint64

	// Network state configuration
	clientExpiry                time.Duration
	maxClosedConns              uint32
	maxClientStats              int
	maxDNSStats                 int
	maxHTTPStats                int
	maxKafkaStats               int
	maxPostgresStats            int
	maxRedisStats               int
	enableConnectionRollup      bool
	processEventConsumerEnabled bool

	localResolver LocalResolver
}

// NewState creates a new network state
func NewState(_ telemetryComponent.Component, clientExpiry time.Duration, maxClosedConns uint32, maxClientStats, maxDNSStats, maxHTTPStats, maxKafkaStats, maxPostgresStats, maxRedisStats int, enableConnectionRollup bool, processEventConsumerEnabled bool) State {
	ns := &networkState{
		clients:                     map[string]*client{},
		clientExpiry:                clientExpiry,
		maxClosedConns:              maxClosedConns,
		maxClientStats:              maxClientStats,
		maxDNSStats:                 maxDNSStats,
		maxHTTPStats:                maxHTTPStats,
		maxKafkaStats:               maxKafkaStats,
		maxPostgresStats:            maxPostgresStats,
		maxRedisStats:               maxRedisStats,
		enableConnectionRollup:      enableConnectionRollup,
		localResolver:               NewLocalResolver(processEventConsumerEnabled),
		processEventConsumerEnabled: processEventConsumerEnabled,
	}

	if ns.enableConnectionRollup && !processEventConsumerEnabled {
		log.Warnf("disabling port rollups since network event consumer is not enabled")
		ns.enableConnectionRollup = false
	}

	return ns
}

//nolint:unused // TODO(NET) Fix unused linter
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

	cs := slice.NewChain(active, closed)
	ns.determineConnectionIntraHost(cs)

	// resolve local connections if rollups are enabled
	if ns.enableConnectionRollup {
		ns.localResolver.Resolve(cs)
	}

	if len(dnsStats) > 0 {
		ns.storeDNSStats(dnsStats)
	}

	aggr := newConnectionAggregator((len(closed)+len(active))/2, ns.enableConnectionRollup, ns.processEventConsumerEnabled, client.dnsStats)
	active = filterConnections(active, func(c *ConnectionStats) bool {
		return !aggr.Aggregate(c)
	})

	closed = filterConnections(closed, func(c *ConnectionStats) bool {
		return !aggr.Aggregate(c)
	})

	aggr.finalize()

	for protocolType, protocolStats := range usmStats {
		switch protocolType {
		case protocols.HTTP:
			stats := protocolStats.(map[http.Key]*http.RequestStats)
			ns.storeHTTPStats(stats)
		case protocols.Kafka:
			stats := protocolStats.(map[kafka.Key]*kafka.RequestStats)
			ns.storeKafkaStats(stats)
		case protocols.HTTP2:
			stats := protocolStats.(map[http.Key]*http.RequestStats)
			ns.storeHTTP2Stats(stats)
		case protocols.Postgres:
			stats := protocolStats.(map[postgres.Key]*postgres.RequestStat)
			ns.storePostgresStats(stats)
		case protocols.Redis:
			stats := protocolStats.(map[redis.Key]*redis.RequestStat)
			ns.storeRedisStats(stats)
		}
	}

	return Delta{
		Conns:    append(active, closed...),
		HTTP:     client.httpStatsDelta,
		HTTP2:    client.http2StatsDelta,
		Kafka:    client.kafkaStatsDelta,
		Postgres: client.postgresStatsDelta,
		Redis:    client.redisStatsDelta,
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
	postgresStatsDroppedDelta := stateTelemetry.postgresStatsDropped.Load() - ns.lastTelemetry.postgresStatsDropped
	redisStatsDroppedDelta := stateTelemetry.redisStatsDropped.Load() - ns.lastTelemetry.redisStatsDropped
	dnsPidCollisionsDelta := stateTelemetry.dnsPidCollisions.Load() - ns.lastTelemetry.dnsPidCollisions

	// Flush log line if any metric is non-zero
	if connDroppedDelta > 0 || closedConnDroppedDelta > 0 || dnsStatsDroppedDelta > 0 || httpStatsDroppedDelta > 0 ||
		http2StatsDroppedDelta > 0 || kafkaStatsDroppedDelta > 0 || postgresStatsDroppedDelta > 0 || redisStatsDroppedDelta > 0 {
		s := "State telemetry: "
		s += " [%d connections dropped due to stats]"
		s += " [%d closed connections dropped]"
		s += " [%d DNS stats dropped]"
		s += " [%d HTTP stats dropped]"
		s += " [%d HTTP2 stats dropped]"
		s += " [%d Kafka stats dropped]"
		s += " [%d postgres stats dropped]"
		s += " [%d redis stats dropped]"
		log.Warnf(s,
			connDroppedDelta,
			closedConnDroppedDelta,
			dnsStatsDroppedDelta,
			httpStatsDroppedDelta,
			http2StatsDroppedDelta,
			kafkaStatsDroppedDelta,
			postgresStatsDroppedDelta,
			redisStatsDroppedDelta,
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
	ns.lastTelemetry.postgresStatsDropped = stateTelemetry.postgresStatsDropped.Load()
	ns.lastTelemetry.redisStatsDropped = stateTelemetry.redisStatsDropped.Load()
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

// StoreClosedConnection wraps the unexported method while locking state
func (ns *networkState) StoreClosedConnection(closed *ConnectionStats) {
	ns.Lock()
	defer ns.Unlock()

	ns.storeClosedConnection(closed)
}

// storeClosedConnection stores the given connection for every client
func (ns *networkState) storeClosedConnection(c *ConnectionStats) {
	for _, client := range ns.clients {
		if i, ok := client.closed.byCookie[c.Cookie]; ok {
			if ns.mergeConnectionStats(&client.closed.conns[i], c) {
				stateTelemetry.statsCookieCollisions.Inc()
				client.closed.replaceAt(i, c)
			}
			continue
		}
		client.closed.insert(c, ns.maxClosedConns)
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
func (ns *networkState) storeKafkaStats(allStats map[kafka.Key]*kafka.RequestStats) {
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

// storePostgresStats stores the latest Postgres stats for all clients
func (ns *networkState) storePostgresStats(allStats map[postgres.Key]*postgres.RequestStat) {
	if len(ns.clients) == 1 {
		for _, client := range ns.clients {
			if len(client.postgresStatsDelta) == 0 && len(allStats) <= ns.maxPostgresStats {
				// optimization for the common case:
				// if there is only one client and no previous state, no memory allocation is needed
				client.postgresStatsDelta = allStats
				return
			}
		}
	}

	for key, stats := range allStats {
		for _, client := range ns.clients {
			prevStats, ok := client.postgresStatsDelta[key]
			if !ok && len(client.postgresStatsDelta) >= ns.maxPostgresStats {
				stateTelemetry.postgresStatsDropped.Inc()
				continue
			}

			if prevStats != nil {
				prevStats.CombineWith(stats)
				client.postgresStatsDelta[key] = prevStats
			} else {
				client.postgresStatsDelta[key] = stats
			}
		}
	}
}

// storeRedisStats stores the latest Redis stats for all clients
func (ns *networkState) storeRedisStats(allStats map[redis.Key]*redis.RequestStat) {
	if len(ns.clients) == 1 {
		for _, client := range ns.clients {
			if len(client.redisStatsDelta) == 0 && len(allStats) <= ns.maxRedisStats {
				// optimization for the common case:
				// if there is only one client and no previous state, no memory allocation is needed
				client.redisStatsDelta = allStats
				return
			}
		}
	}

	for key, stats := range allStats {
		for _, client := range ns.clients {
			prevStats, ok := client.redisStatsDelta[key]
			if !ok && len(client.redisStatsDelta) >= ns.maxRedisStats {
				stateTelemetry.redisStatsDropped.Inc()
				continue
			}

			if prevStats != nil {
				prevStats.CombineWith(stats)
				client.redisStatsDelta[key] = prevStats
			} else {
				client.redisStatsDelta[key] = stats
			}
		}
	}
}

func (ns *networkState) getClient(clientID string) *client {
	if c, ok := ns.clients[clientID]; ok {
		return c
	}
	closedConnections := &closedConnections{conns: make([]ConnectionStats, 0, minClosedCapacity), byCookie: make(map[StatCookie]int)}
	c := &client{
		lastFetch:          time.Now(),
		stats:              make(map[StatCookie]StatCounters),
		closed:             closedConnections,
		dnsStats:           dns.StatsByKeyByNameByType{},
		httpStatsDelta:     map[http.Key]*http.RequestStats{},
		http2StatsDelta:    map[http.Key]*http.RequestStats{},
		kafkaStatsDelta:    map[kafka.Key]*kafka.RequestStats{},
		postgresStatsDelta: map[postgres.Key]*postgres.RequestStat{},
		redisStatsDelta:    map[redis.Key]*redis.RequestStat{},
		lastTelemetries:    make(map[ConnTelemetryType]int64),
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
	closed = filterConnections(client.closed.conns, func(closedConn *ConnectionStats) bool {
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

		//nolint:gosimple // TODO(NET) Fix gosimple linter
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

		if dropped := ns.createStatsForCookie(client, c.Cookie); dropped {
			return false
		}

		ns.updateConnWithStats(client, c.Cookie, c)
		newStats[c.Cookie] = client.stats[c.Cookie]
		return !c.Last.IsZero() // not reporting an "empty" connection
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
func (ns *networkState) createStatsForCookie(client *client, cookie StatCookie) (dropped bool) {
	if _, ok := client.stats[cookie]; !ok {
		if len(client.stats) >= ns.maxClientStats {
			stateTelemetry.connDropped.Inc()
			return true
		}

		client.stats[cookie] = StatCounters{}
	}

	return false
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
			log.Debugf("expiring client: %s, had %d stats and %d closed connections", id, len(c.stats), len(c.closed.conns))
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
			"closed_connections": len(c.closed.conns),
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
		(c.IPTranslation.ReplSrcIP != c.Dest || c.IPTranslation.ReplSrcPort != c.DPort)
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

	dnatCount := 0
	lAddrs := make(map[connKey]struct{}, connections.Len())
	connections.Iterate(func(_ int, conn *ConnectionStats) {
		k := newConnKey(conn, false)
		lAddrs[k] = struct{}{}

		if isDNAT(conn) {
			dnatCount++
		}
	})

	dnats := make(map[dnatKey]struct{}, dnatCount)
	connections.Iterate(func(_ int, conn *ConnectionStats) {
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

		switch conn.Direction {
		case OUTGOING:
			fixOutgoingConnectionDirection(conn)
		case INCOMING:
			fixIncomingConnectionDirection(conn)
		}

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

// fixIncomingConnectionDirection fixes connection direction
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
func fixIncomingConnectionDirection(c *ConnectionStats) {
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
		stateTelemetry.incomingDirectionFixes.Inc()
	}
}

// fixOutgoingConnectionDirection potentially fixes the direction for outgoing connections
//
// When the system-probe starts up there is a race that occurs where port mappings are missing when they
// are bound after the `/proc/net` state is read but before the probes are registered.  When that happens, the
// connections will be marked as outgoing
//
// This function attempts to mitigate that by checking if an outgoing connection is from a non-ephemeral port to
// an ephemeral port on an intra-host connection
func fixOutgoingConnectionDirection(c *ConnectionStats) {
	if c.Direction != OUTGOING || !c.IntraHost {
		return
	}

	sourceNotEphemeral := IsPortInEphemeralRange(c.Family, c.Type, c.SPort) != EphemeralTrue
	destEphemeral := IsPortInEphemeralRange(c.Family, c.Type, c.DPort) == EphemeralTrue

	if sourceNotEphemeral && destEphemeral {
		c.Direction = INCOMING
		stateTelemetry.outgoingDirectionFixes.Inc()
	}
}

type aggregateConnection struct {
	*ConnectionStats
	rttSum, rttVarSum uint64
	count             uint32
}

type aggregationKey struct {
	connKey    ConnectionTuple
	direction  ConnectionDirection
	containers struct {
		source, dest *intern.Value
	}
}

type connectionAggregator struct {
	conns                       map[aggregationKey][]*aggregateConnection
	dnsStats                    dns.StatsByKeyByNameByType
	enablePortRollups           bool
	processEventConsumerEnabled bool
}

func newConnectionAggregator(size int, enablePortRollups, processEventConsumerEnabled bool, dnsStats dns.StatsByKeyByNameByType) *connectionAggregator {
	return &connectionAggregator{
		conns:                       make(map[aggregationKey][]*aggregateConnection, size),
		dnsStats:                    dnsStats,
		enablePortRollups:           enablePortRollups,
		processEventConsumerEnabled: processEventConsumerEnabled,
	}
}

func (a *connectionAggregator) key(c *ConnectionStats) (key aggregationKey, sportRolledUp, dportRolledUp bool) {
	key.connKey = c.ConnectionTuple
	key.direction = c.Direction
	if a.processEventConsumerEnabled {
		key.containers.source = c.ContainerID.Source
	}

	if !a.enablePortRollups {
		return key, false, false
	}

	// local resolution is done in system-probe if rollups
	// are enabled, so add the destination container id to
	// the key as well
	key.containers.dest = c.ContainerID.Dest

	isShortLived := c.IsClosed && (c.Duration > 0 && c.Duration < shortLivedConnectionThreshold)
	sportRolledUp = c.Direction == OUTGOING
	dportRolledUp = c.Direction == INCOMING

	log.TraceFunc(func() string {
		return fmt.Sprintf("type=%s isShortLived=%+v sportRolledUp=%+v", c.Type, isShortLived, sportRolledUp)
	})
	if !isShortLived ||
		(!sportRolledUp && !dportRolledUp) {
		log.TraceFunc(func() string { return fmt.Sprintf("not rolling up connection %+v ", c) })
		return key, false, false
	}

	log.TraceFunc(func() string { return fmt.Sprintf("rolling up connection %+v ", c) })

	if sportRolledUp {
		sport := c.SPort
		// set source port to 0 temporarily for key generation
		c.SPort = 0
		defer func() {
			c.SPort = sport
		}()
	}

	if dportRolledUp {
		dport := c.DPort
		// set dest port to 0 temporarily for key generation
		c.DPort = 0
		defer func() {
			c.DPort = dport
		}()
	}

	key.connKey = c.ConnectionTuple
	return key, sportRolledUp, dportRolledUp
}

func (a *connectionAggregator) canAggregateIPTranslation(t1, t2 *IPTranslation, sportRolledUp, dportRolledUp bool) bool {
	if t1 == t2 || t1 == nil || t2 == nil || *t1 == *t2 {
		return true
	}

	// *t1 != *t2
	if !sportRolledUp && !dportRolledUp {
		return false
	}

	if sportRolledUp {
		d1, d2 := t1.ReplDstPort, t2.ReplDstPort
		t1.ReplDstPort, t2.ReplDstPort = 0, 0
		defer func() {
			t1.ReplDstPort, t2.ReplDstPort = d1, d2
		}()
	}

	if dportRolledUp {
		s1, s2 := t1.ReplSrcPort, t2.ReplSrcPort
		t1.ReplSrcPort, t2.ReplSrcPort = 0, 0
		defer func() {
			t1.ReplSrcPort, t2.ReplSrcPort = s1, s2
		}()
	}

	return *t1 == *t2
}

func (a *connectionAggregator) canAggregateProtocolStack(p1, p2 protocols.Stack) bool {
	return p1.IsUnknown() || p2.IsUnknown() || p1 == p2
}

func (a *connectionAggregator) dns(c *ConnectionStats) map[dns.Hostname]map[dns.QueryType]dns.Stats {
	key, isDNS := DNSKey(c)
	if !isDNS {
		return nil
	}

	if stats, ok := a.dnsStats[key]; ok {
		delete(a.dnsStats, key)
		return stats
	}

	return nil
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
	key, sportRolledUp, dportRolledUp := a.key(c)

	// get dns stats for connection
	c.DNSStats = a.dns(c)

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
		if !a.canAggregateIPTranslation(aggrConn.IPTranslation, c.IPTranslation, sportRolledUp, dportRolledUp) ||
			!a.canAggregateProtocolStack(aggrConn.ProtocolStack, c.ProtocolStack) {
			continue
		}

		aggrConn.merge(c)

		if sportRolledUp {
			// more than one connection with
			// source port dropped in key,
			// so set source port to 0
			aggrConn.SPort = 0
			if aggrConn.IPTranslation != nil {
				aggrConn.IPTranslation.ReplDstPort = 0
			}
		}
		if dportRolledUp {
			// more than one connection with
			// dest port dropped in key,
			// so set dest port to 0
			aggrConn.DPort = 0
			if aggrConn.IPTranslation != nil {
				aggrConn.IPTranslation.ReplSrcPort = 0
			}
		}

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

func (ac *aggregateConnection) merge(c *ConnectionStats) {
	ac.Monotonic = ac.Monotonic.Add(c.Monotonic)
	ac.Last = ac.Last.Add(c.Last)
	ac.rttSum += uint64(c.RTT)
	ac.rttVarSum += uint64(c.RTTVar)
	ac.count++
	if ac.LastUpdateEpoch < c.LastUpdateEpoch {
		ac.LastUpdateEpoch = c.LastUpdateEpoch
	}
	if ac.IPTranslation == nil {
		ac.IPTranslation = c.IPTranslation
	}

	ac.ProtocolStack.MergeWith(c.ProtocolStack)

	if ac.DNSStats == nil {
		ac.DNSStats = c.DNSStats
	} else {
		for hostname, statsByQuery := range c.DNSStats {
			hostStats := ac.DNSStats[hostname]
			if hostStats == nil {
				hostStats = make(map[dns.QueryType]dns.Stats)
				ac.DNSStats[hostname] = hostStats
			}
			for q, stats := range statsByQuery {
				queryStats, ok := hostStats[q]
				if !ok {
					hostStats[q] = stats
					continue
				}

				queryStats.FailureLatencySum += stats.FailureLatencySum
				queryStats.SuccessLatencySum += stats.SuccessLatencySum
				queryStats.Timeouts += stats.Timeouts
				for rcode, count := range stats.CountByRcode {
					queryStats.CountByRcode[rcode] += count
				}
				hostStats[q] = queryStats
			}
		}
	}

	// no need to hold on to dns stats on the aggregated connection
	c.DNSStats = nil
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

	if a.ConnectionTuple != b.ConnectionTuple {
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
