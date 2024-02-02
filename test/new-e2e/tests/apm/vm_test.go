// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
)

type VMFakeintakeSuite struct {
	e2e.BaseSuite[environments.Host]
	transport transport
}

func vmSuiteOpts(t *testing.T, opts ...awshost.ProvisionerOption) []e2e.SuiteOption {
	if !flag.Parsed() {
		flag.Parse()
	}
	devModeEnv, _ := os.LookupEnv("E2E_DEVMODE")

	opts = append(opts, awshost.WithDocker())

	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(opts...)),
	}
	if devModeE, err := strconv.ParseBool(devModeEnv); (err == nil && devModeE) || *devMode {
		t.Log("Running in Dev Mode.")
		options = append(options, e2e.WithDevMode())
	}
	return options
}

// TestDockerFakeintakeSuiteUDS runs basic Trace Agent tests over the UDS transport
func TestVMFakeintakeSuiteUDS(t *testing.T) {
	cfg := `
apm_config.enabled: true
apm_config.receiver_socket: /var/run/datadog/apm.socket
`

	options := vmSuiteOpts(t, awshost.WithAgentOptions(
		// Enable the UDS receiver in the trace-agent
		agentparams.WithAgentConfig(cfg),
	))
	options = append(options, e2e.WithStackName(fmt.Sprintf("apm-vm-suite-uds-%v", os.Getenv("CI_PIPELINE_ID"))))
	e2e.Run(t, &VMFakeintakeSuite{transport: UDS}, options...)
}

// TestDockerFakeintakeSuiteTCP runs basic Trace Agent tests over the TCP transport
func TestVMFakeintakeSuiteTCP(t *testing.T) {
	cfg := `
apm_config.enabled: true
`

	options := vmSuiteOpts(t,
		awshost.WithAgentOptions(
			// Enable the UDS receiver in the trace-agent
			agentparams.WithAgentConfig(cfg),
		),
		awshost.WithEC2InstanceOptions(),
	)
	options = append(options, e2e.WithStackName(fmt.Sprintf("apm-vm-suite-tcp-%v", os.Getenv("CI_PIPELINE_ID"))))
	e2e.Run(t, &VMFakeintakeSuite{transport: TCP}, options...)
}

func (s *VMFakeintakeSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	h := s.Env().RemoteHost
	// Agent must be in the docker group to be able to open and
	// read container info from the docker socket.
	h.MustExecute("sudo groupadd -f -r docker")
	h.MustExecute("sudo usermod -a -G docker dd-agent")

	// Create the /var/run/datadog directory and ensure
	// permissions are correct so the agent can create
	// unix sockets for the UDS transport
	h.MustExecute("sudo mkdir -p /var/run/datadog")
	h.MustExecute("sudo chown dd-agent:dd-agent /var/run/datadog")

	// Restart the agent
	h.MustExecute("sudo systemctl restart datadog-agent")
}

func (s *VMFakeintakeSuite) TestTraceAgentMetrics() {
	// Wait for agent to be live
	s.T().Log("Waiting for Trace Agent to be live.")
	s.Require().NoError(waitRemotePort(s, 8126))

	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)
	s.EventuallyWithTf(func(c *assert.CollectT) {
		testTraceAgentMetrics(s.T(), c, s.Env().FakeIntake)
	}, 2*time.Minute, 10*time.Second, "Failed finding datadog.trace_agent.* metrics")
}

func (s *VMFakeintakeSuite) TestTracesHaveContainerTag() {
	if s.transport != UDS {
		// TODO: Container tagging with cgroup v2 currently only works over UDS
		// We should update this to run over TCP as well once that is implemented.
		s.T().Skip("Container Tagging with Cgroup v2 only works on UDS")
	}

	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-container-tag-%s", s.transport)

	// Wait for agent to be live
	s.T().Log("Waiting for Trace Agent to be live.")
	s.Require().NoError(waitRemotePort(s, 8126))

	// Run Trace Generator
	s.T().Log("Starting Trace Generator.")
	shutdown := runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})
	defer shutdown()

	s.EventuallyWithTf(func(c *assert.CollectT) {
		testTracesHaveContainerTag(s.T(), c, service, s.Env().FakeIntake)
	}, 2*time.Minute, 10*time.Second, "Failed finding traces with container tags")
}

func (s *VMFakeintakeSuite) TestStatsForService() {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-stats-%s", s.transport)

	// Wait for agent to be live
	s.T().Log("Waiting for Trace Agent to be live.")
	s.Require().NoError(waitRemotePort(s, 8126))

	// Run Trace Generator
	s.T().Log("Starting Trace Generator.")
	shutdown := runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})
	defer shutdown()

	s.EventuallyWithTf(func(c *assert.CollectT) {
		testStatsForService(s.T(), c, service, s.Env().FakeIntake)
	}, 2*time.Minute, 10*time.Second, "Failed finding stats")
}

func (s *VMFakeintakeSuite) TestBasicTrace() {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-basic-trace-%s", s.transport)

	// Wait for agent to be live
	s.T().Log("Waiting for Trace Agent to be live.")
	s.Require().NoError(waitRemotePort(s, 8126))

	// Run Trace Generator
	s.T().Log("Starting Trace Generator.")
	shutdown := runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})
	defer shutdown()

	s.T().Log("Waiting for traces.")
	s.EventuallyWithTf(func(c *assert.CollectT) {
		testBasicTraces(c, service, s.Env().FakeIntake, s.Env().Agent.Client)
	}, 2*time.Minute, 10*time.Second, "Failed to find traces with basic properties")
}

func waitRemotePort(v *VMFakeintakeSuite, port uint16) error {
	var (
		c   net.Conn
		err error
	)
	for i := 0; i < 60; i++ {
		v.T().Logf("Waiting for remote:%v", port)
		c, err = v.Env().RemoteHost.DialRemotePort(port)
		if err != nil {
			v.T().Logf("Failed to dial remote:%v: %s\n", port, err)
			time.Sleep(1 * time.Second)
		} else {
			v.T().Logf("Connected to remote:%v\n", port)
			defer c.Close()
			break
		}
	}
	return err
}
