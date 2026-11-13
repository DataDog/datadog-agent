// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"fmt"
	"math"
	"testing"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitForTCPEstablishedConnections waits until at least count TCP connections
// to dstPort are in the ESTABLISHED state inside the given container. It counts
// entries in /proc/net/tcp directly, so it does not depend on networking
// tools (ss, netstat) being installed in the container image.
func waitForTCPEstablishedConnections(t *testing.T, host *components.RemoteHost, container string, dstPort uint16, count int, timeout time.Duration) {
	t.Helper()
	// /proc/net/tcp lists the remote address as %08X:%04X followed by the
	// connection state code (01 = ESTABLISHED).
	pattern := fmt.Sprintf(":%04X 01 ", dstPort)
	cmd := fmt.Sprintf(
		`timeout %s bash -c 'until [ "$(docker exec %s grep -c '\''%s'\'' /proc/net/tcp 2>/dev/null || true)" -ge %d ]; do sleep 0.25; done'`,
		timeout.String(), container, pattern, count,
	)
	if _, err := host.Execute(cmd); err != nil {
		t.Fatalf("timed out waiting for %d established connection(s) to port %d in container %s: %v", count, dstPort, container, err)
	}
}

// test1HostFakeIntakeNPMDumpInfo dump information about the test if it failed
func test1HostFakeIntakeNPMDumpInfo(t *testing.T, FakeIntake *components.FakeIntake) {
	if !t.Failed() {
		return
	}
	t.Log("==== test failed dumping fakeintake info ====")
	m, err := FakeIntake.Client().RouteStats()
	if err != nil {
		t.Logf("fakeintake RouteStats() failed %s", err)
		return
	}
	t.Logf("RouteStats %#+v", m)

	cnx, err := FakeIntake.Client().GetConnections()
	if err != nil {
		t.Logf("fakeintake GetConnections() failed %s", err)
		return
	}
	hostnameNetID, err := FakeIntake.Client().GetConnectionsNames()
	if err != nil {
		t.Logf("fakeintake GetConnectionsNames() failed %s", err)
		return
	}
	// Dump info for test1HostFakeIntakeNPM
	for _, h := range hostnameNetID {
		var prevCollectedTime time.Time
		for i, cc := range cnx.GetPayloadsByName(h) {
			if i > 0 {
				dt := cc.GetCollectedTime().Sub(prevCollectedTime).Seconds()
				t.Logf("hostname+networkID %v diff time %f seconds", h, dt)
			}
			prevCollectedTime = cc.GetCollectedTime()
		}
	}
	// Dump info for test1HostFakeIntakeNPM600cnxBucket
	for _, h := range hostnameNetID {
		for _, cc := range cnx.GetPayloadsByName(h) {
			t.Logf("hostname+networkID %v time %v connections %d group %d/%d", h, cc.GetCollectedTime(), len(cc.Connections), cc.GroupId, cc.GroupSize)
		}
	}
}

// testFakeIntakeNPM
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for 5 payloads and check if the last 2 have a span of 30s +/- 500ms
func test1HostFakeIntakeNPM[Env any](v *e2e.BaseSuite[Env], FakeIntake *components.FakeIntake) {
	t := v.T()
	t.Helper()
	t.Log(time.Now())

	targetHostnameNetID := ""
	// looking for 1 host to send CollectorConnections payload to the fakeintake
	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		hostnameNetID, err := FakeIntake.Client().GetConnectionsNames()
		assert.NoError(c, err, "GetConnectionsNames() errors")
		if !assert.NotEmpty(c, hostnameNetID, "no connections yet") {
			return
		}
		targetHostnameNetID = hostnameNetID[0]

		t.Logf("hostname+networkID %v seen connections", hostnameNetID)
	}, 120*time.Second, time.Second, "no connections received")

	// looking for 5 payloads and check if the last 2 have a span of 30s +/- 1s
	v.EventuallyWithT(func(c *assert.CollectT) {
		cnx, err := FakeIntake.Client().GetConnections()
		assert.NoError(t, err)

		if !assert.GreaterOrEqual(c, len(cnx.GetPayloadsByName(targetHostnameNetID)), 5, "not enough payloads") {
			return
		}
		var payloadsTimestamps []time.Time
		for _, cc := range cnx.GetPayloadsByName(targetHostnameNetID) {
			payloadsTimestamps = append(payloadsTimestamps, cc.GetCollectedTime())
		}
		dt := payloadsTimestamps[4].Sub(payloadsTimestamps[3]).Seconds()
		t.Logf("hostname+networkID %v diff time %f seconds", targetHostnameNetID, dt)

		// we want the test fail now, not retrying on the next payloads
		assert.Greater(t, 1.0, math.Abs(dt-30), "delta between collection is higher than 1s")
	}, 150*time.Second, time.Second, "not enough connections received")
}

