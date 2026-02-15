// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
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
			t.Logf("hostname+networkID %v time %v connections %d", h, cc.GetCollectedTime(), len(cc.Connections))
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

// test1HostFakeIntakeNPM600cnxBucket Validate the agent can communicate with the (fake) backend and send connections
// every 30 seconds with a maximum of 600 connections per payloads, if more another payload will follow.
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for n payloads and check if the last 2 have a maximum span of 200ms
func test1HostFakeIntakeNPM600cnxBucket[Env any](v *e2e.BaseSuite[Env], FakeIntake *components.FakeIntake) {
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
	}, 60*time.Second, time.Second, "no connections received")

	// looking for x payloads (with max 600 connections) and check if the last 2 have a max span of 200ms
	v.EventuallyWithT(func(c *assert.CollectT) {
		cnx, err := FakeIntake.Client().GetConnections()
		assert.NoError(c, err)

		if !assert.GreaterOrEqualf(c, len(cnx.GetPayloadsByName(targetHostnameNetID)), 2, "not enough payloads") {
			return
		}

		cnx.ForeachHostnameConnections(func(cnx *aggregator.Connections, _ string) {
			assert.LessOrEqualf(c, len(cnx.Connections), 600, "too many payloads")
		})

		hostPayloads := cnx.GetPayloadsByName(targetHostnameNetID)
		lastTwoPayloads := hostPayloads[len(hostPayloads)-2:]

		totalConnections := 0
		// the last two should have 600+ connections. The benchmark is set to send 1500 connections,
		// so if the connections check happens to occur exactly at the same time as the benchmark,
		// one side or the other will have 600+ connections.
		for _, payload := range lastTwoPayloads {
			totalConnections += len(payload.Connections)
		}
		assert.GreaterOrEqualf(c, totalConnections, 600, "can't find enough connections 600+")
		cnx600PayloadTime := lastTwoPayloads[0].GetCollectedTime()
		latestPayloadTime := lastTwoPayloads[1].GetCollectedTime()

		dt := latestPayloadTime.Sub(cnx600PayloadTime).Seconds()
		t.Logf("hostname+networkID %v diff time %f seconds", targetHostnameNetID, dt)

		assert.Greater(c, 0.2, dt, "delta between collection is higher than 200ms")
	}, 90*time.Second, time.Second, "not enough connections received")
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
