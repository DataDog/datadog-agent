// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func BenchmarkConnectionsGet(b *testing.B) {
	conns := generateRandConnections(30000)
	closed := generateRandConnections(30000)

	for _, bench := range []struct {
		connCount   int
		closedCount int
	}{
		{
			connCount:   100,
			closedCount: 0,
		},
		{
			connCount:   100,
			closedCount: 50,
		},
		{
			connCount:   100,
			closedCount: 100,
		},
		{
			connCount:   1000,
			closedCount: 0,
		},
		{
			connCount:   1000,
			closedCount: 500,
		},
		{
			connCount:   1000,
			closedCount: 1000,
		},
		{
			connCount:   10000,
			closedCount: 0,
		},
		{
			connCount:   10000,
			closedCount: 5000,
		},
		{
			connCount:   10000,
			closedCount: 10000,
		},
		{
			connCount:   30000,
			closedCount: 0,
		},
		{
			connCount:   30000,
			closedCount: 15000,
		},
		{
			connCount:   30000,
			closedCount: 30000,
		},
	} {
		b.Run(fmt.Sprintf("ConnectionsGet-%d-%d", bench.connCount, bench.closedCount), func(b *testing.B) {
			ns := newDefaultState()

			// Initial fetch to set up client
			ns.GetDelta(DEBUGCLIENT, latestTime.Load(), nil, nil, nil)

			for _, c := range closed[:bench.closedCount] {
				ns.StoreClosedConnections([]ConnectionStats{c})
			}

			b.ResetTimer()
			b.ReportAllocs()

			for n := 0; n < b.N; n++ {
				ns.GetDelta(DEBUGCLIENT, latestTime.Load(), conns[:bench.connCount], nil, nil)
			}
		})
	}
}

func TestRemoveConnections(t *testing.T) {
	conn := ConnectionStats{
		Pid:    123,
		Type:   UDP,
		Family: AFINET,
		Source: util.AddressFromString("127.0.0.1"),
		Dest:   util.AddressFromString("127.0.0.1"),
		SPort:  31890,
		DPort:  80,
		Monotonic: StatCounters{
			SentBytes:   12345,
			RecvBytes:   6789,
			Retransmits: 2,
		},
		Last: StatCounters{
			SentBytes:   12345,
			RecvBytes:   6789,
			Retransmits: 2,
		},
		IntraHost: true,
		Cookie:    0,
	}

	clientID := "1"
	state := newDefaultState()
	conns := state.GetDelta(clientID, latestEpochTime(), nil, nil, nil).Conns
	assert.Equal(t, 0, len(conns))

	conns = state.GetDelta(clientID, latestEpochTime(), []ConnectionStats{conn}, nil, nil).Conns
	assert.Equal(t, 1, len(conns))
	assert.Equal(t, conn, conns[0])

	client := state.clients[clientID]
	assert.Equal(t, 1, len(client.stats))

	state.RemoveConnections([]*ConnectionStats{&conn})
	assert.Equal(t, 0, len(client.stats))
}

func TestRetrieveClosedConnection(t *testing.T) {
	conn := ConnectionStats{
		Pid:    123,
		Type:   TCP,
		Family: AFINET,
		Source: util.AddressFromString("127.0.0.1"),
		Dest:   util.AddressFromString("127.0.0.1"),
		SPort:  31890,
		DPort:  80,
		Monotonic: StatCounters{
			SentBytes:   12345,
			RecvBytes:   6789,
			Retransmits: 2,
		},
		Last: StatCounters{
			SentBytes:   12345,
			RecvBytes:   6789,
			Retransmits: 2,
		},
		IntraHost: true,
		Cookie:    0,
	}

	clientID := "1"

	t.Run("without prior registration", func(t *testing.T) {
		state := newDefaultState()
		state.StoreClosedConnections([]ConnectionStats{conn})
		conns := state.GetDelta(clientID, latestEpochTime(), nil, nil, nil).Conns

		assert.Equal(t, 0, len(conns))
	})

	t.Run("with registration", func(t *testing.T) {
		state := newDefaultState()

		state.RegisterClient(clientID)

		state.StoreClosedConnections([]ConnectionStats{conn})

		conns := state.GetDelta(clientID, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 1, len(conns))
		assert.Equal(t, conn, conns[0])

		// An other client that is not registered should not have the closed connection
		conns = state.GetDelta("2", latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 0, len(conns))

		// It should no more have connections stored
		conns = state.GetDelta(clientID, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 0, len(conns))
	})
}

func buildBasicTelemetry() map[ConnTelemetryType]int64 {
	var res = make(map[ConnTelemetryType]int64)
	for i, telType := range MonotonicConnTelemetryTypes {
		res[telType] = int64(i)
	}
	for i, telType := range ConnTelemetryTypes {
		res[telType] = int64(i)
	}

	return res
}

func TestFirstTelemetryRegistering(t *testing.T) {
	clientID := "1"
	state := newDefaultState()
	state.RegisterClient(clientID)
	telem := buildBasicTelemetry()
	delta := state.GetTelemetryDelta(clientID, telem)

	// On first call, delta and telemetry should be the same
	require.Equal(t, telem, delta)
}

func TestTelemetryDiffing(t *testing.T) {
	clientID := "1"
	t.Run("unique client", func(t *testing.T) {
		state := newDefaultState()
		state.RegisterClient(clientID)
		telem := buildBasicTelemetry()
		_ = state.GetTelemetryDelta(clientID, telem)
		delta := state.GetTelemetryDelta(clientID, telem)

		// As we're passing in the same telemetry for the second call,
		// monotonic values should be 0. The other ones should remain.
		for _, telType := range MonotonicConnTelemetryTypes {
			require.Equal(t, delta[telType], int64(0))
		}
		for _, telType := range ConnTelemetryTypes {
			require.Equal(t, delta[telType], telem[telType])
		}
	})
	t.Run("two clients", func(t *testing.T) {
		state := newDefaultState()
		client2 := "2"

		state.RegisterClient(clientID)
		state.RegisterClient(client2)

		telem := buildBasicTelemetry()

		_ = state.GetTelemetryDelta(clientID, telem)

		// when client2 calls this method, it should see previous telemetry data
		// for ones that aren't monotonic.
		delta := state.GetTelemetryDelta(client2, telem)
		for _, telType := range MonotonicConnTelemetryTypes {
			require.Equal(t, delta[telType], telem[telType])
		}
		for _, telType := range ConnTelemetryTypes {
			// As we've passed the same telemetry data for the two calls, we should
			// accumulate the data for the non monotonic part.
			require.Equal(t, delta[telType], telem[telType]*2)
		}
	})
}

func TestNoPriorRegistrationActiveConnections(t *testing.T) {
	clientID := "1"
	state := newDefaultState()
	conn := ConnectionStats{
		Pid:    123,
		Type:   TCP,
		Family: AFINET,
		Source: util.AddressFromString("127.0.0.1"),
		Dest:   util.AddressFromString("127.0.0.1"),
		SPort:  9000,
		DPort:  1234,
		Monotonic: StatCounters{
			SentBytes: 1,
		},
		Cookie: 0,
	}

	delta := state.GetDelta(clientID, latestEpochTime(), []ConnectionStats{conn}, nil, nil)
	require.NotEmpty(t, delta.Conns)
	require.Equal(t, 1, len(delta.Conns))
}

func TestCleanupClient(t *testing.T) {
	clientID := "1"

	state := NewState(100*time.Millisecond, 50000, 75000, 75000, 75000)
	clients := state.(*networkState).getClients()
	assert.Equal(t, 0, len(clients))

	state.RegisterClient(clientID)

	// Should be a no op
	state.(*networkState).RemoveExpiredClients(time.Now())

	clients = state.(*networkState).getClients()
	assert.Equal(t, 1, len(clients))
	assert.Equal(t, "1", clients[0])

	// Should delete the client 1
	state.(*networkState).RemoveExpiredClients(time.Now().Add(150 * time.Millisecond))

	clients = state.(*networkState).getClients()
	assert.Equal(t, 0, len(clients))
}

