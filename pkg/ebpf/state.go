package ebpf

import (
	"bytes"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var _ NetworkState = &networkState{}

const (
	// DEBUGCLIENT is the ClientID for debugging
	DEBUGCLIENT = "-1"

	// defaultMaxClosedConns & defaultMaxClientStats are the maximum number of objects that can be stored in-memory.
	// With clients checking connection stats roughly every 30s, this gives us roughly ~1.6k + ~2.5k objects a second respectively.
	defaultMaxClosedConns = 50000 // ~100 bytes per conn = 5MB
	defaultMaxClientStats = 75000
	defaultClientExpiry   = 2 * time.Minute
)

// NetworkState takes care of handling the logic for:
// - closed connections
// - sent and received bytes per connection
type NetworkState interface {
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
	GetStats(closedPollLost, closedPollReceived, tracerSkippedCount, expiredTCP uint64) map[string]interface{}

	// DebugNetworkState returns a map with the current network state for a client ID
	DumpState(clientID string) map[string]interface{}
}

type telemetry struct {
	unorderedConns    int
	closedConnDropped int
	connDropped       int
	underflows        int
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

// NewDefaultNetworkState creates a new network state with default settings
func NewDefaultNetworkState() NetworkState {
	return NewNetworkState(defaultClientExpiry, defaultMaxClosedConns, defaultMaxClientStats)
}

// NewNetworkState creates a new network state
func NewNetworkState(clientExpiry time.Duration, maxClosedConns, maxClientStats int) NetworkState {
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
		return latestConns
	}

	// Update all connections with relevant up-to-date stats for client
	conns := ns.mergeConnections(id, connsByKey)

	// XXX: we should change the way we clean this map once
	// https://github.com/golang/go/issues/20135 is solved
	newStats := make(map[string]*stats, len(ns.clients[id].stats))
	for key, st := range ns.clients[id].stats {
		// Don't keep closed connections' stats
		_, isClosed := ns.clients[id].closedConnections[key]
		_, isActive := connsByKey[key]
		if !isClosed || isActive {
			newStats[key] = st
		}
	}
	ns.clients[id].stats = newStats

	// Flush closed connection map and stats
	ns.clients[id].closedConnections = map[string]ConnectionStats{}

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

			conn.MonotonicSentBytes += prev.MonotonicSentBytes
			conn.MonotonicRecvBytes += prev.MonotonicRecvBytes
			conn.MonotonicRetransmits += prev.MonotonicRetransmits
		} else if len(client.closedConnections) >= ns.maxClosedConns {
			ns.telemetry.closedConnDropped++
			continue
		}

		client.closedConnections[string(key)] = conn
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
		if activeConn, ok := active[key]; ok { // This closed connection has become active again
			closedConn.MonotonicSentBytes += activeConn.MonotonicSentBytes
			closedConn.MonotonicRecvBytes += activeConn.MonotonicRecvBytes
			closedConn.MonotonicRetransmits += activeConn.MonotonicRetransmits

			ns.createStatsForKey(client, key)
			ns.updateConnWithStatWithActiveConn(client, key, *activeConn, &closedConn)
		} else {
			ns.updateConnWithStats(client, key, &closedConn)
		}

		conns = append(conns, closedConn)
	}

	// Active connections
	for key, c := range active {
		if closed, ok := client.closedConnections[key]; ok {
			// If this connection was closed while we were collecting active connections it means the active
			// connection is no more up-to date and we already went through the closed connection so let's
			// skip it and not update the stats counters
			if closed.LastUpdateEpoch >= c.LastUpdateEpoch {
				continue
			}

			// If this connection was both closed and reopened, update the counters to reflect only the active connection.
			// The monotonic counters will be the sum of all connections that cross our interval start + finish.
			if stats, ok := client.stats[key]; ok {
				stats.totalRetransmits = c.MonotonicRetransmits
				stats.totalSent = c.MonotonicSentBytes
				stats.totalRecv = c.MonotonicRecvBytes
			}
			continue // We processed this connection during the closed connection pass, so lets not do it again.
		}

		ns.createStatsForKey(client, key)
		ns.updateConnWithStats(client, key, c)

		conns = append(conns, *c)
	}

	return conns
}

// This is used to update the stats when we process a closed connection that became active again
// in this case we want the stats to reflect the new active connections in order to avoid underflows
func (ns *networkState) updateConnWithStatWithActiveConn(client *client, key string, active ConnectionStats, closed *ConnectionStats) {
	if st, ok := client.stats[key]; ok {
		// Check for underflow
		if closed.MonotonicSentBytes < st.totalSent || closed.MonotonicRecvBytes < st.totalRecv || closed.MonotonicRetransmits < st.totalRetransmits {
			ns.telemetry.underflows++
		} else {
			closed.LastSentBytes = closed.MonotonicSentBytes - st.totalSent
			closed.LastRecvBytes = closed.MonotonicRecvBytes - st.totalRecv
			closed.LastRetransmits = closed.MonotonicRetransmits - st.totalRetransmits
		}

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
		// Check for underflow
		if c.MonotonicSentBytes < st.totalSent || c.MonotonicRecvBytes < st.totalRecv || c.MonotonicRetransmits < st.totalRetransmits {
			ns.telemetry.underflows++
		} else {
			c.LastSentBytes = c.MonotonicSentBytes - st.totalSent
			c.LastRecvBytes = c.MonotonicRecvBytes - st.totalRecv
			c.LastRetransmits = c.MonotonicRetransmits - st.totalRetransmits
		}

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
	if ns.telemetry.unorderedConns > 0 || ns.telemetry.underflows > 0 || ns.telemetry.closedConnDropped > 0 || ns.telemetry.connDropped > 0 {
		log.Debugf("state telemetry: [%d unordered conns] [%d stats underflows] [%d connections dropped due to stats] [%d closed connections dropped]",
			ns.telemetry.unorderedConns,
			ns.telemetry.underflows,
			ns.telemetry.closedConnDropped,
			ns.telemetry.connDropped)
	}
	ns.telemetry = telemetry{}
}

// GetStats returns a map of statistics about the current network state
func (ns *networkState) GetStats(closedPollLost, closedPollReceived, tracerSkipped, expiredTCP uint64) map[string]interface{} {
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
		"telemetry": map[string]int{
			"underflows":                   ns.telemetry.underflows,
			"unordered_conns":              ns.telemetry.unorderedConns,
			"closed_conn_dropped":          ns.telemetry.closedConnDropped,
			"conn_dropped":                 ns.telemetry.connDropped,
			"closed_conn_polling_lost":     int(closedPollLost),
			"closed_conn_polling_received": int(closedPollReceived),
			"ok_conns_skipped":             int(tracerSkipped), // Skipped connections (e.g. Local DNS requests)
			"expired_tcp_conns":            int(expiredTCP),
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
