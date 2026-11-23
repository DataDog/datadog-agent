// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

// Plain unit tests of the pure helpers of test1HostFakeIntakeNPM600cnxBucket.
// They need no infrastructure and run in the Bazel test jobs (see BUILD.bazel).

import (
	"testing"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

var unitTestBase = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

// conns returns n connections from ports first, first+1, ... to remotePort.
func conns(n int, first int32, remotePort int32, dir agentmodel.ConnectionDirection) []*agentmodel.Connection {
	out := make([]*agentmodel.Connection, n)
	for i := range out {
		out[i] = &agentmodel.Connection{
			Pid:       42,
			Direction: dir,
			Laddr:     &agentmodel.Addr{Ip: "10.0.0.1", Port: first + int32(i)},
			Raddr:     &agentmodel.Addr{Ip: "10.0.0.2", Port: remotePort},
		}
	}
	return out
}

// payload builds a fakeintake connections payload received at offset after unitTestBase.
func payload(t *testing.T, groupID, groupSize int32, offset time.Duration, connections []*agentmodel.Connection) *aggregator.Connections {
	t.Helper()
	data, err := agentmodel.EncodeMessage(agentmodel.Message{
		Header: agentmodel.MessageHeader{
			Version:  agentmodel.MessageV3,
			Encoding: agentmodel.MessageEncodingProtobuf,
			Type:     agentmodel.TypeCollectorConnections,
		},
		Body: &agentmodel.CollectorConnections{GroupId: groupID, GroupSize: groupSize, Connections: connections},
	})
	require.NoError(t, err)
	parsed, err := aggregator.ParseConnections(api.Payload{Timestamp: unitTestBase.Add(offset), Data: data})
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	return parsed[0]
}

func toServer(c *agentmodel.Connection) bool {
	return c.GetRaddr().GetPort() == httpBinPort
}

func TestCheckConnectionsGroups(t *testing.T) {
	out := agentmodel.ConnectionDirection_outgoing
	burst := func(id int32, offsets ...time.Duration) []*aggregator.Connections {
		return []*aggregator.Connections{
			payload(t, id, 3, offsets[0], conns(600, 1000, httpBinPort, out)),
			payload(t, id, 3, offsets[1], conns(600, 2000, httpBinPort, out)),
			payload(t, id, 3, offsets[2], conns(300, 3000, httpBinPort, out)),
		}
	}

	t.Run("complete burst passes", func(t *testing.T) {
		groups, oversized := checkConnectionsGroups(burst(1, time.Second, time.Second, 2*time.Second), unitTestBase, toServer)
		assert.Empty(t, oversized)
		require.Len(t, groups, 1)
		g := groups[0]
		assert.Empty(t, g.problems)
		assert.Equal(t, 3, g.chunks)
		assert.Equal(t, 1500, g.connections)
		assert.Equal(t, 1500, g.serverConnections)
		assert.Equal(t, time.Second, g.spread)
	})

	t.Run("incomplete group is reported in flight", func(t *testing.T) {
		groups, _ := checkConnectionsGroups(burst(1, time.Second, time.Second, 2*time.Second)[:2], unitTestBase, toServer)
		require.Len(t, groups, 1)
		assert.Equal(t, []string{"2 of 3 chunks (in flight)"}, groups[0].problems)
	})

	t.Run("payloads up to the cutoff are ignored", func(t *testing.T) {
		groups, _ := checkConnectionsGroups(burst(1, 0, time.Second, time.Second), unitTestBase, toServer)
		require.Len(t, groups, 1)
		assert.Equal(t, []string{"2 of 3 chunks (in flight)"}, groups[0].problems)
	})

	t.Run("single-payload collections are skipped", func(t *testing.T) {
		groups, _ := checkConnectionsGroups([]*aggregator.Connections{
			payload(t, 2, 1, time.Second, conns(10, 1000, httpBinPort, out)),
		}, unitTestBase, toServer)
		assert.Empty(t, groups)
	})

	t.Run("resent chunk is counted once", func(t *testing.T) {
		payloads := burst(1, time.Second, time.Second, 2*time.Second)
		payloads = append(payloads, payload(t, 1, 3, 3*time.Second, conns(600, 2000, httpBinPort, out)))
		groups, _ := checkConnectionsGroups(payloads, unitTestBase, toServer)
		require.Len(t, groups, 1)
		assert.Empty(t, groups[0].problems)
		assert.Equal(t, 1, groups[0].duplicates)
		assert.Equal(t, 3, groups[0].chunks)
	})

	t.Run("chunks differing only in direction are distinct", func(t *testing.T) {
		payloads := []*aggregator.Connections{
			payload(t, 1, 2, time.Second, conns(600, 1000, httpBinPort, out)),
			payload(t, 1, 2, time.Second, conns(600, 1000, httpBinPort, agentmodel.ConnectionDirection_incoming)),
		}
		groups, _ := checkConnectionsGroups(payloads, unitTestBase, toServer)
		require.Len(t, groups, 1)
		assert.Equal(t, 2, groups[0].chunks)
		assert.Zero(t, groups[0].duplicates)
	})

	t.Run("chunks too far apart fail", func(t *testing.T) {
		groups, _ := checkConnectionsGroups(burst(1, time.Second, time.Second, time.Second+maxChunkSpread+time.Second), unitTestBase, toServer)
		require.Len(t, groups, 1)
		require.Len(t, groups[0].problems, 1)
		assert.Contains(t, groups[0].problems[0], "apart")
	})

	t.Run("background collection fails", func(t *testing.T) {
		groups, _ := checkConnectionsGroups([]*aggregator.Connections{
			payload(t, 3, 2, time.Second, conns(500, 1000, 443, out)),
			payload(t, 3, 2, time.Second, conns(400, 2000, httpBinPort, out)),
		}, unitTestBase, toServer)
		require.Len(t, groups, 1)
		assert.Len(t, groups[0].problems, 2)
	})

	t.Run("oversized payload is reported", func(t *testing.T) {
		_, oversized := checkConnectionsGroups([]*aggregator.Connections{
			payload(t, 4, 1, time.Second, conns(maxConnsPerMessage+1, 1000, httpBinPort, out)),
		}, unitTestBase, toServer)
		assert.Equal(t, []int32{4}, oversized)
	})
}

func TestHostsWithoutNewCollection(t *testing.T) {
	out := agentmodel.ConnectionDirection_outgoing
	known := map[string]map[int32]bool{
		"new":   {1: true},
		"old":   {1: true},
		"stale": {1: true},
	}
	payloadsByHost := map[string][]*aggregator.Connections{
		"new": {
			payload(t, 1, 1, 0, conns(1, 1000, 80, out)),
			payload(t, 2, 1, 40*time.Second, conns(1, 1000, 80, out)),
		},
		"old": {
			payload(t, 1, 1, 30*time.Second, conns(1, 1000, 80, out)),
		},
		// last reported more than an interval plus maxChunkSpread before the newest payload
		"stale": {
			payload(t, 1, 1, 0, conns(1, 1000, 80, out)),
		},
	}
	pending, active, newest := hostsWithoutNewCollection(payloadsByHost, known)
	assert.Equal(t, []string{"old"}, pending)
	assert.Equal(t, 2, active)
	assert.Equal(t, unitTestBase.Add(40*time.Second), newest)

	pending, active, _ = hostsWithoutNewCollection(map[string][]*aggregator.Connections{}, known)
	assert.Empty(t, pending)
	assert.Zero(t, active)
}