func TestLastStats(t *testing.T) {
	client1 := "1"
	client2 := "2"
	state := newDefaultState()

	dSent := uint64(42)
	dRecv := uint64(133)
	dRetransmits := uint32(7)

	m := StatCounters{
		SentBytes:   36,
		RecvBytes:   24,
		Retransmits: 2,
	}

	conn := ConnectionStats{
		Pid:       123,
		Type:      TCP,
		Family:    AFINET,
		Source:    util.AddressFromString("127.0.0.1"),
		Dest:      util.AddressFromString("127.0.0.1"),
		SPort:     31890,
		DPort:     80,
		Monotonic: m,
	}

	conn2 := conn
	conn2.Monotonic.SentBytes += dSent
	conn2.Monotonic.RecvBytes += dRecv
	conn2.Monotonic.Retransmits += dRetransmits

	conn3 := conn2
	conn3.Monotonic.SentBytes += dSent
	conn3.Monotonic.RecvBytes += dRecv
	conn3.Monotonic.Retransmits += dRetransmits

	// Start by registering the two clients
	state.RegisterClient(client1)
	state.RegisterClient(client2)

	// First get, we should not have any connections stored
	conns := state.GetDelta(client1, latestEpochTime(), nil, nil, nil).Conns
	assert.Equal(t, 0, len(conns))

	// Same for an other client
	conns = state.GetDelta(client2, latestEpochTime(), nil, nil, nil).Conns
	assert.Equal(t, 0, len(conns))

	// We should have only one connection but with last stats equal to monotonic
	conns = state.GetDelta(client1, latestEpochTime(), []ConnectionStats{conn}, nil, nil).Conns
	assert.Equal(t, 1, len(conns))
	assert.Equal(t, conn.Monotonic.SentBytes, conns[0].Last.SentBytes)
	assert.Equal(t, conn.Monotonic.RecvBytes, conns[0].Last.RecvBytes)
	assert.Equal(t, conn.Monotonic.Retransmits, conns[0].Last.Retransmits)
	assert.Equal(t, conn.Monotonic.SentBytes, conns[0].Monotonic.SentBytes)
	assert.Equal(t, conn.Monotonic.RecvBytes, conns[0].Monotonic.RecvBytes)
	assert.Equal(t, conn.Monotonic.Retransmits, conns[0].Monotonic.Retransmits)

	// This client didn't collect the first connection so last stats = monotonic
	conns = state.GetDelta(client2, latestEpochTime(), []ConnectionStats{conn2}, nil, nil).Conns
	assert.Equal(t, 1, len(conns))
	assert.Equal(t, conn2.Monotonic.SentBytes, conns[0].Last.SentBytes)
	assert.Equal(t, conn2.Monotonic.RecvBytes, conns[0].Last.RecvBytes)
	assert.Equal(t, conn2.Monotonic.Retransmits, conns[0].Last.Retransmits)
	assert.Equal(t, conn2.Monotonic.SentBytes, conns[0].Monotonic.SentBytes)
	assert.Equal(t, conn2.Monotonic.RecvBytes, conns[0].Monotonic.RecvBytes)
	assert.Equal(t, conn2.Monotonic.Retransmits, conns[0].Monotonic.Retransmits)

	// client 1 should have conn3 - conn1 since it did not collected conn2
	conns = state.GetDelta(client1, latestEpochTime(), []ConnectionStats{conn3}, nil, nil).Conns
	assert.Equal(t, 1, len(conns))
	assert.Equal(t, 2*dSent, conns[0].Last.SentBytes)
	assert.Equal(t, 2*dRecv, conns[0].Last.RecvBytes)
	assert.Equal(t, 2*dRetransmits, conns[0].Last.Retransmits)
	assert.Equal(t, conn3.Monotonic.SentBytes, conns[0].Monotonic.SentBytes)
	assert.Equal(t, conn3.Monotonic.RecvBytes, conns[0].Monotonic.RecvBytes)
	assert.Equal(t, conn3.Monotonic.Retransmits, conns[0].Monotonic.Retransmits)

	// client 2 should have conn3 - conn2
	conns = state.GetDelta(client2, latestEpochTime(), []ConnectionStats{conn3}, nil, nil).Conns
	assert.Equal(t, 1, len(conns))
	assert.Equal(t, dSent, conns[0].Last.SentBytes)
	assert.Equal(t, dRecv, conns[0].Last.RecvBytes)
	assert.Equal(t, dRetransmits, conns[0].Last.Retransmits)
	assert.Equal(t, conn3.Monotonic.SentBytes, conns[0].Monotonic.SentBytes)
	assert.Equal(t, conn3.Monotonic.RecvBytes, conns[0].Monotonic.RecvBytes)
	assert.Equal(t, conn3.Monotonic.Retransmits, conns[0].Monotonic.Retransmits)
}

func TestLastStatsForClosedConnection(t *testing.T) {
	clientID := "1"
	state := newDefaultState()

	dSent := uint64(42)
	dRecv := uint64(133)
	dRetransmits := uint32(0)

	m := StatCounters{
		SentBytes:   36,
		RecvBytes:   24,
		Retransmits: 1,
	}

	conn := ConnectionStats{
		Pid:       123,
		Type:      TCP,
		Family:    AFINET,
		Source:    util.AddressFromString("127.0.0.1"),
		Dest:      util.AddressFromString("127.0.0.1"),
		SPort:     31890,
		DPort:     80,
		Monotonic: m,
	}

	conn2 := conn
	conn2.Monotonic.SentBytes += dSent
	conn2.Monotonic.RecvBytes += dRecv
	conn2.Monotonic.Retransmits += dRetransmits

	state.RegisterClient(clientID)

	// First get, we should not have any connections stored
	conns := state.GetDelta(clientID, latestEpochTime(), nil, nil, nil).Conns
	assert.Equal(t, 0, len(conns))

	// We should have one connection with last stats equal to monotonic stats
	conns = state.GetDelta(clientID, latestEpochTime(), []ConnectionStats{conn}, nil, nil).Conns
	assert.Equal(t, 1, len(conns))
	assert.Equal(t, conn.Monotonic.SentBytes, conns[0].Last.SentBytes)
	assert.Equal(t, conn.Monotonic.RecvBytes, conns[0].Last.RecvBytes)
	assert.Equal(t, conn.Monotonic.Retransmits, conns[0].Last.Retransmits)
	assert.Equal(t, conn.Monotonic.SentBytes, conns[0].Monotonic.SentBytes)
	assert.Equal(t, conn.Monotonic.RecvBytes, conns[0].Monotonic.RecvBytes)
	assert.Equal(t, conn.Monotonic.Retransmits, conns[0].Monotonic.Retransmits)

	state.StoreClosedConnections([]ConnectionStats{conn2})

	// We should have one connection with last stats
	conns = state.GetDelta(clientID, latestEpochTime(), nil, nil, nil).Conns

	assert.Equal(t, 1, len(conns))
	assert.Equal(t, dSent, conns[0].Last.SentBytes)
	assert.Equal(t, dRecv, conns[0].Last.RecvBytes)
	assert.Equal(t, dRetransmits, conns[0].Last.Retransmits)
	assert.Equal(t, conn2.Monotonic.SentBytes, conns[0].Monotonic.SentBytes)
	assert.Equal(t, conn2.Monotonic.RecvBytes, conns[0].Monotonic.RecvBytes)
	assert.Equal(t, conn2.Monotonic.Retransmits, conns[0].Monotonic.Retransmits)
}

