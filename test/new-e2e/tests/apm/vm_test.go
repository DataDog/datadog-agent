// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-shared-components/secretsutils"
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

func vmProvisionerOpts(opts ...awshost.ProvisionerOption) []awshost.ProvisionerOption {
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
	return opts
}

func vmSuiteOpts(tr transport, opts ...awshost.ProvisionerOption) []e2e.SuiteOption {
	opts = vmProvisionerOpts(opts...)
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(opts...)),
		e2e.WithStackName(fmt.Sprintf("apm-vm-suite-%s", tr)),
	}
	return options
}

func vmAgentConfig(tr transport, extra string) string {
	var cfg string
	switch tr {
	case uds:
		cfg = `
apm_config.enabled: true
apm_config.receiver_socket: /var/run/datadog/apm.socket
`
	case tcp:
		cfg = `
apm_config.enabled: true
`
	}
	return cfg + extra
}

// TestVMFakeintakeSuiteUDS runs basic Trace Agent tests over the UDS transport
func TestVMFakeintakeSuiteUDS(t *testing.T) {
	options := vmSuiteOpts(uds,
		// Enable the UDS receiver in the trace-agent
		awshost.WithAgentOptions(agentparams.WithAgentConfig(vmAgentConfig(uds, ""))))
	e2e.Run(t, NewVMFakeintakeSuite(uds), options...)
}

// TestVMFakeintakeSuiteTCP runs basic Trace Agent tests over the TCP transport
func TestVMFakeintakeSuiteTCP(t *testing.T) {
	options := vmSuiteOpts(tcp,
		awshost.WithAgentOptions(
			// Enable the UDS receiver in the trace-agent
			agentparams.WithAgentConfig(vmAgentConfig(tcp, "")),
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
		s.logJournal(false)
	}, 3*time.Minute, 10*time.Second, "Failed finding datadog.trace_agent.* metrics")
}

func (s *VMFakeintakeSuite) TestTraceAgentMetricTags() {
	// Wait for agent to be live
	s.T().Log("Waiting for Trace Agent to be live.")
	s.Require().NoError(waitRemotePort(s, 8126))

	service := fmt.Sprintf("tracegen-metric-tags-%s", s.transport)
	shutdown := runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})
	defer shutdown()

	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)
	s.EventuallyWithTf(func(c *assert.CollectT) {
		s.logStatus()
		testTraceAgentMetricTags(s.T(), c, service, s.Env().FakeIntake)
		s.logJournal(false)
	}, 3*time.Minute, 10*time.Second, "Failed finding datadog.trace_agent.* metrics with tags")
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
	defer waitTracegenShutdown(&s.Suite, s.Env().FakeIntake)
	shutdown := runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})
	defer shutdown()

	s.EventuallyWithTf(func(c *assert.CollectT) {
		s.logStatus()
		testTracesHaveContainerTag(s.T(), c, service, s.Env().FakeIntake)
		s.logJournal(false)
	}, 3*time.Minute, 10*time.Second, "Failed finding traces with container tags")
}

func (s *VMFakeintakeSuite) TestStatsForService() {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-stats-%s", s.transport)
	addSpanTags := "peer.hostname:foo,span.kind:producer"
	expectPeerTag := "peer.hostname:foo"

	// Wait for agent to be live
	s.T().Log("Waiting for Trace Agent to be live.")
	s.Require().NoError(waitRemotePort(s, 8126))

	// Run Trace Generator
	s.T().Log("Starting Trace Generator.")
	defer waitTracegenShutdown(&s.Suite, s.Env().FakeIntake)
	shutdown := runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport, addSpanTags: addSpanTags})
	defer shutdown()

	s.EventuallyWithTf(func(c *assert.CollectT) {
		s.logStatus()
		testStatsForService(s.T(), c, service, expectPeerTag, s.Env().FakeIntake)
		s.logJournal(false)
	}, 3*time.Minute, 10*time.Second, "Failed finding stats")
}

