package network

import (
	"bytes"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	_ State = &networkState{}
)

const (
	// DEBUGCLIENT is the ClientID for debugging
	DEBUGCLIENT = "-1"
)

// State takes care of handling the logic for:
// - closed connections
// - sent and received bytes per connection
type State interface {
	// Connections returns the list of connections for the given client when provided the latest set of active connections
	Connections(clientID string, latestTime uint64, latestConns []ConnectionStats) []ConnectionStats

	// StoreClosedConnection stores a new closed connection
	StoreClosedConnection(conn ConnectionStats)

	// RemoveClient stops tracking stateful data for a given client
	RemoveClient(clientID string)

	// RemoveExpiredClients removes expired clients from the state
	RemoveExpiredClients(now time.Time)

	// RemoveConnections removes the given keys from the state
	RemoveConnections(keys []string)

	// GetStats returns a map of statistics about the current network state
	GetStats() map[string]interface{}

	// DebugState returns a map with the current network state for a client ID
	DumpState(clientID string) map[string]interface{}
}

type telemetry struct {
	unorderedConns     int64
	closedConnDropped  int64
	connDropped        int64
	statsResets        int64
	timeSyncCollisions int64
}

type stats struct {
	totalSent        uint64
	totalRecv        uint64
	totalRetransmits uint32
}

type client struct {
	lastFetch time.Time

	closedConnections map[string]ConnectionStats
	stats             map[string]*stats
}

type networkState struct {
	sync.Mutex

	clients   map[string]*client
	telemetry telemetry

	buf             *bytes.Buffer // Shared buffer
	latestTimeEpoch uint64

	// Network state configuration
	clientExpiry   time.Duration
	maxClosedConns int
	maxClientStats int
}

// NewDefaultState creates a new network state with default settings
func NewDefaultState() State {
	defaultC := NewDefaultConfig()
	return NewState(defaultC.ClientStateExpiry, defaultC.MaxClosedConnectionsBuffered, defaultC.MaxConnectionsStateBuffered)
}

