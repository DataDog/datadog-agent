// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows
// +build linux windows

package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/config"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	mocks "github.com/DataDog/datadog-agent/pkg/proto/pbgo/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

var originalConfig = config.Datadog

func restoreGlobalConfig() {
	config.Datadog = originalConfig
}

func newConfig() {
	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	config.InitConfig(config.Datadog)
	// force timeout to 0s, otherwise each test waits 60s
	config.Datadog.Set(key(ns, "grpc_connection_timeout_secs"), 0)
}

func loadAgentConfigForTest(t *testing.T, path, networksYamlPath string) *AgentConfig {
	config.InitSystemProbeConfig(config.Datadog)

	require.NoError(t, LoadConfigIfExists(path))

	syscfg, err := sysconfig.Merge(networksYamlPath)
	require.NoError(t, err)

	cfg, err := NewAgentConfig("test", path, syscfg)
	require.NoError(t, err)
	return cfg
}

// TestEnvGrpcConnectionTimeoutSecs tests DD_PROCESS_CONFIG_GRPC_CONNECTION_TIMEOUT_SECS.
// This environment variable cannot be tested with the other environment variables because it is overridden.
func TestEnvGrpcConnectionTimeoutSecs(t *testing.T) {
	syscfg, err := sysconfig.Merge("")
	require.NoError(t, err)

	t.Run("DD_PROCESS_CONFIG_GRPC_CONNECTION_TIMEOUT_SECS", func(t *testing.T) {
		t.Setenv("DD_PROCESS_CONFIG_GRPC_CONNECTION_TIMEOUT_SECS", "1")
		_, _ = NewAgentConfig("test", "", syscfg)
		assert.Equal(t, 1, config.Datadog.GetInt("process_config.grpc_connection_timeout_secs"))
	})

	t.Run("DD_PROCESS_AGENT_GRPC_CONNECTION_TIMEOUT_SECS", func(t *testing.T) {
		t.Setenv("DD_PROCESS_AGENT_GRPC_CONNECTION_TIMEOUT_SECS", "2")
		_, _ = NewAgentConfig("test", "", syscfg)
		assert.Equal(t, 2, config.Datadog.GetInt("process_config.grpc_connection_timeout_secs"))
	})
}

func TestYamlConfig(t *testing.T) {
	// Reset the config
	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))

	f, err := os.CreateTemp("", "yamlConfigTest*.yaml")
	defer os.Remove(f.Name())
	assert.NoError(t, err)

	_, err = f.WriteString(`
log_level: debug
log_to_console: false
process_config:
    log_file: /tmp/test
    dd_agent_bin: /tmp/test
    grpc_connection_timeout_secs: 1
    remote_tagger: true
    process_discovery:
        enabled: true
        interval: 1h
`)
	require.NoError(t, err)

	require.NoError(t, LoadConfigIfExists(f.Name()))

	assert.Equal(t, "debug", config.Datadog.GetString("log_level"))
	assert.False(t, config.Datadog.GetBool("log_to_console"))
	assert.Equal(t, "/tmp/test", config.Datadog.GetString("process_config.log_file"))
	assert.Equal(t, "/tmp/test", config.Datadog.GetString("process_config.dd_agent_bin"))
	assert.Equal(t, 1, config.Datadog.GetInt("process_config.grpc_connection_timeout_secs"))
	assert.True(t, config.Datadog.GetBool("process_config.remote_tagger"))
	assert.True(t, config.Datadog.GetBool("process_config.process_discovery.enabled"))
	assert.Equal(t, time.Hour, config.Datadog.GetDuration("process_config.process_discovery.interval"))
}

func TestOnlyEnvConfigLogLevelOverride(t *testing.T) {
	newConfig()
	defer restoreGlobalConfig()

	t.Setenv("DD_LOG_LEVEL", "error")
	t.Setenv("LOG_LEVEL", "debug")

	syscfg, err := sysconfig.Merge("")
	require.NoError(t, err)
	_, _ = NewAgentConfig("test", "", syscfg)
	assert.Equal(t, "error", config.Datadog.GetString("log_level"))
}

func TestGetHostname(t *testing.T) {
	ctx := context.Background()
	_ = NewDefaultAgentConfig()
	h, err := getHostname(ctx, config.Datadog.GetString("process_config.dd_agent_bin"), 0)
	assert.Nil(t, err)
	// verify we fall back to getting os hostname
	expectedHostname, _ := os.Hostname()
	assert.Equal(t, expectedHostname, h)
}

