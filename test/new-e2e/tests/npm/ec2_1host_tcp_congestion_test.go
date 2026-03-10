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

// slowReaderScript is a Python one-liner that listens on port 9999, accepts a connection
// with TCP_WINDOW_CLAMP=1, and never reads. This forces the receiver to advertise
// window=0 almost immediately, triggering zero-window probes from the sender.
// TCP_WINDOW_CLAMP is socket option 10 on Linux.
const slowReaderScript = `import socket,time;` +
	` s=socket.socket();` +
	` s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);` +
	` s.setsockopt(socket.IPPROTO_TCP,10,1);` +
	` s.bind(('',9999));` +
	` s.listen(1);` +
	` c,_=s.accept();` +
	` c.setsockopt(socket.IPPROTO_TCP,10,1);` +
	` time.sleep(300)`

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
			// JMWTODO remove debug log level
			ec2.WithAgentOptions(agentparams.WithAgentConfig("log_level: debug"), agentparams.WithSystemProbeConfig(systemProbeConfigNPM+"\nlog_level: debug")),
			ec2.WithDocker(),
		)
		return ec2.Run(ctx, awsEnv, env, params)
	}
}

// TestEC2TCPCongestionSuite validates TCP congestion signals by inducing various
// network perturbations and verifying the agent reports the corresponding signals.
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
	// Wait for client to have tc and nc
	host.MustExecute("timeout 120 bash -c 'until docker exec tcp-lab-client tc -V >/dev/null 2>&1 && docker exec tcp-lab-client which nc >/dev/null 2>&1; do sleep 2; done'")
}

// BeforeTest cleans up between tests and flushes the fakeintake.
func (v *ec2TCPCongestionSuite) BeforeTest(suiteName, testName string) {
	v.BaseSuite.BeforeTest(suiteName, testName)
	host := v.Env().RemoteHost
	// Kill client traffic generators, server helper processes, and tc rules from previous tests.
	// Do NOT kill iperf3 on server — it's the persistent -s -D listener.
	host.MustExecute("docker exec tcp-lab-client killall -9 iperf3 nc dd 2>/dev/null; " +
		"docker exec tcp-lab-server killall -9 python3 2>/dev/null; " +
		"docker exec tcp-lab-client tc qdisc del dev eth0 root 2>/dev/null; " +
		"docker exec tcp-lab-server tc qdisc del dev eth0 root 2>/dev/null; " +
		"true")
	if !v.BaseSuite.IsDevMode() {
		v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

// AfterTest dumps fakeintake info on failure.
func (v *ec2TCPCongestionSuite) AfterTest(suiteName, testName string) {
	test1HostFakeIntakeNPMDumpInfo(v.T(), v.Env().FakeIntake)
	v.BaseSuite.AfterTest(suiteName, testName)
}

// pollForTCPCongestionSignal polls fakeintake until a TCP connection on the 172.28.0.0/16
// lab network matches the given predicate, or times out after 90 seconds.
func (v *ec2TCPCongestionSuite) pollForTCPCongestionSignal(description string, predicate func(*agentmodel.Connection) bool) {
	t := v.T()
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
			if predicate(conn) {
				t.Logf("%s: %s:%d -> %s:%d retransmits=%d rto=%d recovery=%d probe0=%d ce=%d reord=%d rcvooo=%d ecn=%v",
					description,
					conn.Laddr.Ip, conn.Laddr.Port, conn.Raddr.Ip, conn.Raddr.Port,
					conn.LastRetransmits, conn.LastTcpRtoCount, conn.LastTcpRecoveryCount,
					conn.LastTcpProbe0Count, conn.LastTcpDeliveredCe, conn.LastTcpReordSeen,
					conn.LastTcpRcvOooPack, conn.TcpEcnNegotiated)
				found = true
			}
		})
		assert.True(c, found, "no TCP connection with %s on lab network", description)
	}, 90*time.Second, 2*time.Second, "timed out waiting for %s", description)
}

// TestTCPCongestion_Retransmits applies 5% packet loss and validates LastRetransmits > 0.
func (v *ec2TCPCongestionSuite) TestTCPCongestion_Retransmits() {
	host := v.Env().RemoteHost
	host.MustExecute("docker exec tcp-lab-client tc qdisc add dev eth0 root netem loss 5%")
	v.T().Cleanup(func() {
		host.MustExecute("docker exec tcp-lab-client tc qdisc del dev eth0 root 2>/dev/null || true")
	})
	host.MustExecute("docker exec -d tcp-lab-client iperf3 -c 172.28.0.10 -p 5201 -t 60")

	v.pollForTCPCongestionSignal("LastRetransmits > 0", func(conn *agentmodel.Connection) bool {
		return conn.LastRetransmits > 0
	})
}

// TestTCPCongestion_RTOCount applies heavy correlated packet loss to trigger RTO timeouts.
// Correlated loss creates burst drops that exhaust dupacks and force RTO.
func (v *ec2TCPCongestionSuite) TestTCPCongestion_RTOCount() {
	host := v.Env().RemoteHost
	host.MustExecute("docker exec tcp-lab-client tc qdisc add dev eth0 root netem loss 15% 50%")
	v.T().Cleanup(func() {
		host.MustExecute("docker exec tcp-lab-client tc qdisc del dev eth0 root 2>/dev/null || true")
	})
	host.MustExecute("docker exec -d tcp-lab-client iperf3 -c 172.28.0.10 -p 5201 -t 60")

	v.pollForTCPCongestionSignal("LastTcpRtoCount > 0", func(conn *agentmodel.Connection) bool {
		return conn.LastTcpRtoCount > 0
	})
}

