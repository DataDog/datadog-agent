// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"errors"
	"math"
	"testing"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	krpretty "github.com/kr/pretty"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/require"
)

const NPMsystemProbeConfig = `
network_config:
  enabled: true
`

type ec2VMSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

// Source of our kitchen CI images test/kitchen/platforms.json
func TestEC2VMSuite(t *testing.T) {
	// Other VM image can be used, our kitchen CI images test/kitchen/platforms.json
	// ec2params.WithImageName("ami-a4dc46db", os.AMD64Arch, ec2os.AmazonLinuxOS) // ubuntu-16-04-4.4

	e2e.Run(t, &ec2VMSuite{}, e2e.FakeIntakeStackDef(nil, agentparams.WithSystemProbeConfig(NPMsystemProbeConfig)))
}

func nplanelend(t *testing.T) {
	t.Logf("==end==")
}

func (v *ec2VMSuite) TestFakeIntakeNPM() {
	t := v.T()

	v.Env().Fakeintake.FlushServerAndResetAggregators()
	defer nplanelend(t)

	targetHostnameNetID := ""
	// looking for 1 host to send CollectorConnections payload to the fakeintake
	err := backoff.Retry(func() error {
		// generate a connection
		v.Env().VM.Execute("curl http://www.datadoghq.com")

		hostnameNetID, err := v.Env().Fakeintake.GetConnectionsNames()
		if err != nil {
			return err
		}
		if len(hostnameNetID) == 0 {
			return errors.New("no connections yet")
		}
		targetHostnameNetID = hostnameNetID[0]

		t.Logf("hostname+networkID %v seen connections", hostnameNetID)
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(time.Second), 60))
	require.NoError(t, err)

	// looking for 3 payloads and check if the last 2 have a span of 30s +/- 500ms
	err = backoff.Retry(func() error {
		cnx, err := v.Env().Fakeintake.GetConnections()
		require.NoError(t, err)

		payloadsTimestamp := cnx.GetCollectedTimeByName(targetHostnameNetID)
		if len(payloadsTimestamp) < 3 {
			return errors.New("not enough payloads")
		}

		dt := float64(payloadsTimestamp[2].Sub(payloadsTimestamp[1])) / float64(time.Second)
		t.Logf("hostname+networkID %v diff time %f seconds", targetHostnameNetID, dt)
		require.LessOrEqual(t, math.Abs(dt-30), 0.5, "delta between collection is higher that 500ms")
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(time.Second), 90))
	require.NoError(t, err)
}

func validateConnection(t *testing.T, c *agentmodel.Connection) {
	require.NotZero(t, c.Pid, "Pid = 0")
	require.NotZero(t, c.NetNS, "network namespace = 0")
	require.NotNil(t, c.Laddr, "Laddr is nil")
	require.NotNil(t, c.Raddr, "Raddr is nil")

	// un-comment the line below when https://datadoghq.atlassian.net/browse/NPM-2958 will be fixed
	// require.False(t, c.LastPacketsSent == 0 && c.LastPacketsReceived == 0, "connection with no packets")

	switch c.Type {
	case agentmodel.ConnectionType_tcp:
		validateTCPConnection(t, c)
	case agentmodel.ConnectionType_udp:
		validateUDPConnection(t, c)
	}

	validateDNSConnection(t, c)
}

func validateDNSConnection(t *testing.T, c *agentmodel.Connection) {
	if c.DnsFailedResponses > 0 || c.DnsSuccessfulResponses > 0 {
		t.Logf(krpretty.Sprintf("DNS %# v", c))
	}
}

func validateTCPConnection(t *testing.T, c *agentmodel.Connection) {
	require.Equal(t, c.Type, agentmodel.ConnectionType_tcp, "connection is not TCP")
	/*
		require.NotZero(t, c.LastRetransmits, "LastRetransmits = 0")
		require.NotZero(t, c.LastTcpClosed, "LastTcpClosed = 0")
		require.NotZero(t, c.LastTcpEstablished, "LastTcpEstablished = 0")
	*/
	require.NotZero(t, c.Rtt, "Rtt = 0")
	require.NotZero(t, c.RttVar, "RttVar = 0")
}

func validateUDPConnection(t *testing.T, c *agentmodel.Connection) {
	require.Equal(t, c.Type, agentmodel.ConnectionType_udp, "connection is not UDP")

	require.Zero(t, c.Rtt, "Rtt != 0")
	require.Zero(t, c.RttVar, "RttVar != 0")

	// we can this only for UDP connection as there are no empty payload packets
	// technically possible but in reality no UDP protocol implement that
	// require.False(t, c.LastBytesSent == 0 && c.LastBytesReceived == 0, "connection with no packet bytes")
}

func (v *ec2VMSuite) TestFakeIntakeNPM_TCP_UDP_DNS() {
	t := v.T()
	defer nplanelend(t)

	err := backoff.Retry(func() error {
		// generate connections
		v.Env().VM.Execute("curl http://www.datadoghq.com")
		dns := v.Env().VM.Execute("dig @8.8.8.8 www.google.fr")
		t.Log(dns)
		//		v.Env().VM.Execute("echo hello > /dev/udp/8.8.8.8/53")

		cnx, err := v.Env().Fakeintake.GetConnections()
		require.NoError(t, err)

		var currentHostname string
		var currentConnection *agentmodel.Connection
		t.Cleanup(func() {
			if t.Failed() {
				t.Logf(krpretty.Sprintf("test failed on host %s at connection %# v", currentHostname, currentConnection))
			}
		})

		foundDNS := false
		for _, hostname := range cnx.GetNames() {
			for _, cc := range cnx.GetPayloadsByName(hostname) {
				for _, c := range cc.Connections {
					if c.Laddr.Ip == "8.8.8.8" || c.Raddr.Ip == "8.8.8.8" {
						foundDNS = true
					}
					/*
						if c.DnsFailedResponses > 0 || c.DnsSuccessfulResponses > 0 {
							foundDNS = true
						}
					*/
				}
			}
		}
		if !foundDNS {
			return errors.New("DNS connection not found")
		}

		nbTotalConnections := 0
		nbTotalTCPConnections := 0
		nbTotalUDPConnections := 0
		hostnames := cnx.GetNames()
		for _, hostname := range hostnames {
			currentHostname = hostname
			nbConnections := 0
			nbTCPConnections := 0
			nbUDPConnections := 0
			collectedConnections := cnx.GetPayloadsByName(hostname)
			for _, cc := range collectedConnections {
				for _, c := range cc.Connections {
					currentConnection = c
					nbConnections++
					switch c.Type {
					case agentmodel.ConnectionType_tcp:
						nbTCPConnections++
					case agentmodel.ConnectionType_udp:
						nbUDPConnections++
					}

					validateConnection(t, c)
				}
			}
			t.Logf("connections %d tcp %d udp %d received by host/netID %s", nbConnections, nbTCPConnections, nbUDPConnections, hostname)
			nbTotalConnections += nbConnections
			nbTotalTCPConnections += nbTCPConnections
			nbTotalUDPConnections += nbUDPConnections
		}
		t.Logf("sum connections %d tcp %d udp %d", nbTotalConnections, nbTotalTCPConnections, nbTotalUDPConnections)
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(time.Second), 60))
	require.NoError(t, err)

}
