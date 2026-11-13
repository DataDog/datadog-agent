// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"fmt"
	"math"
	"net"
	"strings"
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

// httpBinPort is the port the HTTPBin host serves the test URL on.
const httpBinPort = 8080

// connectionsCollectionTimeout bounds the wait for one connections check (30s
// interval) to reach the fakeintake.
const connectionsCollectionTimeout = 90 * time.Second

// waitForConnectionsCollectedAfter waits until every host that recently sent
// connections to the fakeintake (and at least one host) has sent a payload
// received after the given time. The agent gathers a collection right before
// sending it, so once this returns, each host has reported every connection
// closed before that time.
func waitForConnectionsCollectedAfter[Env any](v *e2e.BaseSuite[Env], FakeIntake *components.FakeIntake, after time.Time) {
	t := v.T()
	t.Helper()
	v.EventuallyWithT(func(c *assert.CollectT) {
		cnx, err := FakeIntake.Client().GetConnections()
		if !assert.NoError(c, err, "GetConnections() errors") {
			return
		}
		active := 0
		for _, hostnameNetID := range cnx.GetNames() {
			var latest time.Time
			for _, payload := range cnx.GetPayloadsByName(hostnameNetID) {
				if ct := payload.GetCollectedTime(); ct.After(latest) {
					latest = ct
				}
			}
			// ignore hosts that stopped reporting, which only dev mode (no flush) keeps around
			if latest.Before(after.Add(-time.Minute)) {
				continue
			}
			active++
			assert.Truef(c, latest.After(after), "hostname+networkID %v has not sent connections since %v", hostnameNetID, after)
		}
		assert.NotZero(c, active, "no connections yet")
	}, connectionsCollectionTimeout, time.Second, "no connections collection received after %v", after)
}

// connectionKey identifies a connection inside one collection.
func connectionKey(c *agentmodel.Connection) string {
	return fmt.Sprintf("%d/%d/%d/%s:%d/%s:%d", c.Type, c.Pid, c.NetNS,
		c.GetLaddr().GetIp(), c.GetLaddr().GetPort(), c.GetRaddr().GetIp(), c.GetRaddr().GetPort())
}

// connectionsGroup summarizes one complete multi-payload collection (one GroupId) of a host.
type connectionsGroup struct {
	id                int32
	chunks            int
	connections       int
	serverConnections int
	spread            time.Duration
	// problems lists the checks the group fails; a group with none is the burst collection.
	problems []string
}

// checkConnectionsGroups groups the payloads of one host received from since on by GroupId,
// counting a resent chunk once, and checks every complete group that was split into several
// payloads. It also returns the GroupIds of payloads holding more than maxConnsPerMessage
// connections, and how many duplicate chunks it skipped.
func checkConnectionsGroups(payloads []*aggregator.Connections, since time.Time, isServerConnection func(*agentmodel.Connection) bool) (groups []connectionsGroup, oversized []int32, duplicates int) {
	byID := make(map[int32][]*aggregator.Connections)
	chunkSeen := make(map[int32]map[string]bool)
	var order []int32
	for _, payload := range payloads {
		if payload.GetCollectedTime().Before(since) {
			continue
		}
		if len(payload.Connections) > maxConnsPerMessage {
			oversized = append(oversized, payload.GroupId)
		}
		if _, seen := byID[payload.GroupId]; !seen {
			order = append(order, payload.GroupId)
			chunkSeen[payload.GroupId] = make(map[string]bool)
		}
		// The chunks of one collection hold disjoint connections, so the first
		// connection identifies a chunk, and a resent chunk is skipped.
		key := ""
		if len(payload.Connections) > 0 {
			key = connectionKey(payload.Connections[0])
		}
		if chunkSeen[payload.GroupId][key] {
			duplicates++
			continue
		}
		chunkSeen[payload.GroupId][key] = true
		byID[payload.GroupId] = append(byID[payload.GroupId], payload)
	}

	for _, id := range order {
		chunks := byID[id]
		groupSize := int(chunks[0].GroupSize)
		if groupSize < 2 || len(chunks) < groupSize {
			// a single-payload collection, or chunks still in flight
			continue
		}

		g := connectionsGroup{id: id, chunks: len(chunks)}
		fullChunk := false
		first, last := chunks[0].GetCollectedTime(), chunks[0].GetCollectedTime()
		for _, chunk := range chunks {
			g.connections += len(chunk.Connections)
			for _, conn := range chunk.Connections {
				if isServerConnection(conn) {
					g.serverConnections++
				}
			}
			fullChunk = fullChunk || len(chunk.Connections) == maxConnsPerMessage
			if ct := chunk.GetCollectedTime(); ct.Before(first) {
				first = ct
			} else if ct.After(last) {
				last = ct
			}
		}
		g.spread = last.Sub(first)

		if len(chunks) != groupSize {
			g.problems = append(g.problems, fmt.Sprintf("%d distinct payloads for GroupSize %d", len(chunks), groupSize))
		}
		if g.serverConnections <= maxConnsPerMessage {
			g.problems = append(g.problems, fmt.Sprintf("only %d connections to the server, not the burst", g.serverConnections))
		}
		if !fullChunk {
			g.problems = append(g.problems, fmt.Sprintf("no chunk with %d connections", maxConnsPerMessage))
		}
		if g.spread > maxChunkSpread {
			g.problems = append(g.problems, fmt.Sprintf("chunks arrived %v apart, more than %v", g.spread, maxChunkSpread))
		}
		groups = append(groups, g)
	}
	return groups, oversized, duplicates
}