func TestDefaultConfig(t *testing.T) {
	assert := assert.New(t)

	// assert that some sane defaults are set
	assert.Equal("info", config.Datadog.GetString("log_level"))

	t.Setenv("DOCKER_DD_AGENT", "yes")
	_ = NewDefaultAgentConfig()
	assert.Equal(os.Getenv("HOST_PROC"), "")
	assert.Equal(os.Getenv("HOST_SYS"), "")
	t.Setenv("DOCKER_DD_AGENT", "no")
	assert.Equal(config.DefaultProcessExpVarPort, config.Datadog.GetInt("process_config.expvar_port"))

	assert.Equal("info", config.Datadog.GetString("log_level"))
	assert.True(config.Datadog.GetBool("log_to_console"))
	assert.Equal(config.DefaultProcessAgentLogFile, config.Datadog.GetString("process_config.log_file"))
	assert.Equal(config.DefaultDDAgentBin, config.Datadog.GetString("process_config.dd_agent_bin"))
	assert.Equal(config.DefaultGRPCConnectionTimeoutSecs, config.Datadog.GetInt("process_config.grpc_connection_timeout_secs"))
	assert.False(config.Datadog.GetBool("process_config.remote_tagger"))
	assert.False(config.Datadog.GetBool("process_config.remote_workloadmeta"))
	assert.True(config.Datadog.GetBool("process_config.process_discovery.enabled"))
	assert.Equal(4*time.Hour, config.Datadog.GetDuration("process_config.process_discovery.interval"))
}

func TestAgentConfigYamlAndSystemProbeConfig(t *testing.T) {
	newConfig()
	defer restoreGlobalConfig()

	assert := assert.New(t)

	agentConfig := loadAgentConfigForTest(t, "./testdata/TestDDAgentConfigYamlAndSystemProbeConfig.yaml", "")

	assert.Equal("apikey_20", config.Datadog.GetString("api_key"))
	assert.Equal("http://my-process-app.datadoghq.com", config.Datadog.GetString("process_config.process_dd_url"))
	assert.Equal(10, config.Datadog.GetInt("process_config.queue_size"))
	assert.Equal(8*time.Second, agentConfig.CheckIntervals[ContainerCheckName])
	assert.Equal(30*time.Second, agentConfig.CheckIntervals[ProcessCheckName])
	assert.Equal(5065, config.Datadog.GetInt("process_config.expvar_port"))

	newConfig()
	agentConfig = loadAgentConfigForTest(t, "./testdata/TestDDAgentConfigYamlAndSystemProbeConfig.yaml", "./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-Net.yaml")

	assert.Equal("apikey_20", config.Datadog.GetString("api_key"))
	assert.Equal("http://my-process-app.datadoghq.com", config.Datadog.GetString("process_config.process_dd_url"))
	assert.Equal("server-01", agentConfig.HostName)
	assert.Equal(10, config.Datadog.GetInt("process_config.queue_size"))
	assert.Equal(8*time.Second, agentConfig.CheckIntervals[ContainerCheckName])
	assert.Equal(30*time.Second, agentConfig.CheckIntervals[ProcessCheckName])
	if runtime.GOOS != "windows" {
		assert.Equal("/var/my-location/system-probe.log", agentConfig.SystemProbeAddress)
	}

	newConfig()
	agentConfig = loadAgentConfigForTest(t, "./testdata/TestDDAgentConfigYamlAndSystemProbeConfig.yaml", "./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-Net-2.yaml")

	assert.Equal("apikey_20", config.Datadog.GetString("api_key"))
	assert.Equal("http://my-process-app.datadoghq.com", config.Datadog.GetString("process_config.process_dd_url"))
	assert.Equal(10, config.Datadog.GetInt("process_config.queue_size"))
	assert.Equal(8*time.Second, agentConfig.CheckIntervals[ContainerCheckName])
	assert.Equal(30*time.Second, agentConfig.CheckIntervals[ProcessCheckName])

	newConfig()
	agentConfig = loadAgentConfigForTest(t, "./testdata/TestDDAgentConfigYamlAndSystemProbeConfig.yaml", "./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-Net-Windows.yaml")

	if runtime.GOOS == "windows" {
		assert.Equal("localhost:4444", agentConfig.SystemProbeAddress)
	}
}

func TestEnvOrchestratorAdditionalEndpoints(t *testing.T) {
	newConfig()
	defer restoreGlobalConfig()

	assert := assert.New(t)

	expected := make(map[string]string)
	expected["key1"] = "url1.com"
	expected["key2"] = "url2.com"
	expected["key3"] = "url2.com"
	expected["apikey_20"] = "orchestrator.datadoghq.com" // from config file

	t.Setenv("DD_ORCHESTRATOR_ADDITIONAL_ENDPOINTS", `{"https://url1.com": ["key1"], "https://url2.com": ["key2", "key3"]}`)

	agentConfig := loadAgentConfigForTest(t, "./testdata/TestDDAgentConfigYamlAndSystemProbeConfig.yaml", "./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-Net.yaml")

	for _, actual := range agentConfig.Orchestrator.OrchestratorEndpoints {
		assert.Equal(expected[actual.APIKey], actual.Endpoint.Hostname(), actual)
	}
}