func (s *VMFakeintakeSuite) TestAutoVersionTraces() {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-traces-%s", s.transport)

	// Wait for agent to be live
	s.T().Log("Waiting for Trace Agent to be live.")
	s.Require().NoError(waitRemotePort(s, 8126))

	// Run Trace Generator
	s.T().Log("Starting Trace Generator.")
	defer waitTracegenShutdown(&s.Suite, s.Env().FakeIntake)
	shutdown := runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})
	defer shutdown()

	s.EventuallyWithTf(func(c *assert.CollectT) {
		s.logStatus()
		testAutoVersionTraces(s.T(), c, s.Env().FakeIntake)
		s.logJournal(false)
	}, 3*time.Minute, 10*time.Second, "Failed finding traces")
}

func (s *VMFakeintakeSuite) TestAutoVersionStats() {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-stats-%s", s.transport)

	// Wait for agent to be live
	s.T().Log("Waiting for Trace Agent to be live.")
	s.Require().NoError(waitRemotePort(s, 8126))

	// Run Trace Generator
	s.T().Log("Starting Trace Generator.")
	defer waitTracegenShutdown(&s.Suite, s.Env().FakeIntake)
	shutdown := runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})
	defer shutdown()

	s.EventuallyWithTf(func(c *assert.CollectT) {
		s.logStatus()
		testAutoVersionStats(s.T(), c, s.Env().FakeIntake)
		s.logJournal(false)
	}, 3*time.Minute, 10*time.Second, "Failed finding stats")
}

func (s *VMFakeintakeSuite) TestIsTraceRootTag() {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-stats-%s", s.transport)

	// Wait for agent to be live
	s.T().Log("Waiting for Trace Agent to be live.")
	s.Require().NoError(waitRemotePort(s, 8126))

	// Run Trace Generator
	s.T().Log("Starting Trace Generator.")
	defer waitTracegenShutdown(&s.Suite, s.Env().FakeIntake)
	shutdown := runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})
	defer shutdown()

	s.EventuallyWithTf(func(c *assert.CollectT) {
		s.logStatus()
		testIsTraceRootTag(s.T(), c, s.Env().FakeIntake)
		s.logJournal(false)
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
	defer waitTracegenShutdown(&s.Suite, s.Env().FakeIntake)
	shutdown := runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})
	defer shutdown()

	s.T().Log("Waiting for traces.")
	s.EventuallyWithTf(func(c *assert.CollectT) {
		s.logStatus()
		testBasicTraces(c, service, s.Env().FakeIntake, s.Env().Agent.Client)
		s.logJournal(false)
	}, 3*time.Minute, 10*time.Second, "Failed to find traces with basic properties")
}

func (s *VMFakeintakeSuite) TestProbabilitySampler() {
	s.UpdateEnv(awshost.Provisioner(vmProvisionerOpts(awshost.WithAgentOptions(agentparams.WithAgentConfig(vmAgentConfig(s.transport, `
apm_config.probabilistic_sampler.enabled: true
apm_config.probabilistic_sampler.sampling_percentage: 50
apm_config.probabilistic_sampler.hash_seed: 22
`))))...))

	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-probability-sampler-%s", s.transport)

	// Wait for agent to be live
	s.T().Log("Waiting for Trace Agent to be live.")
	s.Require().NoError(waitRemotePort(s, 8126))

	// Run Trace Generator
	s.T().Log("Starting Trace Generator.")
	defer waitTracegenShutdown(&s.Suite, s.Env().FakeIntake)
	shutdown := runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})
	defer shutdown()

	s.T().Log("Waiting for traces.")
	s.EventuallyWithTf(func(c *assert.CollectT) {
		tracesSampledByProbabilitySampler(s.T(), c, s.Env().FakeIntake)
	}, 2*time.Minute, 10*time.Second, "Failed to find traces sampled by the probability sampler")
}

