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

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ec2VMSuite struct {
	e2e.BaseSuite[environments.Host]
	DevMode bool
}

// TestEC2VMSuite will validate running the agent on a single EC2 VM
func TestEC2VMSuite(t *testing.T) {
	s := &ec2VMSuite{}
	e2eParams := []e2e.SuiteOption{e2e.WithProvisioner(awshost.Provisioner(awshost.WithAgentOptions(agentparams.WithSystemProbeConfig(systemProbeConfigNPM))))}
	// debug helper
	if _, devmode := os.LookupEnv("TESTS_E2E_DEVMODE"); devmode {
		e2eParams = []e2e.SuiteOption{e2e.WithDevMode()}
	}

	// Source of our kitchen CI images test/kitchen/platforms.json
	// Other VM image can be used, our kitchen CI images test/kitchen/platforms.json
	// ec2params.WithImageName("ami-a4dc46db", os.AMD64Arch, ec2os.AmazonLinuxOS) // ubuntu-16-04-4.4
	e2e.Run(t, s, e2eParams...)
}

// TestFakeIntakeNPM Validate the agent can communicate with the (fake) backend and send connections every 30 seconds
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for 3 payloads and check if the last 2 have a span of 30s +/- 500ms
func (v *ec2VMSuite) TestFakeIntakeNPM() {
	t := v.T()

	// default is to reset the current state of the fakeintake aggregators
	if !v.DevMode {
		v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}

	targetHostnameNetID := ""
	// looking for 1 host to send CollectorConnections payload to the fakeintake
	v.EventuallyWithT(func(c *assert.CollectT) {
		// generate a connection
		v.Env().RemoteHost.MustExecute("curl http://www.datadoghq.com")

		hostnameNetID, err := v.Env().FakeIntake.Client().GetConnectionsNames()
		assert.NoError(c, err, "GetConnectionsNames() errors")
		if !assert.NotZero(c, len(hostnameNetID), "no connections yet") {
			return
		}
		targetHostnameNetID = hostnameNetID[0]

		t.Logf("hostname+networkID %v seen connections", hostnameNetID)
	}, 60*time.Second, time.Second, "no connections received")

	// looking for 3 payloads and check if the last 2 have a span of 30s +/- 500ms
	v.EventuallyWithT(func(c *assert.CollectT) {
		cnx, err := v.Env().FakeIntake.Client().GetConnections()
		assert.NoError(t, err)

		if !assert.Greater(c, len(cnx.GetPayloadsByName(targetHostnameNetID)), 2, "not enough payloads") {
			return
		}
		var payloadsTimestamps []time.Time
		for _, cc := range cnx.GetPayloadsByName(targetHostnameNetID) {
			payloadsTimestamps = append(payloadsTimestamps, cc.GetCollectedTime())
		}
		dt := payloadsTimestamps[2].Sub(payloadsTimestamps[1]).Seconds()
		t.Logf("hostname+networkID %v diff time %f seconds", targetHostnameNetID, dt)

		// we want the test fail now, not retrying on the next payloads
		require.Greater(t, 0.5, math.Abs(dt-30), "delta between collection is higher than 500ms")
	}, 90*time.Second, time.Second, "not enough connections received")
}

// TestFakeIntakeNPM_TCP_UDP_DNS validate we received tcp, udp, and DNS connections
// with some basic checks, like IPs/Ports present, DNS query has been captured, ...
func (v *ec2VMSuite) TestFakeIntakeNPM_TCP_UDP_DNS() {
	t := v.T()

	// default is to reset the current state of the fakeintake aggregators
	if !v.DevMode {
		v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}

	v.EventuallyWithT(func(c *assert.CollectT) {
		// generate connections
		v.Env().RemoteHost.MustExecute("curl http://www.datadoghq.com")
		v.Env().RemoteHost.MustExecute("dig @8.8.8.8 www.google.ch")

		cnx, err := v.Env().FakeIntake.Client().GetConnections()
		require.NoError(c, err)

		foundDNS := false
		cnx.ForeachConnection(func(c *agentmodel.Connection, cc *agentmodel.CollectorConnections, hostname string) {
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