// TestTCPCongestion_RecoveryCount applies moderate correlated loss to trigger
// SACK/NewReno fast recovery via triple duplicate ACKs.
func (v *ec2TCPCongestionSuite) TestTCPCongestion_RecoveryCount() {
	host := v.Env().RemoteHost
	host.MustExecute("docker exec tcp-lab-client tc qdisc add dev eth0 root netem loss 5% 25%")
	v.T().Cleanup(func() {
		host.MustExecute("docker exec tcp-lab-client tc qdisc del dev eth0 root 2>/dev/null || true")
	})
	host.MustExecute("docker exec -d tcp-lab-client iperf3 -c 172.28.0.10 -p 5201 -t 60")

	v.pollForTCPCongestionSignal("LastTcpRecoveryCount > 0", func(conn *agentmodel.Connection) bool {
		return conn.LastTcpRecoveryCount > 0
	})
}

// TestTCPCongestion_ZeroWindowProbes starts a slow reader that accepts a connection with
// TCP_WINDOW_CLAMP=1 and never reads, causing the receiver to advertise window=0.
// The sender then sends zero-window probes which should be counted by probe0_count.
func (v *ec2TCPCongestionSuite) TestTCPCongestion_ZeroWindowProbes() {
	host := v.Env().RemoteHost
	t := v.T()

	// Start slow-reader on server port 9999
	host.MustExecute(fmt.Sprintf(`docker exec -d tcp-lab-server python3 -c "%s"`, slowReaderScript))
	t.Cleanup(func() {
		host.MustExecute("docker exec tcp-lab-server killall -9 python3 2>/dev/null || true")
	})

	// Wait for the slow-reader to be listening
	host.MustExecute("timeout 30 bash -c 'until docker exec tcp-lab-server nc -z localhost 9999 2>/dev/null; do sleep 0.5; done'")

	// Client floods data to the slow reader — send blocks once receive buffer fills
	host.MustExecute("docker exec -d tcp-lab-client bash -c 'dd if=/dev/zero bs=64k count=10000 2>/dev/null | nc 172.28.0.10 9999'")
	t.Cleanup(func() {
		host.MustExecute("docker exec tcp-lab-client killall -9 nc dd 2>/dev/null || true")
	})

	v.pollForTCPCongestionSignal("LastTcpProbe0Count > 0", func(conn *agentmodel.Connection) bool {
		return conn.LastTcpProbe0Count > 0
	})
}

// TestTCPCongestion_Reordering applies packet reordering via tc netem. Reordered packets
// are sent immediately while others are delayed 50ms, causing out-of-order delivery.
func (v *ec2TCPCongestionSuite) TestTCPCongestion_Reordering() {
	host := v.Env().RemoteHost
	host.MustExecute("docker exec tcp-lab-client tc qdisc add dev eth0 root netem delay 50ms reorder 25% 50%")
	v.T().Cleanup(func() {
		host.MustExecute("docker exec tcp-lab-client tc qdisc del dev eth0 root 2>/dev/null || true")
	})
	host.MustExecute("docker exec -d tcp-lab-client iperf3 -c 172.28.0.10 -p 5201 -t 60")

	v.pollForTCPCongestionSignal("LastTcpReordSeen > 0", func(conn *agentmodel.Connection) bool {
		return conn.LastTcpReordSeen > 0
	})
}

// TestTCPCongestion_ECN validates ECN negotiation and CE-marked segment delivery.
// Both containers have tcp_ecn=1 set at startup via docker-compose sysctls.
// The netem ecn flag marks ECN-capable packets with CE instead of dropping them.
func (v *ec2TCPCongestionSuite) TestTCPCongestion_ECN() {
	host := v.Env().RemoteHost
	host.MustExecute("docker exec tcp-lab-client tc qdisc add dev eth0 root netem loss 10% ecn")
	v.T().Cleanup(func() {
		host.MustExecute("docker exec tcp-lab-client tc qdisc del dev eth0 root 2>/dev/null || true")
	})
	host.MustExecute("docker exec -d tcp-lab-client iperf3 -c 172.28.0.10 -p 5201 -t 60")

	v.pollForTCPCongestionSignal("TcpEcnNegotiated", func(conn *agentmodel.Connection) bool {
		return conn.TcpEcnNegotiated
	})
	v.pollForTCPCongestionSignal("LastTcpDeliveredCe > 0", func(conn *agentmodel.Connection) bool {
		return conn.LastTcpDeliveredCe > 0
	})
}

// TestTCPCongestion_RcvOOOPack applies packet reordering on the server's egress so that
// data sent by the server arrives out-of-order at the client. iperf3 reverse mode (-R)
// makes the server the sender, causing the client socket to accumulate rcv_ooopack.
func (v *ec2TCPCongestionSuite) TestTCPCongestion_RcvOOOPack() {
	host := v.Env().RemoteHost
	// Reorder on server egress → data arrives OOO at the client receiver.
	host.MustExecute("docker exec tcp-lab-server tc qdisc add dev eth0 root netem delay 10ms reorder 50% 50%")
	v.T().Cleanup(func() {
		host.MustExecute("docker exec tcp-lab-server tc qdisc del dev eth0 root 2>/dev/null || true")
	})
	// -R: server sends data to client; client accumulates rcv_ooopack on its receiving socket.
	host.MustExecute("docker exec -d tcp-lab-client iperf3 -c 172.28.0.10 -p 5201 -t 60 -R")

	v.pollForTCPCongestionSignal("LastTcpRcvOooPack > 0", func(conn *agentmodel.Connection) bool {
		return conn.LastTcpRcvOooPack > 0
	})
}