// test1HostFakeIntakeNPM600cnxBucket Validate the agent can communicate with the (fake) backend and send connections
// every 30 seconds with a maximum of 600 connections per payloads, if more another payload will follow.
//   - no payload has more than 600 connections
//   - one collection (one GroupId) holding more than 600 connections to serverAddr:httpBinPort, the burst generated
//     by generateConnections, was split into GroupSize >= 2 payloads, one of them holding exactly 600 connections
//   - all the payloads of that collection arrived within maxChunkSpread of each other
//
// The burst must be told apart from earlier connections to the same server. The previous test may have passed on a
// collection gathered while its own burst was running, and dev mode skips the fakeintake flush between tests, so
// the rest of an older burst can still be on its way. Before calling generateConnections, the helper waits for one
// collection gathered after it was called, which carries any such leftovers, and then ignores every payload received
// before the burst starts. A collection straddling that cutoff is left incomplete and skipped.
//
// A collection only qualifies when it is complete. Incomplete collections (chunks still in flight) and collections
// failing a check are logged and skipped, so background collections can't fail the test: it passes as soon as one
// collection satisfies every check. Duplicate chunks, e.g. a forwarder retry, are counted once.
func test1HostFakeIntakeNPM600cnxBucket[Env any](v *e2e.BaseSuite[Env], FakeIntake *components.FakeIntake, serverAddr string, generateConnections func()) {
	t := v.T()
	t.Helper()

	waitForConnectionsCollectedAfter(v, FakeIntake, time.Now())
	workloadStart := time.Now()
	t.Logf("%v, generating connections and ignoring payloads received before it", workloadStart)
	generateConnections()

	// serverAddr is an IP in the EC2 environments; only compare it when it is one
	serverIsIP := net.ParseIP(serverAddr) != nil
	isServerConnection := func(c *agentmodel.Connection) bool {
		return c.GetRaddr().GetPort() == httpBinPort && (!serverIsIP || c.GetRaddr().GetIp() == serverAddr)
	}

	logged := make(map[string]bool)
	v.EventuallyWithT(func(c *assert.CollectT) {
		cnx, err := FakeIntake.Client().GetConnections()
		if !assert.NoError(c, err, "GetConnections() errors") {
			return
		}
		hostnameNetIDs := cnx.GetNames()
		if !assert.NotEmpty(c, hostnameNetIDs, "no connections yet") {
			return
		}

		// Check every host, because a fakeintake can receive payloads from more
		// than one host and the burst host is not always the first name.
		found := false
		for _, hostnameNetID := range hostnameNetIDs {
			groups, oversized, duplicates := checkConnectionsGroups(cnx.GetPayloadsByName(hostnameNetID), workloadStart, isServerConnection)
			assert.Emptyf(c, oversized, "hostname+networkID %v: groups with a payload of more than %d connections", hostnameNetID, maxConnsPerMessage)
			for _, g := range groups {
				verdict := "OK"
				if len(g.problems) > 0 {
					verdict = "skipped: " + strings.Join(g.problems, "; ")
				} else {
					found = true
				}
				// log each group once, not on every tick
				if key := fmt.Sprintf("%s/%d/%s", hostnameNetID, g.id, verdict); !logged[key] {
					logged[key] = true
					t.Logf("hostname+networkID %v group %d: %d chunks, %d connections (%d to %s:%d), spread %v, %d duplicate chunks on the host, %s",
						hostnameNetID, g.id, g.chunks, g.connections, g.serverConnections, serverAddr, httpBinPort, g.spread, duplicates, verdict)
				}
			}
		}
		assert.True(c, found, "no complete collection of the burst split into several payloads yet")
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
