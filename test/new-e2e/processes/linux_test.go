// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processes

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/clients"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/pkg/sftp"
	"gopkg.in/yaml.v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	agentMajorVersion = 7
	agentDistChannel  = "nightly"
	// Beta is for QA
	//agentDistChannel = "beta"
)

type LinuxTestSuite struct {
	suite.Suite

	sftpClient    *sftp.Client
	ec2           *EC2TestEnv
	ddAPIClient   *datadog.APIClient
	procAPIClient *datadogV2.ProcessesApi
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestLinuxTestSuite(t *testing.T) {
	suite.Run(t, new(LinuxTestSuite))
}

func (s *LinuxTestSuite) SetupSuite() {
	var err error
	s.ec2, err = NewEC2TestEnv("process-agent-linux-test")
	require.NoError(s.T(), err)

	waitForDocker(s.T(), s.ec2.sshClient)

	s.sftpClient, err = sftp.NewClient(s.ec2.sshClient)
	require.NoError(s.T(), err)

	configuration := datadog.NewConfiguration()
	s.ddAPIClient = datadog.NewAPIClient(configuration)
	s.procAPIClient = datadogV2.NewProcessesApi(s.ddAPIClient)
}

func (s *LinuxTestSuite) TearDownSuite() {
	s.ec2.Close()
}

func (s *LinuxTestSuite) TearDownTest() {
	killAndRemoveContainers(s.T(), s.ec2.sshClient)
	s.T().Log("uninstalling the datadog agent")
	stdout, err := clients.ExecuteCommand(s.ec2.sshClient, "sudo apt-get remove --purge datadog-agent -y")
	if err != nil {
		s.T().Logf("error uninstalling the datadog agent. stdout:%s, err: %v", stdout, err)
	}
}

func (s *LinuxTestSuite) TestProcessAgentOnLinux() {
	hostName := createHostName(s.T().Name())
	hostTag := fmt.Sprintf("host:%s", hostName)

	s.T().Logf("start the datadog agent with hostname: %s", hostName)
	// install the agent
	stdout, err := clients.ExecuteCommand(s.ec2.sshClient, fmt.Sprintf(
		"REPO_URL=datad0g.com"+
			" DD_SITE=\"datadoghq.com\""+
			" DD_AGENT_MAJOR_VERSION=%d"+
			" DD_API_KEY=%s"+
			" DD_AGENT_DIST_CHANNEL=%s"+
			" bash -c \"$(curl -L https://s3.amazonaws.com/dd-agent/scripts/install_script.sh)\"",
		agentMajorVersion, s.ec2.ddAPIKey, agentDistChannel),
	)
	s.T().Log(stdout)
	require.NoError(s.T(), err)

	// Setup Process Config
	ddCfg := agentConfig{
		APIKey:      s.ec2.ddAPIKey,
		Hostname:    hostName,
		LogsEnabled: true,
		ProcessCfg: ProcessConfig{
			Enabled: true,
		},
	}

	cfgYaml, err := yaml.Marshal(&ddCfg)
	require.NoError(s.T(), err)
	s.T().Logf("%s", cfgYaml)
	_, err = clients.ExecuteCommand(s.ec2.sshClient, "sudo chmod o+rw /etc/datadog-agent/datadog.yaml")
	require.NoError(s.T(), err)

	file, err := s.sftpClient.OpenFile("/etc/datadog-agent/datadog.yaml", os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	require.NoError(s.T(), err)
	_, err = file.Write(cfgYaml)
	require.NoError(s.T(), err)

	// Setup System Probe Config
	sysProbeCfg := probeConfig{
		SysProbeCfg: SystemProbeConfig{
			Enabled: true,
			ProcCfg: ProcessConfig{
				Enabled: true,
			},
		},
		NetworkCfg: NetworkConfig{
			Enabled: true,
		},
	}

	sysProbeYaml, err := yaml.Marshal(&sysProbeCfg)
	require.NoError(s.T(), err)
	s.T().Logf("%s", sysProbeYaml)
	_, err = clients.ExecuteCommand(s.ec2.sshClient, "sudo touch /etc/datadog-agent/system-probe.yaml")
	require.NoError(s.T(), err)
	_, err = clients.ExecuteCommand(s.ec2.sshClient, "sudo chmod o+rw /etc/datadog-agent/system-probe.yaml")
	require.NoError(s.T(), err)

	file, err = s.sftpClient.OpenFile("/etc/datadog-agent/system-probe.yaml", os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	require.NoError(s.T(), err)
	_, err = file.Write(sysProbeYaml)
	require.NoError(s.T(), err)

	// Restart Process Agent for new agentConfig
	_, err = clients.ExecuteCommand(s.ec2.sshClient, "sudo service datadog-agent restart")
	require.NoError(s.T(), err)

	s.waitForProcessAgent()

	// Start containers
	stdout, err = clients.ExecuteCommand(s.ec2.sshClient,
		"docker run --name spring-hello -d springio/gs-spring-boot-docker")
	s.T().Log(stdout)
	assert.NoError(s.T(), err)

	stdout, err = clients.ExecuteCommand(s.ec2.sshClient, "docker run --name sleepy  -d busybox sleep 9999")
	s.T().Log(stdout)
	assert.NoError(s.T(), err)

	// Check processes are
	ctx := datadog.NewDefaultContext(context.Background())
	assert.Eventually(s.T(), func() bool {
		resp, _, err := s.procAPIClient.ListProcesses(ctx, *datadogV2.NewListProcessesOptionalParameters().
			WithSearch("java").
			WithTags(fmt.Sprintf("%s,container_name:spring-hello", hostTag)).
			WithPageLimit(2),
		)
		if err != nil {
			s.T().Logf("Error when calling `ProcessesApi.ListProcesses`go: %v\n", err)
			return false
		}
		if procs, ok := resp.GetDataOk(); ok && procs != nil {
			return len(*procs) == 1
		}
		return false
	}, 2*time.Minute, 1*time.Second)

	assert.Eventually(s.T(), func() bool {
		resp, _, err := s.procAPIClient.ListProcesses(ctx, *datadogV2.NewListProcessesOptionalParameters().
			WithTags(fmt.Sprintf("%s,container_name:sleepy", hostTag)).
			WithPageLimit(2),
		)
		if err != nil {
			s.T().Logf("Error when calling `ProcessesApi.ListProcesses`: %v\n", err)
			return false
		}
		if procs, ok := resp.GetDataOk(); ok && procs != nil {
			return len(*procs) == 1
		}
		return false
	}, 2*time.Minute, 1*time.Second)
}

func (s *LinuxTestSuite) waitForProcessAgent() {
	var stdout string
	require.Eventuallyf(s.T(), func() bool {
		// Get process log
		stdout, _ = clients.ExecuteCommand(s.ec2.sshClient,
			"grep \"Starting process-agent for\" /var/log/datadog/process-agent.log")
		return strings.Contains(stdout, "Starting process-agent for")
	}, 2*time.Minute, 500*time.Millisecond, "process-agent is not running")

	s.T().Log("process-agent is running")
}

type agentConfig struct {
	APIKey      string        `yaml:"api_key"`
	Hostname    string        `yaml:"hostname"`
	ProcessCfg  ProcessConfig `yaml:"process_config,omitempty"`
	LogsEnabled bool          `yaml:"logs_enabled"`
}

type ProcessConfig struct {
	Enabled bool `yaml:"enabled"`
}

type probeConfig struct {
	NetworkCfg  NetworkConfig     `yaml:"network_config,omitempty"`
	SysProbeCfg SystemProbeConfig `yaml:"system_probe_config,omitempty"`
}

type NetworkConfig struct {
	Enabled bool `yaml:"enabled"`
}

type SystemProbeConfig struct {
	Enabled bool          `yaml:"enabled"`
	ProcCfg ProcessConfig `yaml:"process_config,omitempty"`
}