func TestRaceConditions(t *testing.T) {
	nClients := 10

	// Generate random conns
	genConns := func(n uint32) []ConnectionStats {
		conns := make([]ConnectionStats, 0, n)
		for i := uint32(0); i < n; i++ {
			conns = append(conns, ConnectionStats{
				Pid:    1 + i,
				Type:   TCP,
				Family: AFINET,
				Source: util.AddressFromString("127.0.0.1"),
				Dest:   util.AddressFromString("127.0.0.1"),
				SPort:  uint16(rand.Int()),
				DPort:  uint16(rand.Int()),
				Monotonic: StatCounters{
					SentBytes:   uint64(rand.Int()),
					RecvBytes:   uint64(rand.Int()),
					Retransmits: uint32(rand.Int()),
				},
			})
		}
		return conns
	}

	state := newDefaultState()
	nConns := uint32(100)

	var wg sync.WaitGroup
	wg.Add(nClients)

	// Spawn multiple clients to get multiple times
	for i := 1; i <= nClients; i++ {
		go func(c string) {
			defer wg.Done()
			defer state.RemoveClient(c)
			timer := time.NewTimer(1 * time.Second)
			for {
				select {
				case <-timer.C:
					return
				default:
					state.GetDelta(c, latestEpochTime(), genConns(nConns), nil, nil)
				}
			}
		}(fmt.Sprintf("%d", i))
	}

	wg.Wait()
}

