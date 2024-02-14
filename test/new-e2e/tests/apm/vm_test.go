// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
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
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
)

type VMFakeintakeSuite struct {
	e2e.BaseSuite[environments.Host]
	transport    transport
	extraLogging bool
}

// NewVMFakeintakeSuite returns a new VMFakeintakeSuite
func NewVMFakeintakeSuite(tr transport) *VMFakeintakeSuite {
	extraLogging := false
	if v, found := os.LookupEnv("EXTRA_LOGGING"); found {
		extraLogging, _ = strconv.ParseBool(v)
	}
	return &VMFakeintakeSuite{
		transport:    tr,
		extraLogging: extraLogging,
	}
}

func vmSuiteOpts(tr transport, opts ...awshost.ProvisionerOption) []e2e.SuiteOption {
	setupScript := `#!/bin/bash
# /var/run/datadog directory is necessary for UDS socket creation
sudo mkdir -p /var/run/datadog
sudo groupadd -r dd-agent
sudo useradd -r -M -g dd-agent dd-agent
sudo chown dd-agent:dd-agent /var/run/datadog

# Agent must be in the docker group to be able to open and read
# container info from the docker socket.
sudo groupadd -f -r docker
sudo usermod -a -G docker dd-agent
`
	opts = append(opts,
		awshost.WithDocker(),
		// Create the /var/run/datadog directory and ensure
		// permissions are correct so the agent can create
		// unix sockets for the UDS transport and communicate with the docker socket.
		awshost.WithEC2InstanceOptions(ec2.WithUserData(setupScript)),
	)

	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(opts...)),
		e2e.WithStackName(fmt.Sprintf("apm-vm-suite-%s-%v", tr, os.Getenv("CI_PIPELINE_ID"))),
	}
	return options
}

// TestVMFakeintakeSuiteUDS runs basic Trace Agent tests over the UDS transport
func TestVMFakeintakeSuiteUDS(t *testing.T) {
	cfg := `
apm_config.enabled: true
apm_config.receiver_socket: /var/run/datadog/apm.socket
`

	options := vmSuiteOpts(uds,
		// Enable the UDS receiver in the trace-agent
		awshost.WithAgentOptions(agentparams.WithAgentConfig(cfg)))
	e2e.Run(t, NewVMFakeintakeSuite(uds), options...)
}

// TestVMFakeintakeSuiteTCP runs basic Trace Agent tests over the TCP transport
func TestVMFakeintakeSuiteTCP(t *testing.T) {
	cfg := `
apm_config.enabled: true
`

	options := vmSuiteOpts(tcp,
		awshost.WithAgentOptions(
			// Enable the UDS receiver in the trace-agent
			agentparams.WithAgentConfig(cfg),
		),
		awshost.WithEC2InstanceOptions(),
	)
	e2e.Run(t, NewVMFakeintakeSuite(tcp), options...)
}

func (s *VMFakeintakeSuite) TestTraceAgentMetrics() {
	// Wait for agent to be live
	s.T().Log("Waiting for Trace Agent to be live.")
	s.Require().NoError(waitRemotePort(s, 8126))

	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)
	s.EventuallyWithTf(func(c *assert.CollectT) {
		s.logStatus()
		testTraceAgentMetrics(s.T(), c, s.Env().FakeIntake)
		s.logJournal()
	}, 3*time.Minute, 10*time.Second, "Failed finding datadog.trace_agent.* metrics")
}

func (s *VMFakeintakeSuite) TestTracesHaveContainerTag() {
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
		s.logStatus()
		testTracesHaveContainerTag(s.T(), c, service, s.Env().FakeIntake)
		s.logJournal()
	}, 3*time.Minute, 10*time.Second, "Failed finding traces with container tags")
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
		s.logStatus()
		testStatsForService(s.T(), c, service, s.Env().FakeIntake)
		s.logJournal()
	}, 3*time.Minute, 10*time.Second, "Failed finding stats")
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
		s.logStatus()
		testBasicTraces(c, service, s.Env().FakeIntake, s.Env().Agent.Client)
		s.logJournal()
	}, 3*time.Minute, 10*time.Second, "Failed to find traces with basic properties")
}

type statusReporter struct {
	v *VMFakeintakeSuite
}

func (r *statusReporter) String() string {
	log, err := r.v.Env().RemoteHost.Execute("sudo journalctl -n1000 -xu datadog-agent-trace")
	if err != nil {
		log = fmt.Sprintf("Failed to run journalctl to get trace agent logs: %v", err)
	}
	status, err := r.v.Env().RemoteHost.Execute("sudo systemctl status datadog-agent-trace")
	if err != nil {
		status = fmt.Sprintf("Failed to run systemctl status to get trace agent status: %v", status)
	}
	return log + "\n" + status
}

func waitRemotePort(v *VMFakeintakeSuite, port uint16) error {
	var (
		c   net.Conn
		err error
	)

	v.Eventually(func() bool {
		v.T().Logf("Waiting for remote:%v", port)
		// TODO: Use the e2e context
		c, err = v.Env().RemoteHost.DialRemotePort(port)
		if err != nil {
			v.T().Logf("Failed to dial remote:%v: %s\n", port, err)
			return false
		}
		v.T().Logf("Connected to remote:%v\n", port)
		defer c.Close()
		return true
	}, 60*time.Second, 1*time.Second, "Failed to dial remote:%v: %s\n%s", port, err, &statusReporter{v})
	return err
}

func (s *VMFakeintakeSuite) logStatus() {
	if !s.extraLogging {
		return
	}
	status, err := s.Env().RemoteHost.Execute("sudo systemctl status datadog-agent-trace")
	if err != nil {
		s.T().Log("cannot log status", err)
		return
	}
	s.T().Log(status)
}

func (s *VMFakeintakeSuite) logJournal() {
	if !s.extraLogging {
		return
	}
	journal, err := s.Env().RemoteHost.Execute("sudo journalctl -n1000 -xu datadog-agent-trace")
	if err != nil {
		s.T().Log("cannot log journal", err)
		return
	}
	s.T().Log(journal)
}