// maxConnsPerMessage is the default system_probe_config.max_conns_per_message,
// the most connections the agent puts in one CollectorConnections payload.
const maxConnsPerMessage = 600

// maxChunkSpread bounds how long the fakeintake may take to receive every
// chunk of one connections collection. The agent sends the chunks one after
// another, and each send waits for the previous chunk's response, so the spread
// depends on the fakeintake round-trip. CI has measured 6ms to 2.5s. The bound
// stays well under the 30s check interval, so all chunks must still arrive
// together rather than in a later cycle.
const maxChunkSpread = 5 * time.Second

// test1HostFakeIntakeNPM600cnxBucket Validate the agent can communicate with the (fake) backend and send connections
// every 30 seconds with a maximum of 600 connections per payloads, if more another payload will follow.
//   - no payload has more than 600 connections
//   - one collection (one GroupId) was split into GroupSize >= 2 payloads, one of them holding exactly 600 connections
//   - all the payloads of that collection arrived within maxChunkSpread of each other
//
// It searches every payload received since the test started, not only the latest ones: the burst is
// flushed in a single collection and newer collections only carry background connections.
func test1HostFakeIntakeNPM600cnxBucket[Env any](v *e2e.BaseSuite[Env], FakeIntake *components.FakeIntake) {
	t := v.T()
	t.Helper()
	t.Log(time.Now())

	v.EventuallyWithT(func(c *assert.CollectT) {
		cnx, err := FakeIntake.Client().GetConnections()
		if !assert.NoError(c, err, "GetConnections() errors") {
			return
		}
		hostnameNetIDs := cnx.GetNames()
		if !assert.NotEmpty(c, hostnameNetIDs, "no connections yet") {
			return
		}

		cnx.ForeachHostnameConnections(func(cnx *aggregator.Connections, _ string) {
			assert.LessOrEqualf(c, len(cnx.Connections), maxConnsPerMessage, "too many connections in one payload")
		})

		// Check every host, because a fakeintake can receive payloads from more
		// than one host and the burst host is not always the first name.
		found := false
		for _, hostnameNetID := range hostnameNetIDs {
			groups := make(map[int32][]*aggregator.Connections)
			var groupOrder []int32
			for _, payload := range cnx.GetPayloadsByName(hostnameNetID) {
				if _, seen := groups[payload.GroupId]; !seen {
					groupOrder = append(groupOrder, payload.GroupId)
				}
				groups[payload.GroupId] = append(groups[payload.GroupId], payload)
			}

			for _, groupID := range groupOrder {
				chunks := groups[groupID]
				groupSize := int(chunks[0].GroupSize)
				if groupSize < 2 || len(chunks) < groupSize {
					// a single-payload collection, or chunks still in flight
					continue
				}

				totalConnections := 0
				fullChunk := false
				first, last := chunks[0].GetCollectedTime(), chunks[0].GetCollectedTime()
				for _, chunk := range chunks {
					totalConnections += len(chunk.Connections)
					fullChunk = fullChunk || len(chunk.Connections) == maxConnsPerMessage
					if ct := chunk.GetCollectedTime(); ct.Before(first) {
						first = ct
					} else if ct.After(last) {
						last = ct
					}
				}
				spread := last.Sub(first)
				t.Logf("hostname+networkID %v group %d: %d chunks, %d connections, spread %v",
					hostnameNetID, groupID, len(chunks), totalConnections, spread)

				assert.Equalf(c, groupSize, len(chunks), "group %d has more payloads than its GroupSize", groupID)
				assert.Truef(c, fullChunk, "group %d has no chunk with %d connections", groupID, maxConnsPerMessage)
				assert.Greaterf(c, totalConnections, maxConnsPerMessage, "group %d does not hold %d+ connections", groupID, maxConnsPerMessage)
				assert.LessOrEqualf(c, spread, maxChunkSpread, "group %d chunks arrived %v apart", groupID, spread)
				found = true
			}
		}
		assert.True(c, found, "no collection split into several payloads yet")
	}, 90*time.Second, time.Second, "no 600+ connections collection split into chunks")
}