func TestSameKeyEdgeCases(t *testing.T) {
	// For this test all the connections have the same key
	// Each vertical bar represents a collection for a given client
	// Each horizontal bar represents a connection lifespan (from start to end with the number of sent bytes written on top of the line)

	client := "c"
	conn := ConnectionStats{
		Pid:       123,
		Type:      TCP,
		Family:    AFINET,
		Source:    util.AddressFromString("127.0.0.1"),
		Dest:      util.AddressFromString("127.0.0.1"),
		Monotonic: StatCounters{SentBytes: 3},
		Cookie:    1,
	}

	t.Run("ShortlivedConnection", func(t *testing.T) {
		// +     3 bytes      +
		// |                  |
		// |   +---------+    |
		// |                  |
		// +                  +

		// c0                 c1

		// We expect:
		// c0: Nothing
		// c1: Monotonic: 3 bytes, Last seen: 3 bytes
		state := newDefaultState()

		// Let's register our client
		state.RegisterClient(client)

		// First get, we should have nothing
		conns := state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 0, len(conns))

		// Store the connection as closed
		state.StoreClosedConnections([]ConnectionStats{conn})

		// Second get, we should have monotonic and last stats = 3
		conns = state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 1, len(conns))
		assert.Equal(t, 3, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 3, int(conns[0].Last.SentBytes))

		// should not hold on to closed connection stats
		assert.Empty(t, state.clients["c"].stats)
	})

	t.Run("TwoShortlivedConnections", func(t *testing.T) {
		//  +    3 bytes       5 bytes    +
		//  |                             |
		//  |    +-----+       +-----+    |
		//  |                             |
		//  +                             +

		//  c0                            c1

		// We expect:
		// c0: Nothing
		// c1: Monotonic: 8 bytes, Last seen 8 bytes

		state := newDefaultState()

		// Let's register our client
		state.RegisterClient(client)

		// First get, we should have nothing
		conns := state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 0, len(conns))

		// Store the connection as closed
		state.StoreClosedConnections([]ConnectionStats{conn})

		conn2 := conn
		conn2.Cookie = 2
		conn2.Monotonic = StatCounters{SentBytes: 5}
		conn2.LastUpdateEpoch++
		// Store the connection another time
		state.StoreClosedConnections([]ConnectionStats{conn2})

		// Second get, we should have monotonic and last stats = 8
		conns = state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 1, len(conns))
		assert.Equal(t, 8, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 8, int(conns[0].Last.SentBytes))

		// should not hold on to closed connection stats
		assert.Empty(t, state.clients["c"].stats)
	})

	t.Run("TwoShortlivedConnectionsCrossing-1", func(t *testing.T) {
		// +    1 b  +  1 bytes    1 b +   1 b        +
		// |         |                 |              |
		// |    +-----------+      +------------+     |
		// |         |                 |              |
		// +         +                 +              +

		// c0        c1                c2             c3
		// We expect:

		// c0: Nothing
		// c1: Monotonic: 1 bytes, Last seen: 1 bytes
		// c2: Monotonic: 3 bytes, Last seen: 2 bytes
		// c3: Monotonic: 2 bytes, Last seen: 1 bytes

		state := newDefaultState()

		// Let's register our client
		state.RegisterClient(client)

		// First get for client c, we should have nothing
		conns := state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns
		assert.Len(t, conns, 0)

		conn := ConnectionStats{
			Pid:       123,
			Type:      TCP,
			Family:    AFINET,
			Source:    util.AddressFromString("127.0.0.1"),
			Dest:      util.AddressFromString("127.0.0.1"),
			SPort:     9000,
			DPort:     1234,
			Monotonic: StatCounters{SentBytes: 1},
			Cookie:    1,
		}

		// Simulate this connection starting
		conns = state.GetDelta(client, latestEpochTime(), []ConnectionStats{conn}, nil, nil).Conns
		require.Len(t, conns, 1)
		assert.EqualValues(t, 1, conns[0].Last.SentBytes)
		assert.EqualValues(t, 1, conns[0].Monotonic.SentBytes)

		// should not hold on to closed connection stats
		assert.Len(t, state.clients["c"].stats, 1)

		// Store the connection as closed
		conn.Monotonic.SentBytes++
		conn.LastUpdateEpoch = latestEpochTime()
		state.StoreClosedConnections([]ConnectionStats{conn})

		conn2 := conn
		conn2.Monotonic.SentBytes = 1
		conn2.Cookie = 2
		conn2.LastUpdateEpoch = latestEpochTime()
		// Retrieve the connections
		conns = state.GetDelta(client, latestEpochTime(), []ConnectionStats{conn2}, nil, nil).Conns
		require.Len(t, conns, 2)
		assert.EqualValues(t, uint64(1), conns[0].Last.SentBytes)
		assert.EqualValues(t, uint64(2), conns[0].Monotonic.SentBytes)
		// should not hold on to active connection stats
		assert.Len(t, state.clients["c"].stats, 1)
		assert.Contains(t, state.clients["c"].stats, conn2.Cookie)

		conn2.Monotonic.SentBytes++
		conn.LastUpdateEpoch = latestEpochTime()
		// Store the connection as closed
		state.StoreClosedConnections([]ConnectionStats{conn2})

		conns = state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns
		require.Len(t, conns, 1)
		assert.EqualValues(t, 1, conns[0].Last.SentBytes)
		assert.EqualValues(t, 2, conns[0].Monotonic.SentBytes)
		// should not hold on to closed connection stats
		assert.Len(t, state.clients["c"].stats, 0)
	})

	t.Run("TwoShortlivedConnectionsCrossing-2", func(t *testing.T) {
		// +    3 bytes    2 b  +  3 bytes    1 b +   2 b        +
		// |                    |                 |              |
		// |    +-----+    +-----------+      +------------+     |
		// |                    |                 |              |
		// +                    +                 +              +

		// c0                   c1                c2             c3
		// We expect:

		// c0: Nothing
		// c1: Monotonic: 5 bytes, Last seen: 5 bytes
		// c2: Monotonic: 6 bytes, Last seen: 4 bytes
		// c3: Monotonic: 3 bytes, Last seen: 2 bytes

		state := newDefaultState()

		// Let's register our client
		state.RegisterClient(client)

		// First get, we should have nothing
		conns := state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 0, len(conns))

		// Store the connection as closed
		state.StoreClosedConnections([]ConnectionStats{conn})

		conn2 := conn
		conn2.Cookie = 2
		conn2.Monotonic = StatCounters{SentBytes: 2}
		conn2.LastUpdateEpoch++
		// Store the connection as an opened connection
		cs := []ConnectionStats{conn2}

		// Second get, we should have monotonic and last stats = 5
		conns = state.GetDelta(client, latestEpochTime(), cs, nil, nil).Conns
		require.Equal(t, 2, len(conns))
		assert.Equal(t, 3, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 3, int(conns[0].Last.SentBytes))
		assert.Equal(t, 2, int(conns[1].Monotonic.SentBytes))
		assert.Equal(t, 2, int(conns[1].Last.SentBytes))
		// should not hold on to closed connection stats
		assert.Len(t, state.clients["c"].stats, 1)
		assert.Contains(t, state.clients["c"].stats, conn2.Cookie)

		// Store the connection as closed
		conn2.Monotonic.SentBytes += 3
		conn2.LastUpdateEpoch++
		state.StoreClosedConnections([]ConnectionStats{conn2})

		// Store the connection again
		conn3 := conn2
		conn3.Monotonic.SentBytes = 1
		conn3.Cookie = 3
		conn3.LastUpdateEpoch++
		cs = []ConnectionStats{conn3}

		// Third get, we should have monotonic = 6 and last stats = 4
		conns = state.GetDelta(client, latestEpochTime(), cs, nil, nil).Conns
		assert.Equal(t, 2, len(conns))
		assert.Equal(t, 5, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 3, int(conns[0].Last.SentBytes))
		assert.Equal(t, 1, int(conns[1].Monotonic.SentBytes))
		assert.Equal(t, 1, int(conns[1].Last.SentBytes))
		// should not hold on to closed connection stats
		assert.Len(t, state.clients["c"].stats, 1)
		assert.Contains(t, state.clients["c"].stats, conn3.Cookie)

		// Store the connection as closed
		conn3.Monotonic.SentBytes += 2
		state.StoreClosedConnections([]ConnectionStats{conn3})

		// 4th get, we should have monotonic = 3 and last stats = 2
		conns = state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 1, len(conns))
		assert.Equal(t, 3, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 2, int(conns[0].Last.SentBytes))
		// should not hold on to closed connection stats
		assert.Empty(t, state.clients["c"].stats)
	})

	t.Run("ConnectionCrossing", func(t *testing.T) {
		// 3 b  +  5 bytes        +
		//      |                 |
		// +-----------+          |
		//      |                 |
		//      +                 +

		//     c0                c1
		// We expect:

		// c0: Monotonic: 3 bytes, Last seen: 3 bytes
		// c1: Monotonic: 8 bytes, Last seen: 5 bytes

		state := newDefaultState()

		// Let's register our client
		state.RegisterClient(client)

		// First get we should have nothing
		conns := state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 0, len(conns))

		// Store the connection as opened
		cs := []ConnectionStats{conn}

		// First get, we should have monotonic = 3 and last seen = 3
		conns = state.GetDelta(client, latestEpochTime(), cs, nil, nil).Conns
		assert.Equal(t, 1, len(conns))
		assert.Equal(t, 3, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 3, int(conns[0].Last.SentBytes))
		assert.Len(t, state.clients["c"].stats, 1)
		assert.Contains(t, state.clients["c"].stats, conn.Cookie)

		// Store the connection as closed
		conn2 := conn
		conn2.Monotonic = StatCounters{SentBytes: 8}
		state.StoreClosedConnections([]ConnectionStats{conn2})

		// Second get, we should have monotonic = 8 and last stats = 5
		conns = state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 1, len(conns))
		assert.Equal(t, 8, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 5, int(conns[0].Last.SentBytes))
		assert.Empty(t, state.clients["c"].stats)
	})

	t.Run("TwoShortlivedConnectionsCrossingWithTwoClients", func(t *testing.T) {
		//              +    3 bytes    2 b  +  3 bytes    1 b +   2 b        +
		//              |                    |                 |              |
		// client c     |    +-----+    +-----------+      +------------+     |
		//              |                    |                 |              |
		//              +                    +                 +              +
		//
		//              c0                   c1                c2             c3
		//
		//
		//              +    3 bytes  +  3 b    +  2 b      2 b     +  1 b         +
		//              |             |         |                   |              |
		// client d     |    +-----+  |  +----------+      +------------+          |
		//              |             |         |                   |              |
		//              +             +         +                   +              +
		//
		//              d0            d1        d2                  d3             d4

		// We expect:
		// c0: Nothing
		// d0: Nothing
		// d1: Monotonic: 3 bytes, Last seen: 3 bytes (this connection started after closed + collect, so we reset monotonic)
		// c1: Monotonic: 5 bytes, Last seen: 5 bytes
		// d2: Monotonic: 3 bytes, Last seen 3 bytes
		// c2: Monotonic: 6 bytes, Last seen: 4 bytes
		// d3: Monotonic: 7 bytes, Last seen 4 bytes
		// c3: Monotonic: 3 bytes, Last seen: 2 bytes
		// d4: Monotonic: 3 bytes, Last seen: 1 bytes

		clientD := "d"

		state := newDefaultState()

		// Let's register our client
		state.RegisterClient(client)

		// First get for client c, we should have nothing
		conns := state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 0, len(conns))

		// First get for client d, we should have nothing
		conns = state.GetDelta(clientD, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 0, len(conns))

		// Store the connection as closed
		state.StoreClosedConnections([]ConnectionStats{conn})

		// Second get for client d we should have monotonic and last stats = 3
		conns = state.GetDelta(clientD, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 1, len(conns))
		assert.Equal(t, 3, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 3, int(conns[0].Last.SentBytes))
		assert.Empty(t, state.clients["d"].stats)

		// Store the connection as an opened connection
		conn2 := conn
		conn2.Monotonic.SentBytes = 2
		conn2.Cookie = 2
		conn2.LastUpdateEpoch++
		cs := []ConnectionStats{conn2}

		// Second get, for client c we should have monotonic and last stats = 5
		conns = state.GetDelta(client, latestEpochTime(), cs, nil, nil).Conns
		assert.Equal(t, 2, len(conns))
		assert.Equal(t, 3, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 3, int(conns[0].Last.SentBytes))
		assert.Equal(t, 2, int(conns[1].Monotonic.SentBytes))
		assert.Equal(t, 2, int(conns[1].Last.SentBytes))
		assert.Len(t, state.clients["c"].stats, 1)
		assert.Contains(t, state.clients["c"].stats, conn2.Cookie)

		// Store the connection as an opened connection
		conn2.Monotonic.SentBytes++
		conn2.LastUpdateEpoch++
		cs = []ConnectionStats{conn2}

		// Third get, for client d we should have monotonic = 3 and last stats = 3
		conns = state.GetDelta(clientD, latestEpochTime(), cs, nil, nil).Conns
		assert.Equal(t, 1, len(conns))
		assert.Equal(t, 3, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 3, int(conns[0].Last.SentBytes))
		assert.Len(t, state.clients["d"].stats, 1)
		assert.Contains(t, state.clients["d"].stats, conn2.Cookie)

		// Store the connection as closed
		conn2.Monotonic.SentBytes += 2
		conn2.LastUpdateEpoch++
		state.StoreClosedConnections([]ConnectionStats{conn2})

		// Store the connection again
		conn3 := conn2
		conn3.Monotonic.SentBytes = 1
		conn3.Cookie = 3
		conn3.LastUpdateEpoch++
		cs = []ConnectionStats{conn3}

		// Third get, for client c, we should have monotonic = 6 and last stats = 4
		conns = state.GetDelta(client, latestEpochTime(), cs, nil, nil).Conns
		assert.Equal(t, 2, len(conns))
		assert.Equal(t, 5, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 3, int(conns[0].Last.SentBytes))
		assert.Equal(t, 1, int(conns[1].Monotonic.SentBytes))
		assert.Equal(t, 1, int(conns[1].Last.SentBytes))
		assert.Len(t, state.clients["c"].stats, 1)
		assert.Contains(t, state.clients["c"].stats, conn3.Cookie)

		// Store the connection again
		conn3.Monotonic.SentBytes++
		conn3.LastUpdateEpoch++
		cs = []ConnectionStats{conn3}

		// 4th get, for client d, we should have monotonic = 7 and last stats = 4
		conns = state.GetDelta(clientD, latestEpochTime(), cs, nil, nil).Conns
		assert.Equal(t, 2, len(conns))
		assert.Equal(t, 5, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 2, int(conns[0].Last.SentBytes))
		assert.Equal(t, 2, int(conns[1].Monotonic.SentBytes))
		assert.Equal(t, 2, int(conns[1].Last.SentBytes))
		assert.Len(t, state.clients["d"].stats, 1)
		assert.Contains(t, state.clients["d"].stats, conn3.Cookie)

		// Store the connection as closed
		conn3.Monotonic.SentBytes++
		conn3.LastUpdateEpoch++
		state.StoreClosedConnections([]ConnectionStats{conn3})

		// 4th get, for client c we should have monotonic = 3 and last stats = 2
		conns = state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 1, len(conns))
		assert.Equal(t, 3, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 2, int(conns[0].Last.SentBytes))
		assert.Empty(t, state.clients["c"].stats)

		// 5th get, for client d we should have monotonic = 3 and last stats = 1
		conns = state.GetDelta(clientD, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 1, len(conns))
		assert.Equal(t, 3, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 1, int(conns[0].Last.SentBytes))
		assert.Empty(t, state.clients["d"].stats)
	})

	t.Run("ShortlivedConnectionCrossingWithThreeClients", func(t *testing.T) {
		//              +    3 bytes    2 b  +  3 bytes
		//              |                    |
		// client c     |    +-----+    +-----------+
		//              |                    |
		//              +                    +
		//
		//              c0                   c1
		//
		//
		//              +    3 bytes  +  3 b    +  2 b
		//              |             |         |
		// client d     |    +-----+  |  +----------+
		//              |             |         |
		//              +             +         +
		//
		//              d0            d1        d2
		//
		//
		//              +    2 b + 1b  +    5 bytes   +
		//              |        |     |              |
		// client e     |    +-----+   | +---------+  |
		//              |        |     |              |
		//              +        +     +              +
		//
		//              e0       e1    e2             e3

		// We expect:
		// c0, d0, e0: Nothing
		// e1: Monotonic: 2 bytes, Last seen 2 bytes
		// d1: Monotonic 3 bytes, Last seen: 3 bytes
		// e2: Monotonic: 3 bytes, Last seen: 1 bytes
		// c1: Monotonic: 5 bytes, Last seen: 5 bytes
		// d2: Monotonic: 3 bytes, Last seen 3 bytes
		// e3: Monotonic: 5 bytes, Last seen: 5 bytes

		clientD := "d"
		clientE := "e"

		state := newDefaultState()

		// Let's register our clients
		state.RegisterClient(client)
		state.RegisterClient(clientD)
		state.RegisterClient(clientE)

		// First get for client c, we should have nothing
		conns := state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 0, len(conns))

		// First get for client d, we should have nothing
		conns = state.GetDelta(clientD, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 0, len(conns))

		// First get for client e, we should have nothing
		conns = state.GetDelta(clientE, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 0, len(conns))

		// Store the connection
		conn.Monotonic.SentBytes = 2
		conn.LastUpdateEpoch++
		cs := []ConnectionStats{conn}

		// Second get for client e we should have monotonic and last stats = 2
		conns = state.GetDelta(clientE, latestEpochTime(), cs, nil, nil).Conns
		assert.Equal(t, 1, len(conns))
		assert.Equal(t, 2, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 2, int(conns[0].Last.SentBytes))
		assert.Len(t, state.clients["e"].stats, 1)
		assert.Contains(t, state.clients["e"].stats, conn.Cookie)

		// Store the connection as closed
		conn.Monotonic.SentBytes++
		conn.LastUpdateEpoch++
		state.StoreClosedConnections([]ConnectionStats{conn})

		// Second get for client d we should have monotonic and last stats = 3
		conns = state.GetDelta(clientD, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 1, len(conns))
		assert.Equal(t, 3, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 3, int(conns[0].Last.SentBytes))
		assert.Empty(t, state.clients["d"].stats)

		// Third get for client e we should have monotonic = 3and last stats = 1
		conns = state.GetDelta(clientE, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 1, len(conns))
		assert.Equal(t, 3, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 1, int(conns[0].Last.SentBytes))
		assert.Empty(t, state.clients["e"].stats)

		// Store the connection as an opened connection
		conn2 := conn
		conn2.Monotonic.SentBytes = 2
		conn2.Cookie = 2
		conn2.LastUpdateEpoch++
		cs = []ConnectionStats{conn2}

		// Second get, for client c we should have monotonic and last stats = 5
		conns = state.GetDelta(client, latestEpochTime(), cs, nil, nil).Conns
		assert.Equal(t, 2, len(conns))
		assert.Equal(t, 3, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 3, int(conns[0].Last.SentBytes))
		assert.Equal(t, 2, int(conns[1].Monotonic.SentBytes))
		assert.Equal(t, 2, int(conns[1].Last.SentBytes))
		assert.Len(t, state.clients["c"].stats, 1)
		assert.Contains(t, state.clients["c"].stats, conn2.Cookie)

		// Store the connection as an opened connection
		conn2.Monotonic.SentBytes++
		conn2.LastUpdateEpoch++
		cs = []ConnectionStats{conn2}

		// Third get, for client d we should have monotonic = 3 and last stats = 3
		conns = state.GetDelta(clientD, latestEpochTime(), cs, nil, nil).Conns
		assert.Equal(t, 1, len(conns))
		assert.Equal(t, 3, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 3, int(conns[0].Last.SentBytes))
		assert.Len(t, state.clients["d"].stats, 1)
		assert.Contains(t, state.clients["d"].stats, conn2.Cookie)

		// Store the connection as closed
		conn2.Monotonic.SentBytes += 2
		conn2.LastUpdateEpoch++
		state.StoreClosedConnections([]ConnectionStats{conn2})

		// 4th get, for client e we should have monotonic = 5 and last stats = 5
		conns = state.GetDelta(clientE, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 1, len(conns))
		assert.Equal(t, 5, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 5, int(conns[0].Last.SentBytes))
		assert.Empty(t, state.clients["e"].stats)
	})

	t.Run("LonglivedConnectionWithTwoClientsJoiningAtDifferentTimes", func(t *testing.T) {
		//              +      3 bytes       +  1 + 3 b        +   2 b
		//              |                    |                 |
		// client c     |    +------------------------------------------+
		//              |                    |                 |
		//              +                    +                 +
		//
		//              c0                   c1                c2
		//
		//                                                5 bytes
		//                                        +                      +
		//                                        |                      |
		// client d                               |---------------------+|
		//                                        |                      |
		//                                        +                      +
		//
		//                                       d0                      d1

		// We expect:
		// c0: Nothing
		// c1: Monotonic: 3 bytes, Last seen: 3 bytes
		// d0: Monotonic: 4 bytes, Last seen: 4 bytes
		// c2: Monotonic: 7 bytes, Last seen: 4 bytes
		// d1: Monotonic: 9 bytes, Last seen: 5 bytes

		clientD := "d"

		state := newDefaultState()

		// First get for client c, we should have nothing
		conns := state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns
		assert.Equal(t, 0, len(conns))

		// Second get for client c we should have monotonic and last stats = 3
		conns = state.GetDelta(client, latestEpochTime(), []ConnectionStats{conn}, nil, nil).Conns
		assert.Len(t, conns, 1)
		assert.Equal(t, 3, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 3, int(conns[0].Last.SentBytes))
		assert.Len(t, state.clients["c"].stats, 1)
		assert.Contains(t, state.clients["c"].stats, conn.Cookie)

		conn2 := conn
		conn2.Monotonic.SentBytes++
		conn2.LastUpdateEpoch++

		// First get for client d we should have monotonic = 4 and last bytes = 4
		conns = state.GetDelta(clientD, latestEpochTime(), []ConnectionStats{conn2}, nil, nil).Conns
		assert.Len(t, conns, 1)
		assert.Equal(t, 4, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 4, int(conns[0].Last.SentBytes))
		assert.Len(t, state.clients["d"].stats, 1)
		assert.Contains(t, state.clients["d"].stats, conn2.Cookie)

		conn3 := conn2
		conn3.Monotonic.SentBytes += 3
		conn3.LastUpdateEpoch++

		// Third get for client c we should have monotonic = 7 and last bytes = 4
		conns = state.GetDelta(client, latestEpochTime(), []ConnectionStats{conn3}, nil, nil).Conns
		assert.Len(t, conns, 1)
		assert.Equal(t, 7, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 4, int(conns[0].Last.SentBytes))
		assert.Len(t, state.clients["c"].stats, 1)
		assert.Contains(t, state.clients["c"].stats, conn3.Cookie)

		conn4 := conn3
		conn4.Monotonic.SentBytes += 2
		conn4.LastUpdateEpoch++

		// Second get for client d we should have monotonic = 9 and last bytes = 5
		conns = state.GetDelta(clientD, latestEpochTime(), []ConnectionStats{conn4}, nil, nil).Conns
		assert.Len(t, conns, 1)
		assert.Equal(t, 9, int(conns[0].Monotonic.SentBytes))
		assert.Equal(t, 5, int(conns[0].Last.SentBytes))
		assert.Len(t, state.clients["c"].stats, 1)
		assert.Contains(t, state.clients["c"].stats, conn4.Cookie)
	})
}

