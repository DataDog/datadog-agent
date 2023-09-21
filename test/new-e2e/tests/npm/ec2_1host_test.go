// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"math"
	"os"
	"testing"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	krpretty "github.com/kr/pretty"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
)

const NPMsystemProbeConfig = `
network_config:
  enabled: true
`

type ec2VMSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
	DevMode bool
}

// TestEC2VMSuite will validate running the agent on a single EC2 VM
func TestEC2VMSuite(t *testing.T) {
	s := &ec2VMSuite{}
	e2eParams := []params.Option{}
	// debug helper
	if _, devmode := os.LookupEnv("TESTS_E2E_DEVMODE"); devmode {
		e2eParams = []params.Option{params.WithDevMode(), params.WithSkipDeleteOnFailure()}
		s.DevMode = true
	}

	// Source of our kitchen CI images test/kitchen/platforms.json
	// Other VM image can be used, our kitchen CI images test/kitchen/platforms.json
	// ec2params.WithImageName("ami-a4dc46db", os.AMD64Arch, ec2os.AmazonLinuxOS) // ubuntu-16-04-4.4
	e2e.Run(t, s, e2e.FakeIntakeStackDef(nil, agentparams.WithSystemProbeConfig(NPMsystemProbeConfig)), e2eParams...)
}

// TestFakeIntakeNPM Validate the agent can communicate with the (fake) backend and send connections every 30 seconds
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for 3 payloads and check if the last 2 have a span of 30s +/- 500ms
func (v *ec2VMSuite) TestFakeIntakeNPM() {
	t := v.T()

	// default is to reset the current state of the fakeintake aggregators
	if !v.DevMode {
		v.Env().Fakeintake.FlushServerAndResetAggregators()
	}

	targetHostnameNetID := ""
	// looking for 1 host to send CollectorConnections payload to the fakeintake
	v.EventuallyWithT(func(c *assert.CollectT) {
		// generate a connection
		v.Env().VM.Execute("curl http://www.datadoghq.com")

		hostnameNetID, err := v.Env().Fakeintake.GetConnectionsNames()
		assert.NoError(c, err, "GetConnectionsNames() errors")
		assert.NotZero(c, len(hostnameNetID), "no connections yet")
		targetHostnameNetID = hostnameNetID[0]

		t.Logf("hostname+networkID %v seen connections", hostnameNetID)
	}, 60*time.Second, time.Second, "no connections received")

	// looking for 3 payloads and check if the last 2 have a span of 30s +/- 500ms
	v.EventuallyWithT(func(c *assert.CollectT) {
		cnx, err := v.Env().Fakeintake.GetConnections()
		assert.NoError(t, err)

		assert.Greater(c, len(cnx.GetPayloadsByName(targetHostnameNetID)), 2, "not enough payloads")
		if len(cnx.GetPayloadsByName(targetHostnameNetID)) < 3 {
			return
		}

		var payloadsTimestamps []time.Time
		for _, cc := range cnx.GetPayloadsByName(targetHostnameNetID) {
			payloadsTimestamps = append(payloadsTimestamps, cc.GetCollectedTime())
		}
		dt := float64(payloadsTimestamps[2].Sub(payloadsTimestamps[1])) / float64(time.Second)
		t.Logf("hostname+networkID %v diff time %f seconds", targetHostnameNetID, dt)

		// we want the test fail now, not retrying on the next payloads
		if math.Abs(dt-30) > 0.5 {
			t.Log("delta between collection is higher that 500ms")
			c.FailNow()
		}
	}, 90*time.Second, time.Second, "no connections received")
}

// TestFakeIntakeNPM_TCP_UDP_DNS validate we received tcp, udp, and DNS connections
// with some basic checks, like IPs/Ports present, DNS query has been captured, ...
func (v *ec2VMSuite) TestFakeIntakeNPM_TCP_UDP_DNS() {
	t := v.T()

	v.EventuallyWithT(func(c *assert.CollectT) {
		// generate connections
		v.Env().VM.Execute("curl http://www.datadoghq.com")
		v.Env().VM.Execute("dig @8.8.8.8 www.google.ch")

		cnx, err := v.Env().Fakeintake.GetConnections()
		assert.NoError(c, err)

		var currentHostname string
		var currentConnection *agentmodel.Connection
		t.Cleanup(func() {
			if t.Failed() {
				t.Logf(krpretty.Sprintf("test failed on host %s at connection %# v", currentHostname, currentConnection))
			}
		})

		foundDNS := false
		cnx.ForeachConnection(func(c *agentmodel.Connection, cc *agentmodel.CollectorConnections, hostname string) {
			currentHostname = hostname
			currentConnection = c
			if len(c.DnsStatsByDomainOffsetByQueryType) > 0 {
				foundDNS = true
			}
		})
		assert.True(c, foundDNS, "DNS not found")

		type countCnx struct {
			hit int
			TCP int
			UDP int
		}
		countConnections := make(map[string]*countCnx)

		cnx.ForeachConnection(func(c *agentmodel.Connection, cc *agentmodel.CollectorConnections, hostname string) {
			currentHostname = hostname
			currentConnection = c
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