// test1HostFakeIntakeNPMResolvConf validates that connections include resolv.conf
// data. It looks for at least one connection with a non-zero ResolvConfIdx whose
// corresponding entry in CollectorConnections.ResolvConfs contains "nameserver".
// Any connection with a non-zero ResolvConfIdx that is out of bounds of the
// ResolvConfs list is treated as a hard failure.
func test1HostFakeIntakeNPMResolvConf[Env any](v *e2e.BaseSuite[Env], FakeIntake *components.FakeIntake) {
	t := v.T()
	t.Helper()
	t.Log(time.Now())

	v.EventuallyWithT(func(c *assert.CollectT) {
		cnx, err := FakeIntake.Client().GetConnections()
		require.NoError(c, err, "GetConnections() errors")
		require.NotNil(c, cnx, "GetConnections() returned nil ConnectionsAggregator")
		require.NotEmpty(c, cnx.GetNames(), "no connections yet")

		resolvConfsFound := 0
		cnx.ForeachConnection(func(conn *agentmodel.Connection, cc *agentmodel.CollectorConnections, hostname string) {
			if conn.ResolvConfIdx < 0 {
				return
			}
			// fail the whole test if out of bounds
			require.Less(t, int(conn.ResolvConfIdx), len(cc.ResolvConfs),
				"ResolvConfIdx %d out of bounds (len=%d) on host %s",
				conn.ResolvConfIdx, len(cc.ResolvConfs), hostname)

			resolvConfsFound++
			rc := cc.ResolvConfs[conn.ResolvConfIdx]
			t.Logf("found resolv.conf data: idx=%d content=%q", conn.ResolvConfIdx, rc)

			require.Contains(c, rc, "nameserver", "resolv.conf data didn't contain a nameserver")
		})
		assert.NotZero(c, resolvConfsFound, "no connection with resolv.conf data found")
	}, 60*time.Second, time.Second)
}

// test1HostFakeIntakeNPMTCPUDPDNS validate we received tcp, udp, and DNS connections
// with some basic checks, like IPs/Ports present, DNS query has been captured, ...
func test1HostFakeIntakeNPMTCPUDPDNS[Env any](v *e2e.BaseSuite[Env], FakeIntake *components.FakeIntake) {
	t := v.T()
	t.Helper()
	t.Log(time.Now())

	v.EventuallyWithT(func(c *assert.CollectT) {
		cnx, err := FakeIntake.Client().GetConnections()
		assert.NoError(c, err, "GetConnections() errors")
		if !assert.NotNil(c, cnx, "GetConnections() returned nil ConnectionsAggregator") {
			return
		}

		if !assert.NotEmpty(c, cnx.GetNames(), "no connections yet") {
			return
		}

		foundDNS := false
		cnx.ForeachConnection(func(c *agentmodel.Connection, _ *agentmodel.CollectorConnections, _ string) {
			if len(c.DnsStatsByDomainOffsetByQueryType) > 0 {
				foundDNS = true
			}
		})
		if !assert.True(c, foundDNS, "DNS not found") {
			return
		}

		type countCnx struct {
			hit int
			TCP int
			UDP int
		}
		countConnections := make(map[string]*countCnx)

		helperCleanup(t)
		cnx.ForeachConnection(func(c *agentmodel.Connection, cc *agentmodel.CollectorConnections, hostname string) {
			var count *countCnx
			var found bool
			if count, found = countConnections[hostname]; !found {
				countConnections[hostname] = &countCnx{}
				count = countConnections[hostname]
			}
			count.hit++

			switch c.Type {
			case agentmodel.ConnectionType_tcp:
				count.TCP++
			case agentmodel.ConnectionType_udp:
				count.UDP++
			}
			validateConnection(t, c, cc, hostname)
		})

		totalConnections := countCnx{}
		for host, count := range countConnections {
			t.Logf("connections %d tcp %d udp %d received by host/netID %s", count.hit, count.TCP, count.UDP, host)
			totalConnections.hit += count.hit
			totalConnections.TCP += count.TCP
			totalConnections.UDP += count.UDP
		}
		t.Logf("sum connections %d tcp %d udp %d", totalConnections.hit, totalConnections.TCP, totalConnections.UDP)
	}, 60*time.Second, time.Second)
}