func TestStatsResetOnUnderflow(t *testing.T) {
	conn := ConnectionStats{
		Pid:       123,
		Type:      TCP,
		Family:    AFINET,
		Source:    util.AddressFromString("127.0.0.1"),
		Dest:      util.AddressFromString("127.0.0.1"),
		Monotonic: StatCounters{SentBytes: 3},
		IntraHost: true,
	}

	client := "client"

	state := newDefaultState()

	// Register the client
	state.RegisterClient(client)

	// Get the connections once to register stats
	conns := state.GetDelta(client, latestEpochTime(), []ConnectionStats{conn}, nil, nil).Conns
	require.Len(t, conns, 1)

	// Expect LastStats to be 3
	conn.Last.SentBytes = 3
	assert.Equal(t, conn, conns[0])

	// Get the connections again but by simulating an underflow
	conn.Monotonic.SentBytes--

	conns = state.GetDelta(client, latestEpochTime(), []ConnectionStats{conn}, nil, nil).Conns
	require.Len(t, conns, 0) // dropped because last stats are zero
}

func TestDoubleCloseOnTwoClients(t *testing.T) {
	conn := ConnectionStats{
		Pid:       123,
		Type:      TCP,
		Family:    AFINET,
		Source:    util.AddressFromString("127.0.0.1"),
		Dest:      util.AddressFromString("127.0.0.1"),
		Monotonic: StatCounters{SentBytes: 3},
		Last:      StatCounters{SentBytes: 3},
		IntraHost: true,
	}

	client1 := "1"
	client2 := "2"

	state := newDefaultState()

	// Register the clients
	state.RegisterClient(client1)
	state.RegisterClient(client2)

	// Store the closed connection twice
	state.StoreClosedConnections([]ConnectionStats{conn})
	conn.LastUpdateEpoch++
	state.StoreClosedConnections([]ConnectionStats{conn})

	// Get the connections for client1 we should have only one with stats counted only once
	conns := state.GetDelta(client1, latestEpochTime(), nil, nil, nil).Conns
	require.Len(t, conns, 1)
	assert.Equal(t, conn, conns[0])

	// Same for client2
	conns = state.GetDelta(client2, latestEpochTime(), nil, nil, nil).Conns
	require.Len(t, conns, 1)
	assert.Equal(t, conn, conns[0])
}

