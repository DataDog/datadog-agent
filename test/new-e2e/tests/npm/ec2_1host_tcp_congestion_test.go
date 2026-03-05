// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"fmt"
	"strings"
	"testing"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

type ec2TCPCongestionSuite struct {
	e2e.BaseSuite[environments.Host]
}

func tcpCongestionEnvProvisioner() provisioners.PulumiEnvRunFunc[environments.Host] {
	return func(ctx *pulumi.Context, env *environments.Host) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}
		params := ec2.GetParams(
			ec2.WithAgentOptions(agentparams.WithSystemProbeConfig(systemProbeConfigNPM)),
			ec2.WithDocker(),
		)
		return ec2.Run(ctx, awsEnv, env, params)
	}
}

// TestEC2TCPCongestionSuite validates TCP congestion signals (retransmits, RTT, etc.)
// by inducing packet loss with tc netem and verifying the agent reports them.
func TestEC2TCPCongestionSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ec2TCPCongestionSuite{}, []e2e.SuiteOption{
		e2e.WithProvisioner(provisioners.NewTypedPulumiProvisioner(
			"tcpCongestion", tcpCongestionEnvProvisioner(), nil,
		)),
	}...)
}

func (v *ec2TCPCongestionSuite) SetupSuite() {
	v.BaseSuite.SetupSuite()
	defer v.CleanupOnSetupFailure()

	host := v.Env().RemoteHost

	// Docker and docker-compose are installed by ec2.WithDocker() in the provisioner.
	// Write compose file and start containers.
	host.MustExecute(fmt.Sprintf("mkdir -p /tmp/tcp-congestion && cat > /tmp/tcp-congestion/docker-compose.yaml << 'EOFCOMPOSE'\n%sEOFCOMPOSE", dockerTCPCongestionComposeYaml))
	host.MustExecute("cd /tmp/tcp-congestion && docker-compose up -d")

	// Wait for iperf3 server ready
	host.MustExecute("timeout 120 bash -c 'until docker exec tcp-lab-server which iperf3 >/dev/null 2>&1 && docker exec tcp-lab-server nc -z localhost 5201 2>/dev/null; do sleep 2; done'")
	// Wait for client to have tc
	host.MustExecute("timeout 120 bash -c 'until docker exec tcp-lab-client tc -V >/dev/null 2>&1; do sleep 2; done'")
}

// BeforeTest flushes the fakeintake aggregators
func (v *ec2TCPCongestionSuite) BeforeTest(suiteName, testName string) {
	v.BaseSuite.BeforeTest(suiteName, testName)
	if !v.BaseSuite.IsDevMode() {
		v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

// AfterTest dumps fakeintake info on failure and cleans up tc rules
func (v *ec2TCPCongestionSuite) AfterTest(suiteName, testName string) {
	test1HostFakeIntakeNPMDumpInfo(v.T(), v.Env().FakeIntake)
	v.Env().RemoteHost.MustExecute("docker exec tcp-lab-client tc qdisc del dev eth0 root 2>/dev/null || true")
	v.BaseSuite.AfterTest(suiteName, testName)
}

// TestTCPCongestion_Retransmits applies 5% packet loss on the client container
// and runs iperf3 traffic, then polls fakeintake for a TCP connection with
// LastRetransmits > 0 on the lab network.
func (v *ec2TCPCongestionSuite) TestTCPCongestion_Retransmits() {
	t := v.T()
	host := v.Env().RemoteHost

	// Apply 5% packet loss on client egress
	host.MustExecute("docker exec tcp-lab-client tc qdisc add dev eth0 root netem loss 5%")
	t.Cleanup(func() {
		host.MustExecute("docker exec tcp-lab-client tc qdisc del dev eth0 root 2>/dev/null || true")
	})

	// Start iperf3 traffic for 60s in the background
	host.MustExecute("docker exec -d tcp-lab-client iperf3 -c 172.28.0.10 -p 5201 -t 60")

	// Poll until a TCP connection with LastRetransmits > 0 appears
	v.EventuallyWithT(func(c *assert.CollectT) {
		cnx, err := v.Env().FakeIntake.Client().GetConnections()
		if !assert.NoError(c, err) || !assert.NotNil(c, cnx) {
			return
		}
		found := false
		cnx.ForeachConnection(func(conn *agentmodel.Connection, _ *agentmodel.CollectorConnections, _ string) {

			if found {
				return
			}
			if conn.Type != agentmodel.ConnectionType_tcp {
				return
			}
			if conn.Laddr == nil || conn.Raddr == nil {
				return
			}
			if !strings.HasPrefix(conn.Laddr.Ip, "172.28.") && !strings.HasPrefix(conn.Raddr.Ip, "172.28.") {
				return
			}
			t.Logf("JMW %s:%d -> %s:%d bytesSent=%d bytesReceived=%d retransmits=%d",
				conn.Laddr.Ip, conn.Laddr.Port, conn.Raddr.Ip, conn.Raddr.Port, conn.LastBytesSent, conn.LastBytesReceived, conn.LastRetransmits)
			if conn.LastRetransmits > 0 {
				t.Logf("retransmits found: %s:%d -> %s:%d retransmits=%d",
					conn.Laddr.Ip, conn.Laddr.Port, conn.Raddr.Ip, conn.Raddr.Port, conn.LastRetransmits)
				found = true
			}
		})
		assert.True(c, found, "no TCP connection with LastRetransmits > 0 on lab network")
	}, 90*time.Second, 2*time.Second, "timed out waiting for retransmits")
}