func (s *VMFakeintakeSuite) TestSIGTERM() {
	output := s.Env().RemoteHost.MustExecute("cat /opt/datadog-agent/run/trace-agent.pid")
	pid, err := strconv.ParseInt(strings.TrimSpace(output), 10, 64)
	s.Require().NoError(err, "failed to parse trace-agent pid")

	start := time.Now()
	_, err = s.Env().RemoteHost.Execute(fmt.Sprintf("sudo kill -SIGTERM %d", pid))
	s.Require().NoError(err, "failed to send SIGTERM to trace-agent")
	s.EventuallyWithTf(func(c *assert.CollectT) {
		_, err := s.Env().RemoteHost.Execute("pgrep -x trace-agent")
		if err == nil {
			c.Errorf("trace-agent should not be running")
			return
		}

		// pgrep exits with 1 if no process is found
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitStatus() == 1 {
			end := time.Now()
			s.T().Logf("trace-agent exited after %s", end.Sub(start).String())
			return
		}
		assert.NoError(c, err, "failed to check the trace-agent process state")
	}, 30*time.Second, 1*time.Second, "failed to stop trace-agent service in 30 seconds")

	// make sure the trace-agent is running after this test
	s.Env().RemoteHost.MustExecute("sudo systemctl start datadog-agent-trace.service")
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
		c, err = v.Env().RemoteHost.DialPort(port)
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

func (s *VMFakeintakeSuite) logJournal(force bool) {
	if !s.extraLogging && !force {
		return
	}
	journal, err := s.Env().RemoteHost.Execute("sudo journalctl -n1000 -xu datadog-agent-trace")
	if err != nil {
		s.T().Log("cannot log journal", err)
		return
	}
	s.T().Log(journal)
}

func (s *VMFakeintakeSuite) TestAPIKeyRefresh() {
	apiKey1 := strings.Repeat("1", 32)
	apiKey2 := strings.Repeat("2", 32)

	rootDir := "/tmp/" + s.T().Name()
	s.Env().RemoteHost.MkdirAll(rootDir)

	secretResolverPath := filepath.Join(rootDir, "secret-resolver.py")

	s.T().Log("Setting up the secret resolver and the initial api key file")

	secretClient := secretsutils.NewSecretClient(s.T(), s.Env().RemoteHost, rootDir)
	secretClient.SetSecret("api_key", apiKey1)

	extraconfig := fmt.Sprintf(`
api_key: ENC[api_key]
log_level: debug

secret_backend_command: %s
secret_backend_arguments:
  - %s
secret_backend_remove_trailing_line_break: true
secret_backend_command_allow_group_exec_perm: true

agent_ipc:
  port: 5004
  config_refresh_interval: 5
`, secretResolverPath, rootDir)

	s.UpdateEnv(awshost.Provisioner(
		vmProvisionerOpts(
			awshost.WithAgentOptions(
				agentparams.WithAgentConfig(vmAgentConfig(s.transport, extraconfig)),
				secretsutils.WithUnixSecretSetupScript(secretResolverPath, true),
				agentparams.WithSkipAPIKeyInConfig(), // api_key is already provided in the config
			),
		)...),
	)

	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-apikey-refresh-%s", s.transport)

	// Run Trace Generator
	s.T().Log("Starting Trace Generator.")
	defer waitTracegenShutdown(&s.Suite, s.Env().FakeIntake)
	shutdown := runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})
	defer shutdown()

	s.T().Log("Waiting for traces (apiKey1)")
	s.EventuallyWithTf(func(c *assert.CollectT) {
		testBasicTraces(c, service, s.Env().FakeIntake, s.Env().Agent.Client)
	}, 2*time.Minute, 10*time.Second, "Failed to find traces with basic properties")

	// update api_key
	s.T().Log("Updating the api key")
	secretClient.SetSecret("api_key", apiKey2)

	// trigger a refresh of the core-agent secrets
	s.T().Log("Refreshing core-agent secrets")
	secretRefreshOutput := s.Env().Agent.Client.Secret(agentclient.WithArgs([]string{"refresh"}))
	// ensure the api_key was refreshed, fail directly otherwise
	require.Contains(s.T(), secretRefreshOutput, "api_key")

	// wait enough time for API Key refresh on trace-agent
	time.Sleep(15 * time.Second)

	err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	s.T().Log("Waiting for traces (apiKey2)")
	s.EventuallyWithTf(func(c *assert.CollectT) {
		testBasicTraces(c, service, s.Env().FakeIntake, s.Env().Agent.Client)
	}, 2*time.Minute, 10*time.Second, "Failed to find traces with basic properties")

	s.logJournal(true)
}