func TestUnorderedCloseEvent(t *testing.T) {
	conn := ConnectionStats{
		Pid:       123,
		Type:      TCP,
		Family:    AFINET,
		Source:    util.AddressFromString("127.0.0.1"),
		Dest:      util.AddressFromString("127.0.0.1"),
		Monotonic: StatCounters{SentBytes: 3},
	}

	client := "client"
	state := newDefaultState()

	// Register the client
	state.RegisterClient(client)

	// Simulate storing a closed connection while we were reading from the eBPF map
	// in this case the closed conn will have an earlier epoch
	conn.LastUpdateEpoch = latestEpochTime() + 1
	conn.Monotonic.SentBytes++
	conn.Monotonic.RecvBytes = 1
	state.StoreClosedConnections([]ConnectionStats{conn})

	conn.LastUpdateEpoch--
	conn.Monotonic.SentBytes--
	conn.Monotonic.RecvBytes = 0
	conns := state.GetDelta(client, latestEpochTime(), []ConnectionStats{conn}, nil, nil).Conns
	require.Len(t, conns, 1)
	assert.EqualValues(t, 4, conns[0].Last.SentBytes)
	assert.EqualValues(t, 1, conns[0].Last.RecvBytes)

	// Simulate some other gets
	assert.Len(t, state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns, 0)
	assert.Len(t, state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns, 0)
	assert.Len(t, state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns, 0)

	// Simulate having the connection getting active again
	conn.LastUpdateEpoch = latestEpochTime()
	conn.Monotonic.SentBytes--
	state.StoreClosedConnections([]ConnectionStats{conn})

	conns = state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns
	require.Len(t, conns, 1)
	assert.EqualValues(t, 2, conns[0].Last.SentBytes)
	assert.EqualValues(t, 0, conns[0].Last.RecvBytes)

	// Ensure we don't have underflows / unordered conns
	assert.Zero(t, state.telemetry.statsUnderflows)

	assert.Len(t, state.GetDelta(client, latestEpochTime(), nil, nil, nil).Conns, 0)
}

func TestAggregateClosedConnectionsTimestamp(t *testing.T) {
	conn := ConnectionStats{
		Pid:       123,
		Type:      TCP,
		Family:    AFINET,
		Source:    util.AddressFromString("127.0.0.1"),
		Dest:      util.AddressFromString("127.0.0.1"),
		Monotonic: StatCounters{SentBytes: 3},
	}

	client := "client"
	state := newDefaultState()

	// Let's register our client
	state.RegisterClient(client)

	conn.LastUpdateEpoch = latestEpochTime()
	state.StoreClosedConnections([]ConnectionStats{conn})

	conn.LastUpdateEpoch = latestEpochTime()
	state.StoreClosedConnections([]ConnectionStats{conn})

	conn.LastUpdateEpoch = latestEpochTime()
	state.StoreClosedConnections([]ConnectionStats{conn})

	// Make sure the connections we get has the latest timestamp
	delta := state.GetDelta(client, latestEpochTime(), nil, nil, nil)
	assert.Equal(t, conn.LastUpdateEpoch, delta.Conns[0].LastUpdateEpoch)
}

func TestDNSStatsWithMultipleClients(t *testing.T) {
	c := ConnectionStats{
		Pid:    123,
		Type:   TCP,
		Family: AFINET,
		Source: util.AddressFromString("127.0.0.1"),
		Dest:   util.AddressFromString("127.0.0.1"),
		SPort:  1000,
		DPort:  53,
	}

	dKey := dns.Key{ClientIP: c.Source, ClientPort: c.SPort, ServerIP: c.Dest, Protocol: getIPProtocol(c.Type)}

	getStats := func() dns.StatsByKeyByNameByType {
		var d = dns.ToHostname("foo.com")
		statsByDomain := make(dns.StatsByKeyByNameByType)
		stats := make(map[dns.QueryType]dns.Stats)
		countByRcode := make(map[uint32]uint32)
		countByRcode[uint32(DNSResponseCodeNoError)] = 1
		stats[dns.TypeA] = dns.Stats{CountByRcode: countByRcode}
		statsByDomain[dKey] = make(map[dns.Hostname]map[dns.QueryType]dns.Stats)
		statsByDomain[dKey][d] = stats
		return statsByDomain
	}

	client1 := "client1"
	client2 := "client2"
	client3 := "client3"
	state := newDefaultState()

	getRCodeFrom := func(delta Delta, c ConnectionStats, domain string, qtype dns.QueryType, code int) uint32 {
		key, _ := DNSKey(&c)
		stats, ok := delta.DNSStats[key]
		require.Truef(t, ok, "couldn't find DNSStats for connection: %+v", c)

		domainStats, ok := stats[dns.ToHostname(domain)]
		require.Truef(t, ok, "couldn't find DNSStats for domain: %s", domain)

		queryTypeStats, ok := domainStats[qtype]
		require.Truef(t, ok, "couldn't find DNSStats for query type: %s", qtype)

		return queryTypeStats.CountByRcode[uint32(code)]
	}

	// Register the first two clients
	state.RegisterClient(client1)
	state.RegisterClient(client2)

	// We should have nothing on first call
	assert.Len(t, state.GetDelta(client1, latestEpochTime(), nil, nil, nil).Conns, 0)
	assert.Len(t, state.GetDelta(client2, latestEpochTime(), nil, nil, nil).Conns, 0)

	c.Monotonic = StatCounters{SentBytes: 100, RecvBytes: 200}
	c.Cookie = 1
	c.LastUpdateEpoch = latestEpochTime()

	delta := state.GetDelta(client1, latestEpochTime(), []ConnectionStats{c}, getStats(), nil)
	require.Len(t, delta.Conns, 1)

	rcode := getRCodeFrom(delta, delta.Conns[0], "foo.com", dns.TypeA, DNSResponseCodeNoError)
	assert.EqualValues(t, 1, rcode)

	// Register the third client but also pass in dns stats
	delta = state.GetDelta(client3, latestEpochTime(), []ConnectionStats{c}, getStats(), nil)
	require.Len(t, delta.Conns, 1)

	// DNS stats should be available for the new client
	rcode = getRCodeFrom(delta, delta.Conns[0], "foo.com", dns.TypeA, DNSResponseCodeNoError)
	assert.EqualValues(t, 1, rcode)

	delta = state.GetDelta(client2, latestEpochTime(), []ConnectionStats{c}, getStats(), nil)
	require.Len(t, delta.Conns, 1)

	// 2nd client should get accumulated stats
	rcode = getRCodeFrom(delta, delta.Conns[0], "foo.com", dns.TypeA, DNSResponseCodeNoError)
	assert.EqualValues(t, 3, rcode)
}