func TestGetHostnameFromGRPC(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockAgentClient(ctrl)

	mockClient.EXPECT().GetHostname(
		gomock.Any(),
		&pb.HostnameRequest{},
	).Return(&pb.HostnameReply{Hostname: "unit-test-hostname"}, nil)

	t.Run("hostname returns from grpc", func(t *testing.T) {
		hostname, err := getHostnameFromGRPC(ctx, func(ctx context.Context, opts ...grpc.DialOption) (pb.AgentClient, error) {
			return mockClient, nil
		}, config.DefaultGRPCConnectionTimeoutSecs*time.Second)

		assert.Nil(t, err)
		assert.Equal(t, "unit-test-hostname", hostname)
	})

	t.Run("grpc client is unavailable", func(t *testing.T) {
		grpcErr := errors.New("no grpc client")
		hostname, err := getHostnameFromGRPC(ctx, func(ctx context.Context, opts ...grpc.DialOption) (pb.AgentClient, error) {
			return nil, grpcErr
		}, config.DefaultGRPCConnectionTimeoutSecs*time.Second)

		assert.NotNil(t, err)
		assert.Equal(t, grpcErr, errors.Unwrap(err))
		assert.Empty(t, hostname)
	})
}

func TestGetHostnameFromCmd(t *testing.T) {
	t.Run("valid hostname", func(t *testing.T) {
		h, err := getHostnameFromCmd("agent-success", fakeExecCommand)
		assert.Nil(t, err)
		assert.Equal(t, "unit_test_hostname", h)
	})

	t.Run("no hostname returned", func(t *testing.T) {
		h, err := getHostnameFromCmd("agent-empty_hostname", fakeExecCommand)
		assert.NotNil(t, err)
		assert.Equal(t, "", h)
	})
}

func TestInvalidHostname(t *testing.T) {
	syscfg, err := sysconfig.Merge("")
	require.NoError(t, err)

	// Lower the GRPC timeout, otherwise the test will time out in CI
	config.Datadog.Set("process_config.grpc_connection_timeout_secs", 1)

	// Input yaml file has an invalid hostname (localhost) so we expect to configure via environment
	agentConfig, err := NewAgentConfig("test", "./testdata/TestDDAgentConfigYamlOnly-InvalidHostname.yaml", syscfg)
	require.NoError(t, err)

	expectedHostname, _ := os.Hostname()
	assert.Equal(t, expectedHostname, agentConfig.HostName)
}

// TestGetHostnameShellCmd is a method that is called as a substitute for a dd-agent shell command,
// the GO_TEST_PROCESS flag ensures that if it is called as part of the test suite, it is skipped.
func TestGetHostnameShellCmd(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}

	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "No command\n")
		os.Exit(2)
	}

	cmd, args := args[0], args[1:]
	switch cmd {
	case "agent-success":
		assert.EqualValues(t, []string{"hostname"}, args)
		fmt.Fprintf(os.Stdout, "unit_test_hostname")
	case "agent-empty_hostname":
		assert.EqualValues(t, []string{"hostname"}, args)
		fmt.Fprintf(os.Stdout, "")
	}
}

// TestProcessDiscoveryInterval tests to make sure that the process discovery interval validation works properly
func TestProcessDiscoveryInterval(t *testing.T) {
	for _, tc := range []struct {
		name             string
		interval         time.Duration
		expectedInterval time.Duration
	}{
		{
			name:             "allowed interval",
			interval:         8 * time.Hour,
			expectedInterval: 8 * time.Hour,
		},
		{
			name:             "below minimum",
			interval:         0,
			expectedInterval: discoveryMinInterval,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Mock(t)
			cfg.Set("process_config.process_discovery.interval", tc.interval)

			agentCfg := NewDefaultAgentConfig()
			assert.NoError(t, agentCfg.LoadAgentConfig(""))

			assert.Equal(t, tc.expectedInterval, agentCfg.CheckIntervals[DiscoveryCheckName])
		})
	}
}

// fakeExecCommand is a function that initialises a new exec.Cmd, one which will
// simply call TestShellProcessSuccess rather than the command it is provided. It will
// also pass through the command and its arguments as an argument to TestShellProcessSuccess
func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestGetHostnameShellCmd", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_TEST_PROCESS=1"}
	return cmd
}