// NewState creates a new network state
func NewState(clientExpiry time.Duration, maxClosedConns, maxClientStats int) State {
	return &networkState{
		clients:        map[string]*client{},
		telemetry:      telemetry{},
		clientExpiry:   clientExpiry,
		maxClosedConns: maxClosedConns,
		maxClientStats: maxClientStats,
		buf:            &bytes.Buffer{},
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

// Connections returns the connections for the given client
// If the client is not registered yet, we register it and return the connections we have in the global state
// Otherwise we return both the connections with last stats and the closed connections for this client
func (ns *networkState) Connections(id string, latestTime uint64, latestConns []ConnectionStats) []ConnectionStats {
	ns.Lock()
	defer ns.Unlock()

	// Update the latest known time
	ns.latestTimeEpoch = latestTime
	connsByKey := getConnsByKey(latestConns, ns.buf)

	// If its the first time we've seen this client, use global state as connection set
	if client, ok := ns.newClient(id); !ok {
		for key, c := range connsByKey {
			ns.createStatsForKey(client, key)
			ns.updateConnWithStats(client, key, c)

			// We force last stats to be 0 on a new client this is purely to
			// have a coherent definition of LastXYZ and should not have an impact
			// on collection since we drop the first get in the process-agent
			c.LastSentBytes = 0
			c.LastRecvBytes = 0
			c.LastRetransmits = 0
		}

		ns.determineConnectionIntraHost(latestConns)
		return latestConns
	}

	// Update all connections with relevant up-to-date stats for client
	conns := ns.mergeConnections(id, connsByKey)

	// XXX: we should change the way we clean this map once
	// https://github.com/golang/go/issues/20135 is solved
	newStats := make(map[string]*stats, len(ns.clients[id].stats))
	for key, st := range ns.clients[id].stats {
		// Only keep active connections stats
		if _, isActive := connsByKey[key]; isActive {
			newStats[key] = st
		}
	}
	ns.clients[id].stats = newStats

	// Flush closed connection map and stats
	ns.clients[id].closedConnections = map[string]ConnectionStats{}
	ns.determineConnectionIntraHost(conns)

	return conns
}

// getConnsByKey returns a mapping of byte-key -> connection for easier access + manipulation
func getConnsByKey(conns []ConnectionStats, buf *bytes.Buffer) map[string]*ConnectionStats {
	connsByKey := make(map[string]*ConnectionStats, len(conns))
	for i, c := range conns {
		key, err := c.ByteKey(buf)
		if err != nil {
			log.Warnf("failed to create byte key: %s", err)
			continue
		}
		connsByKey[string(key)] = &conns[i]
	}
	return connsByKey
}

// StoreClosedConnection stores the given connection for every client
func (ns *networkState) StoreClosedConnection(conn ConnectionStats) {
	ns.Lock()
	defer ns.Unlock()

	key, err := conn.ByteKey(ns.buf)
	if err != nil {
		log.Warnf("failed to create byte key: %s", err)
		return
	}

	for _, client := range ns.clients {
		// If we've seen this closed connection already, lets combine the two
		if prev, ok := client.closedConnections[string(key)]; ok {
			// We received either the connections either out of order, or it's the same one we've already seen.
			// Lets skip it for now.
			if prev.LastUpdateEpoch >= conn.LastUpdateEpoch {
				ns.telemetry.unorderedConns++
				continue
			}

			prev.MonotonicSentBytes += conn.MonotonicSentBytes
			prev.MonotonicRecvBytes += conn.MonotonicRecvBytes
			prev.MonotonicRetransmits += conn.MonotonicRetransmits
			// Also update the timestamp
			prev.LastUpdateEpoch = conn.LastUpdateEpoch
			client.closedConnections[string(key)] = prev
		} else if len(client.closedConnections) >= ns.maxClosedConns {
			ns.telemetry.closedConnDropped++
			continue
		} else {
			client.closedConnections[string(key)] = conn
		}
	}
}

// newClient creates a new client and returns true if the given client already exists
func (ns *networkState) newClient(clientID string) (*client, bool) {
	if c, ok := ns.clients[clientID]; ok {
		return c, true
	}

	c := &client{
		lastFetch:         time.Now(),
		stats:             map[string]*stats{},
		closedConnections: map[string]ConnectionStats{},
	}
	ns.clients[clientID] = c
	return c, false
}

// mergeConnections return the connections and takes care of updating their last stat counters
func (ns *networkState) mergeConnections(id string, active map[string]*ConnectionStats) []ConnectionStats {
	now := time.Now()

	client := ns.clients[id]
	client.lastFetch = now

	conns := make([]ConnectionStats, 0, len(active)+len(client.closedConnections))

	// Closed connections
	for key, closedConn := range client.closedConnections {
		// If the connection is also active, check the epochs to understand what's going on
		if activeConn, ok := active[key]; ok {
			// If closed conn is newer it means that the active connection is outdated, let's ignore it
			if closedConn.LastUpdateEpoch > activeConn.LastUpdateEpoch {
				ns.updateConnWithStats(client, key, &closedConn)
			} else if closedConn.LastUpdateEpoch < activeConn.LastUpdateEpoch {
				// Else if the active conn is newer, it likely means that it became active again
				// in this case we aggregate the two
				closedConn.MonotonicSentBytes += activeConn.MonotonicSentBytes
				closedConn.MonotonicRecvBytes += activeConn.MonotonicRecvBytes
				closedConn.MonotonicRetransmits += activeConn.MonotonicRetransmits

				ns.createStatsForKey(client, key)
				ns.updateConnWithStatWithActiveConn(client, key, *activeConn, &closedConn)

				// We also update the counters to reflect only the active connection
				// The monotonic counters will be the sum of all connections that cross our interval start + finish.
				if stats, ok := client.stats[key]; ok {
					stats.totalRetransmits = activeConn.MonotonicRetransmits
					stats.totalSent = activeConn.MonotonicSentBytes
					stats.totalRecv = activeConn.MonotonicRecvBytes
				}
			} else {
				// Else the closed connection and the active connection have the same epoch
				// XXX: For now we assume that the closed connection is the more recent one but this is not guaranteed
				// To fix this we should have a way to uniquely identify a connection
				// (using the startTimestamp or a monotonic counter)
				ns.telemetry.timeSyncCollisions++
				log.Tracef("Time collision for connections: closed:%+v, active:%+v", closedConn, *activeConn)
				ns.updateConnWithStats(client, key, &closedConn)
			}
		} else {
			ns.updateConnWithStats(client, key, &closedConn)
		}

		conns = append(conns, closedConn)
	}

	// Active connections
	for key, c := range active {
		// If the connection was closed, it has already been processed so skip it
		if _, ok := client.closedConnections[key]; ok {
			continue
		}

		ns.createStatsForKey(client, key)
		ns.updateConnWithStats(client, key, c)

		conns = append(conns, *c)
	}

	return conns
}

// This is used to update the stats when we process a closed connection that became active again
// in this case we want the stats to reflect the new active connections in order to avoid resets
func (ns *networkState) updateConnWithStatWithActiveConn(client *client, key string, active ConnectionStats, closed *ConnectionStats) {
	if st, ok := client.stats[key]; ok {
		// Check for underflows
		ns.handleStatsUnderflow(key, st, closed)

		closed.LastSentBytes = closed.MonotonicSentBytes - st.totalSent
		closed.LastRecvBytes = closed.MonotonicRecvBytes - st.totalRecv
		closed.LastRetransmits = closed.MonotonicRetransmits - st.totalRetransmits

		// Update stats object with latest values
		st.totalSent = active.MonotonicSentBytes
		st.totalRecv = active.MonotonicRecvBytes
		st.totalRetransmits = active.MonotonicRetransmits
	} else {
		closed.LastSentBytes = closed.MonotonicSentBytes
		closed.LastRecvBytes = closed.MonotonicRecvBytes
		closed.LastRetransmits = closed.MonotonicRetransmits
	}
}

func (ns *networkState) updateConnWithStats(client *client, key string, c *ConnectionStats) {
	if st, ok := client.stats[key]; ok {
		// Check for underflows
		ns.handleStatsUnderflow(key, st, c)

		c.LastSentBytes = c.MonotonicSentBytes - st.totalSent
		c.LastRecvBytes = c.MonotonicRecvBytes - st.totalRecv
		c.LastRetransmits = c.MonotonicRetransmits - st.totalRetransmits

		// Update stats object with latest values
		st.totalSent = c.MonotonicSentBytes
		st.totalRecv = c.MonotonicRecvBytes
		st.totalRetransmits = c.MonotonicRetransmits
	} else {
		c.LastSentBytes = c.MonotonicSentBytes
		c.LastRecvBytes = c.MonotonicRecvBytes
		c.LastRetransmits = c.MonotonicRetransmits
	}
}

// handleStatsUnderflow checks if we are going to have an underflow when computing last stats and if it's the case it resets the stats to avoid it
func (ns *networkState) handleStatsUnderflow(key string, st *stats, c *ConnectionStats) {
	if c.MonotonicSentBytes < st.totalSent || c.MonotonicRecvBytes < st.totalRecv || c.MonotonicRetransmits < st.totalRetransmits {
		ns.telemetry.statsResets++
		log.Debugf("Stats reset triggered for key:%s, stats:%+v, connection:%+v", BeautifyKey(key), *st, *c)
		st.totalSent = 0
		st.totalRecv = 0
		st.totalRetransmits = 0
	}
}

// createStatsForKey will create a new stats object for a key if it doesn't already exist.
func (ns *networkState) createStatsForKey(client *client, key string) {
	if _, ok := client.stats[key]; !ok {
		if len(client.stats) >= ns.maxClientStats {
			ns.telemetry.connDropped++
			return
		}
		client.stats[key] = &stats{}
	}
}

func (ns *networkState) RemoveClient(clientID string) {
	ns.Lock()
	defer ns.Unlock()
	delete(ns.clients, clientID)
}

func (ns *networkState) RemoveExpiredClients(now time.Time) {
	ns.Lock()
	defer ns.Unlock()

	for id, c := range ns.clients {
		if c.lastFetch.Add(ns.clientExpiry).Before(now) {
			log.Debugf("expiring client: %s, had %d stats and %d closed connections", id, len(c.stats), len(c.closedConnections))
			delete(ns.clients, id)
		}
	}
}

func (ns *networkState) RemoveConnections(keys []string) {
	ns.Lock()
	defer ns.Unlock()

	for _, c := range ns.clients {
		for _, key := range keys {
			delete(c.stats, key)
		}
	}

	// Flush log line if any metric is non zero
	if ns.telemetry.unorderedConns > 0 || ns.telemetry.statsResets > 0 || ns.telemetry.closedConnDropped > 0 || ns.telemetry.connDropped > 0 || ns.telemetry.timeSyncCollisions > 0 {
		log.Warnf("state telemetry: [%d unordered conns] [%d stats stats_resets] [%d connections dropped due to stats] [%d closed connections dropped] [%d time sync collisions]",
			ns.telemetry.unorderedConns,
			ns.telemetry.statsResets,
			ns.telemetry.closedConnDropped,
			ns.telemetry.connDropped,
			ns.telemetry.timeSyncCollisions)
	}

	ns.telemetry = telemetry{}
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
			"stats_resets":         ns.telemetry.statsResets,
			"unordered_conns":      ns.telemetry.unorderedConns,
			"closed_conn_dropped":  ns.telemetry.closedConnDropped,
			"conn_dropped":         ns.telemetry.connDropped,
			"time_sync_collisions": ns.telemetry.timeSyncCollisions,
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
			data[BeautifyKey(connKey)] = map[string]uint64{
				"total_sent":        s.totalSent,
				"total_recv":        s.totalRecv,
				"total_retransmits": uint64(s.totalRetransmits),
			}
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

	lAddrs := make(map[connKey]struct{})
	for _, conn := range connections {
		lAddrs[newConnKey(&conn, false)] = struct{}{}
	}

	for i := range connections {
		conn := &connections[i]
		keyWithRAddr := newConnKey(conn, true)

		if conn.Source == conn.Dest || (conn.Source.IsLoopback() && conn.Dest.IsLoopback()) {
			conn.IntraHost = true
			continue
		}

		_, ok := lAddrs[keyWithRAddr]
		if ok {
			conn.IntraHost = true
		}
	}
}