func TestHTTPStats(t *testing.T) {
	c := ConnectionStats{
		Source: util.AddressFromString("1.1.1.1"),
		Dest:   util.AddressFromString("0.0.0.0"),
		SPort:  1000,
		DPort:  80,
	}

	key := http.NewKey(c.Source, c.Dest, c.SPort, c.DPort, "/testpath", true, http.MethodGet)

	httpStats := make(map[http.Key]*http.RequestStats)
	var rs http.RequestStats
	httpStats[key] = &rs

	// Register client & pass in HTTP stats
	state := newDefaultState()
	delta := state.GetDelta("client", latestEpochTime(), []ConnectionStats{c}, nil, httpStats)

	// Verify connection has HTTP data embedded in it
	assert.Len(t, delta.HTTP, 1)

	// Verify HTTP data has been flushed
	delta = state.GetDelta("client", latestEpochTime(), []ConnectionStats{c}, nil, nil)
	assert.Len(t, delta.HTTP, 0)
}

func TestHTTPStatsWithMultipleClients(t *testing.T) {
	c := ConnectionStats{
		Source: util.AddressFromString("1.1.1.1"),
		Dest:   util.AddressFromString("0.0.0.0"),
		SPort:  1000,
		DPort:  80,
	}

	getStats := func(path string) map[http.Key]*http.RequestStats {
		httpStats := make(map[http.Key]*http.RequestStats)
		key := http.NewKey(c.Source, c.Dest, c.SPort, c.DPort, path, true, http.MethodGet)
		var rs http.RequestStats
		httpStats[key] = &rs
		return httpStats
	}

	client1 := "client1"
	client2 := "client2"
	client3 := "client3"
	state := newDefaultState()

	// Register the first two clients
	state.RegisterClient(client1)
	state.RegisterClient(client2)

	// We should have nothing on first call
	assert.Len(t, state.GetDelta(client1, latestEpochTime(), nil, nil, nil).HTTP, 0)
	assert.Len(t, state.GetDelta(client2, latestEpochTime(), nil, nil, nil).HTTP, 0)

	// Store the connection to both clients & pass HTTP stats to the first client
	c.LastUpdateEpoch = latestEpochTime()
	state.StoreClosedConnections([]ConnectionStats{c})

	delta := state.GetDelta(client1, latestEpochTime(), nil, nil, getStats("/testpath"))
	assert.Len(t, delta.HTTP, 1)

	// Verify that the HTTP stats were also stored in the second client
	delta = state.GetDelta(client2, latestEpochTime(), nil, nil, nil)
	assert.Len(t, delta.HTTP, 1)

	// Register a third client & verify that it does not have the HTTP stats
	delta = state.GetDelta(client3, latestEpochTime(), []ConnectionStats{c}, nil, nil)
	assert.Len(t, delta.HTTP, 0)

	c.LastUpdateEpoch = latestEpochTime()
	state.StoreClosedConnections([]ConnectionStats{c})

	// Pass in new HTTP stats to the first client
	delta = state.GetDelta(client1, latestEpochTime(), nil, nil, getStats("/testpath2"))
	assert.Len(t, delta.HTTP, 1)

	// And the second client
	delta = state.GetDelta(client2, latestEpochTime(), nil, nil, getStats("/testpath3"))
	assert.Len(t, delta.HTTP, 2)

	// Verify that the third client also accumulated both new HTTP stats
	delta = state.GetDelta(client3, latestEpochTime(), nil, nil, nil)
	assert.Len(t, delta.HTTP, 2)
}

func TestDetermineConnectionIntraHost(t *testing.T) {
	tests := []struct {
		name      string
		conn      ConnectionStats
		intraHost bool
	}{
		{
			name: "equal source/dest",
			conn: ConnectionStats{
				Source: util.AddressFromString("1.1.1.1"),
				Dest:   util.AddressFromString("1.1.1.1"),
				SPort:  123,
				DPort:  456,
			},
			intraHost: true,
		},
		{
			name: "source/dest loopback",
			conn: ConnectionStats{
				Source: util.AddressFromString("127.0.0.1"),
				Dest:   util.AddressFromString("127.0.0.1"),
				SPort:  123,
				DPort:  456,
			},
			intraHost: true,
		},
		{
			name: "dest nat'ed to loopback",
			conn: ConnectionStats{
				Source: util.AddressFromString("1.1.1.1"),
				Dest:   util.AddressFromString("2.2.2.2"),
				SPort:  123,
				DPort:  456,
				IPTranslation: &IPTranslation{
					ReplSrcIP:   util.AddressFromString("127.0.0.1"),
					ReplDstIP:   util.AddressFromString("1.1.1.1"),
					ReplSrcPort: 456,
					ReplDstPort: 123,
				},
			},
			intraHost: true,
		},
		{
			name: "local connection with nat on both sides",
			conn: ConnectionStats{
				Source:    util.AddressFromString("1.1.1.1"),
				Dest:      util.AddressFromString("169.254.169.254"),
				SPort:     12345,
				DPort:     80,
				Direction: OUTGOING,
				NetNS:     1212,
				IPTranslation: &IPTranslation{
					ReplSrcIP:   util.AddressFromString("127.0.0.1"),
					ReplDstIP:   util.AddressFromString("1.1.1.1"),
					ReplSrcPort: 8181,
					ReplDstPort: 12345,
				},
			},
			intraHost: true,
		},
		{
			name: "local connection with nat on both sides",
			conn: ConnectionStats{
				Source:    util.AddressFromString("127.0.0.1"),
				Dest:      util.AddressFromString("1.1.1.1"),
				SPort:     8181,
				DPort:     12345,
				Direction: INCOMING,
				NetNS:     1233,
				IPTranslation: &IPTranslation{
					ReplSrcIP:   util.AddressFromString("1.1.1.1"),
					ReplDstIP:   util.AddressFromString("169.254.169.254"),
					ReplSrcPort: 12345,
					ReplDstPort: 80,
				},
			},
			intraHost: true,
		},
		{
			name: "remote connection with source translation (redirect)",
			conn: ConnectionStats{
				Source:    util.AddressFromString("4.4.4.4"),
				Dest:      util.AddressFromString("2.2.2.2"),
				SPort:     12345,
				DPort:     80,
				Direction: INCOMING,
				NetNS:     2,
				IPTranslation: &IPTranslation{
					ReplSrcIP:   util.AddressFromString("2.2.2.2"),
					ReplDstIP:   util.AddressFromString("127.0.0.1"),
					ReplSrcPort: 12345,
					ReplDstPort: 15006,
				},
			},
			intraHost: false,
		},
		{
			name: "local connection, same network ns",
			conn: ConnectionStats{
				Source:    util.AddressFromString("1.1.1.1"),
				Dest:      util.AddressFromString("2.2.2.2"),
				SPort:     12345,
				DPort:     80,
				Direction: OUTGOING,
				NetNS:     1,
			},
			intraHost: true,
		},
		{
			name: "local connection, same network ns",
			conn: ConnectionStats{
				Source:    util.AddressFromString("2.2.2.2"),
				Dest:      util.AddressFromString("1.1.1.1"),
				SPort:     80,
				DPort:     12345,
				Direction: INCOMING,
				NetNS:     1,
			},
			intraHost: true,
		},
		{
			name: "local connection, different network ns",
			conn: ConnectionStats{
				Source:    util.AddressFromString("1.1.1.1"),
				Dest:      util.AddressFromString("2.2.2.2"),
				SPort:     12345,
				DPort:     80,
				Direction: OUTGOING,
				NetNS:     1,
			},
			intraHost: true,
		},
		{
			name: "local connection, different network ns",
			conn: ConnectionStats{
				Source:    util.AddressFromString("2.2.2.2"),
				Dest:      util.AddressFromString("1.1.1.1"),
				SPort:     80,
				DPort:     12345,
				Direction: INCOMING,
				NetNS:     2,
			},
			intraHost: true,
		},
		{
			name: "remote connection",
			conn: ConnectionStats{
				Source:    util.AddressFromString("1.1.1.1"),
				Dest:      util.AddressFromString("3.3.3.3"),
				SPort:     12345,
				DPort:     80,
				Direction: OUTGOING,
				NetNS:     1,
			},
			intraHost: false,
		},
	}

	conns := make([]ConnectionStats, 0, len(tests))
	for _, te := range tests {
		conns = append(conns, te.conn)
	}
	state := newDefaultState()
	state.determineConnectionIntraHost(conns)
	for i, te := range tests {
		if i >= len(conns) {
			assert.Failf(t, "missing connection for %s", te.name)
			continue
		}
		c := conns[i]
		assert.Equal(t, te.intraHost, c.IntraHost, "name: %s, conn: %+v", te.name, c)
		if c.Direction == INCOMING {
			if c.IntraHost {
				assert.Nil(t, c.IPTranslation, "name: %s, conn: %+v", te.name, c)
			} else {
				assert.NotNil(t, c.IPTranslation, "name: %s, conn: %+v", te.name, c)
			}
		}
	}
}

