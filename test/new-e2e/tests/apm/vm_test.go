// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"fmt"
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
	opts = append(opts, awshost.WithDocker())
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
	setupScript := `#!/bin/bash
sudo mkdir -p /var/run/datadog
sudo groupadd -r dd-agent
sudo useradd -r -M -g dd-agent dd-agent
sudo chown dd-agent:dd-agent /var/run/datadog`

	options := vmSuiteOpts(uds,
		// Create the /var/run/datadog directory and ensure
		// permissions are correct so the agent can create
		// unix sockets for the UDS transport
		awshost.WithEC2InstanceOptions(ec2.WithUserData(setupScript)),

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

func (s *VMFakeintakeSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	h := s.Env().RemoteHost
	// Agent must be in the docker group to be able to open and
	// read container info from the docker socket.
	h.MustExecute("sudo groupadd -f -r docker")
	h.MustExecute("sudo usermod -a -G docker dd-agent")

	// Restart the agent
	h.MustExecute("sudo systemctl restart datadog-agent")
}

func (s *VMFakeintakeSuite) TestTraceAgentMetrics() {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)
	s.EventuallyWithTf(func(c *assert.CollectT) {
		s.logStatus()
		testTraceAgentMetrics(s.T(), c, s.Env().FakeIntake)
		s.logJournal()
	}, 3*time.Minute, 10*time.Second, "Failed finding datadog.trace_agent.* metrics")
}

func (s *VMFakeintakeSuite) TestTracesHaveContainerTag() {
	if s.transport != uds {
		// TODO: Container tagging with cgroup v2 currently only works over UDS
		// We should update this to run over TCP as well once that is implemented.
		s.T().Skip("Container Tagging with Cgroup v2 only works on UDS")
	}

	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-container-tag-%s", s.transport)

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
