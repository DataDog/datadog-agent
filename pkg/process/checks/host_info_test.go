// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/config"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	pbmocks "github.com/DataDog/datadog-agent/pkg/proto/pbgo/mocks/core"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func TestGetHostname(t *testing.T) {
	cfg := config.Mock(t)
	ctx := context.Background()
	h, err := getHostname(ctx, cfg.GetString("process_config.dd_agent_bin"), 0)
	assert.Nil(t, err)
	// verify we fall back to getting os hostname
	expectedHostname, _ := os.Hostname()
	assert.Equal(t, expectedHostname, h)
}

func TestGetHostnameFromGRPC(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := pbmocks.NewMockAgentClient(ctrl)

	mockClient.EXPECT().GetHostname(
		gomock.Any(),
		&pb.HostnameRequest{},
	).Return(&pb.HostnameReply{Hostname: "unit-test-hostname"}, nil)

	t.Run("hostname returns from grpc", func(t *testing.T) {
		hostname, err := getHostnameFromGRPC(ctx, func(ctx context.Context, address, cmdPort string, opts ...grpc.DialOption) (pb.AgentClient, error) {
			return mockClient, nil
		}, config.DefaultGRPCConnectionTimeoutSecs*time.Second)

		assert.Nil(t, err)
		assert.Equal(t, "unit-test-hostname", hostname)
	})

	t.Run("grpc client is unavailable", func(t *testing.T) {
		grpcErr := errors.New("no grpc client")
		hostname, err := getHostnameFromGRPC(ctx, func(ctx context.Context, address, cmdPort string, opts ...grpc.DialOption) (pb.AgentClient, error) {
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

func TestResolveHostname(t *testing.T) {
	osHostname, err := os.Hostname()
	require.NoError(t, err, "failed to get hostname from OS")

	testCases := []struct {
		name        string
		agentFlavor string
		ddAgentBin  string
		// function to define the host name returned from the core agent
		coreAgentHostname func(context.Context) (string, error)
		// hostname specified in the config
		configHostname   string
		expectedHostname string
	}{
		{
			name:             "valid hostname specified in config",
			agentFlavor:      flavor.ProcessAgent,
			configHostname:   "unit-test-hostname",
			expectedHostname: "unit-test-hostname",
		},
		{
			name:        "invalid hostname and unable to get hostname from core-agent will fallback to os hostname",
			agentFlavor: flavor.ProcessAgent,
			// sets an invalid agent binary to force the fallback to os.Hostname()
			ddAgentBin:       "invalid_agent_binary",
			configHostname:   "localhost",
			expectedHostname: osHostname,
		},
		{
			name:        "running in core agent so use standard hostname lookup",
			agentFlavor: flavor.DefaultAgent,
			coreAgentHostname: func(ctx context.Context) (string, error) {
				return "core-agent-hostname", nil
			},
			expectedHostname: "core-agent-hostname",
		},
		{
			name:        "running in iot agent so use standard hostname lookup",
			agentFlavor: flavor.IotAgent,
			coreAgentHostname: func(ctx context.Context) (string, error) {
				return "iot-agent-hostname", nil
			},
			expectedHostname: "iot-agent-hostname",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			oldFlavor := flavor.GetFlavor()
			defer flavor.SetFlavor(oldFlavor)
			flavor.SetFlavor(tc.agentFlavor)

			cfg := config.Mock(t)
			// Lower the GRPC timeout, otherwise the test will time out in CI
			cfg.SetWithoutSource("process_config.grpc_connection_timeout_secs", 1)

			cfg.SetWithoutSource("hostname", tc.configHostname)

			if tc.ddAgentBin != "" {
				cfg.SetWithoutSource("process_config.dd_agent_bin", tc.ddAgentBin)
			}

			if tc.coreAgentHostname != nil {
				previous := coreAgentGetHostname
				defer func() {
					coreAgentGetHostname = previous
				}()

				coreAgentGetHostname = tc.coreAgentHostname
			}

			hostName, err := resolveHostName(cfg)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedHostname, hostName)
		})
	}
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

// fakeExecCommand is a function that initialises a new exec.Cmd, one which will
// simply call TestShellProcessSuccess rather than the command it is provided. It will
// also pass through the command and its arguments as an argument to TestShellProcessSuccess
func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestGetHostnameShellCmd", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_TEST_PROCESS=1", "DD_LOG_LEVEL=info"} // Set LOG LEVEL to info
	return cmd
}