func TestClosedMergingWithAddressCollision(t *testing.T) {
	const client = "foo"

	c1 := ConnectionStats{
		Pid:    123,
		Type:   TCP,
		Family: AFINET,
		Source: util.AddressFromString("127.0.0.1"),
		Dest:   util.AddressFromString("127.0.0.1"),
		SPort:  1000,
		DPort:  8080,
		Monotonic: StatCounters{
			SentBytes: 100,
		},
		Cookie: 1,
		IPTranslation: &IPTranslation{
			ReplSrcIP:   util.AddressFromString("1.1.1.1"),
			ReplDstIP:   util.AddressFromString("2.2.2.2"),
			ReplSrcPort: 123,
			ReplDstPort: 456,
		},
	}
	c2 := ConnectionStats{
		Pid:    123,
		Type:   TCP,
		Family: AFINET,
		Source: util.AddressFromString("127.0.0.1"),
		Dest:   util.AddressFromString("127.0.0.1"),
		SPort:  1000,
		DPort:  8080,
		Monotonic: StatCounters{
			SentBytes: 150,
		},
		Cookie: 2,
		IPTranslation: &IPTranslation{
			ReplSrcIP:   util.AddressFromString("3.3.3.3"),
			ReplDstIP:   util.AddressFromString("4.4.4.4"),
			ReplSrcPort: 123,
			ReplDstPort: 456,
		},
	}

	t.Run("ephemeral connections", func(t *testing.T) {
		// tests the state aggregation of *ephemeral* connections that share the
		// same source/destination addresses but that differ in their NAT
		// translations. this tends to happen in high-load scenarios with a lot
		// of connection churn.
		// note that by *ephemeral* we mean connections whose entire lifecycle
		// (eg. TCP_ESTABLISHED to TCP_CLOSE) happens whithin two consecutive
		// connection checks.

		state := newDefaultState()
		state.RegisterClient(client)

		state.StoreClosedConnections([]ConnectionStats{c1})
		state.StoreClosedConnections([]ConnectionStats{c2})

		active := ConnectionStats{
			Pid:    123,
			Type:   TCP,
			Family: AFINET,
			Source: util.AddressFromString("127.0.0.1"),
			Dest:   util.AddressFromString("127.0.0.1"),
			SPort:  1000,
			DPort:  8080,
			Monotonic: StatCounters{
				SentBytes: 150,
			},
			Cookie: 1,
			IPTranslation: &IPTranslation{
				ReplSrcIP:   util.AddressFromString("5.5.5.5"),
				ReplDstIP:   util.AddressFromString("6.6.6.6"),
				ReplSrcPort: 123,
				ReplDstPort: 456,
			},
		}

		// these two connections will be treated as distinct and won't be aggregated.
		// also pass in an active connection with the same (non-nat) tuple; this
		// should aggregated into the first closed connection c1 only
		delta := state.GetDelta(client, latestEpochTime(), []ConnectionStats{active}, nil, nil)
		connections := delta.Conns

		assert.Len(t, delta.Conns, 2)
		assert.Condition(t, func() bool {
			// assert c1 is present
			for _, c := range connections {
				if c.IPTranslation != nil && c.IPTranslation.ReplSrcIP == util.AddressFromString("1.1.1.1") {
					return c.Last.SentBytes == 150 // max(c1, active)
				}
			}
			return false
		})

		assert.Condition(t, func() bool {
			// assert c2 is present
			for _, c := range connections {
				if c.IPTranslation != nil && c.IPTranslation.ReplSrcIP == util.AddressFromString("3.3.3.3") {
					return c.Last.SentBytes == 150 // only c2
				}
			}
			return false
		})
	})

	t.Run("long-lived connection", func(t *testing.T) {
		state := newDefaultState()
		state.RegisterClient(client)

		// despite having different NAT translations these 2 connections will be
		// interpreted as one. we do this because over the lifecycle of a long-lived
		// connection the conntrack entry is looked up multiple times and may change
		// (usually from a nil to a non-nil value). this behavior stems from a
		// *limitation* in our connection tracking code and should be revisited
		// once we find a way to reliably get the NAT translation the *first*
		// time a connection is seen
		_ = state.GetDelta(client, latestEpochTime(), []ConnectionStats{c1}, nil, nil)
		c2.Cookie = c1.Cookie
		state.StoreClosedConnections([]ConnectionStats{c2})

		// assert that the value returned by the second call to `GetDelta` represents c2 - c1
		delta := state.GetDelta(client, latestEpochTime(), nil, nil, nil)
		assert.Len(t, delta.Conns, 1)
		assert.Equal(t, uint64(50), delta.Conns[0].Last.SentBytes)
	})

}

func generateRandConnections(n int) []ConnectionStats {
	cs := make([]ConnectionStats, 0, n)
	for i := 0; i < n; i++ {
		cs = append(cs, ConnectionStats{
			Pid:    123,
			Type:   TCP,
			Family: AFINET,
			Source: util.AddressFromString("127.0.0.1"),
			Dest:   util.AddressFromString("127.0.0.1"),
			SPort:  uint16(rand.Intn(math.MaxUint16)),
			DPort:  uint16(rand.Intn(math.MaxUint16)),
			Monotonic: StatCounters{
				RecvBytes:   rand.Uint64(),
				SentBytes:   rand.Uint64(),
				Retransmits: rand.Uint32(),
			},
		})
	}
	return cs
}

var latestTime atomic.Uint64

func latestEpochTime() uint64 {
	return latestTime.Inc()
}

func newDefaultState() *networkState {
	// Using values from ebpf.NewConfig()
	return NewState(2*time.Minute, 50000, 75000, 75000, 7500).(*networkState)
}

func getIPProtocol(nt ConnectionType) uint8 {
	switch nt {
	case TCP:
		return syscall.IPPROTO_TCP
	case UDP:
		return syscall.IPPROTO_UDP
	default:
		panic("unknown connection type")
	}
}
