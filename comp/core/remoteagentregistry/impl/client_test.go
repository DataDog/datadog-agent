// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestCallAgentsForService(t *testing.T) {
	ipcComp := ipcmock.New(t)
	config := configmock.New(t)

	// Set a short timeout for testing
	config.SetWithoutSource("remote_agent_registry.query_timeout", 1*time.Second)

	// Create agents with different response delays
	fastAgent := buildRemoteAgent(t, ipcComp, "fast-agent", "Fast Agent", "1111",
		withStatusProvider(map[string]string{"status": "fast"}, nil),
	)
	// SlowAgent takes more time to respond
	slowAgent := buildRemoteAgent(t, ipcComp, "slow-agent", "Slow Agent", "2222",
		withStatusProvider(map[string]string{"status": "slow"}, nil),
		withDelay(2*time.Second),
	)
	// SlowAgent takes more time to respond
	wrongSessionIDAgent := buildRemoteAgent(t, ipcComp, "wrong-session-id-agent", "Wrong Session ID Agent", "3333",
		withStatusProvider(map[string]string{"status": "slow"}, nil),
		withFakeSessionID("wrong-session-id"),
	)
	// SlowAgent takes more time to respond
	emptySessionIDAgent := buildRemoteAgent(t, ipcComp, "empty-session-id-agent", "Empty Session ID Agent", "4444",
		withStatusProvider(map[string]string{"status": "slow"}, nil),
		withFakeSessionID(""),
	)
	// Agent without status provider
	agentWithoutStatusProvider := buildRemoteAgent(t, ipcComp, "without-status-provider", "Without Status Provider", "4444")

	deltaTime := 50 * time.Millisecond

	// Test callAgentsForService helper with status provider
	type outStruct struct {
		flavor  string
		errCode codes.Code
	}

	grpcCall := func(ctx context.Context, remoteAgent *remoteAgentClient, opts ...grpc.CallOption) (*pb.GetStatusDetailsResponse, error) {
		return remoteAgent.GetStatusDetails(ctx, &pb.GetStatusDetailsRequest{}, opts...)
	}

	processor := func(details remoteagentregistry.RegisteredAgent, _ *pb.GetStatusDetailsResponse, err error) outStruct {
		// In order to simplify the test, we return a specific error for session_id mismatch
		if err != nil && strings.Contains(err.Error(), "session_id mismatch") {
			return outStruct{
				flavor:  details.Flavor,
				errCode: codes.InvalidArgument,
			}
		}

		e, ok := grpcStatus.FromError(err)
		require.True(t, ok)

		return outStruct{
			flavor:  details.Flavor,
			errCode: e.Code(),
		}
	}

	testCases := []struct {
		name                    string
		remoteAgent             []*testRemoteAgentServer
		expectedCodes           map[string][]codes.Code
		withEphemeralAgent      bool
		shouldSucceedInLessThan time.Duration
	}{
		{
			name:        "1 success",
			remoteAgent: []*testRemoteAgentServer{fastAgent},
			expectedCodes: map[string][]codes.Code{
				"fast-agent": {codes.OK},
			},
			withEphemeralAgent:      false,
			shouldSucceedInLessThan: 200 * time.Millisecond,
		},
		{
			name:        "1 timeout + 1 success",
			remoteAgent: []*testRemoteAgentServer{fastAgent, slowAgent},
			expectedCodes: map[string][]codes.Code{
				"fast-agent": {codes.OK},
				"slow-agent": {codes.DeadlineExceeded},
			},
			withEphemeralAgent:      false,
			shouldSucceedInLessThan: 1 * time.Second,
		},
		{
			name:        "1 success + 1 wrong session ID",
			remoteAgent: []*testRemoteAgentServer{fastAgent, wrongSessionIDAgent},
			expectedCodes: map[string][]codes.Code{
				"fast-agent":             {codes.OK},
				"wrong-session-id-agent": {codes.InvalidArgument},
			},
			withEphemeralAgent:      false,
			shouldSucceedInLessThan: 200 * time.Millisecond,
		},
		{
			name:        "1 success + 1 empty session ID",
			remoteAgent: []*testRemoteAgentServer{fastAgent, emptySessionIDAgent},
			expectedCodes: map[string][]codes.Code{
				"fast-agent":             {codes.OK},
				"empty-session-id-agent": {codes.InvalidArgument},
			},
			withEphemeralAgent:      false,
			shouldSucceedInLessThan: 200 * time.Millisecond,
		},
		{
			name:        "1 success + 1 dead agent",
			remoteAgent: []*testRemoteAgentServer{fastAgent},
			expectedCodes: map[string][]codes.Code{
				"fast-agent": {codes.OK},
				// Both codes are acceptable because the remote agent either returns codes.Unavailable when turning off or codes.DeadlineExceeded when already turned off
				// "dead-agent": {codes.DeadlineExceeded, codes.Unavailable},
				"dead-agent": {codes.DeadlineExceeded, codes.Unavailable},
			},
			withEphemeralAgent:      true,
			shouldSucceedInLessThan: time.Second,
		},
		{
			name:        "1 success + 1 without status provider",
			remoteAgent: []*testRemoteAgentServer{fastAgent, agentWithoutStatusProvider},
			expectedCodes: map[string][]codes.Code{
				"fast-agent": {codes.OK},
			},
			shouldSucceedInLessThan: 200 * time.Millisecond,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Build the component
			provides, _, _, _, _ := buildComponent(t)
			component := provides.Comp.(*remoteAgentRegistry)

			for _, remoteAgent := range testCase.remoteAgent {
				sessionID, _, err := component.RegisterRemoteAgent(&remoteAgent.RegistrationData)
				require.NoError(t, err)
				remoteAgent.registeredSessionID = sessionID
			}

			if testCase.withEphemeralAgent {
				deadAgent := buildAndRegisterRemoteAgent(t, ipcComp, component, "dead-agent", "Dead Agent", "3333",
					withStatusProvider(map[string]string{"status": "dead"}, nil),
				)
				// let the deadAgent start its server
				time.Sleep(500 * time.Millisecond)
				deadAgent.Stop()
			}

			// Test callAgentsForService helper with status provider

			start := time.Now()
			statuses := callAgentsForService(component, StatusServiceName, grpcCall, processor)

			elapsed := time.Since(start)
			require.Less(t, elapsed, testCase.shouldSucceedInLessThan+deltaTime, "Should succeed in less than the expected time")

			require.Equal(t, len(testCase.expectedCodes), len(statuses), "Should have the expected number of statuses")
			for _, status := range statuses {
				require.Contains(t, testCase.expectedCodes[status.flavor], status.errCode)
			}
		})
	}
}

// This test checks that the remote agent service discovery works as expected
func TestRemoteAgentServiceDiscovery(t *testing.T) {
	ipcComp := ipcmock.New(t)
	config := configmock.New(t)

	// Set a short timeout for testing
	config.SetWithoutSource("remote_agent_registry.query_timeout", 1*time.Second)

	// Agent without any services (empty services list)
	agentWithoutServices := buildRemoteAgent(t, ipcComp, "without-services", "Without Services", "1111")
	// Agent without status provider
	agentWithoutStatusProvider := buildRemoteAgent(t, ipcComp, "without-status-provider", "Without Status Provider", "2222")
	// Agent with status provider
	agentWithStatusProvider := buildRemoteAgent(t, ipcComp, "with-status-provider", "With Status Provider", "3333",
		withStatusProvider(map[string]string{"status": "content"}, nil),
	)

	// Build the component
	provides, _, _, _, _ := buildComponent(t)
	component := provides.Comp.(*remoteAgentRegistry)

	testCases := []struct {
		name             string
		agent            *testRemoteAgentServer
		expectedServices []remoteAgentServiceName
		shouldFail       bool
	}{
		{name: "without-services", agent: agentWithoutServices, expectedServices: []remoteAgentServiceName{}, shouldFail: false},
		{name: "without-status-provider", agent: agentWithoutStatusProvider, expectedServices: []remoteAgentServiceName{}, shouldFail: false},
		{name: "with-status-provider", agent: agentWithStatusProvider, expectedServices: []remoteAgentServiceName{StatusServiceName}, shouldFail: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			sessionID, _, err := component.RegisterRemoteAgent(&testCase.agent.RegistrationData)

			if testCase.shouldFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Check that the agent is registered
			component.agentMapMu.Lock()
			remoteAgent, ok := component.agentMap[sessionID]
			require.True(t, ok)
			require.Equal(t, remoteAgent.services, testCase.expectedServices)
			component.agentMapMu.Unlock()
		})
	}
}

func TestSanitizeString(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal case with spaces and mixed case",
			input:    "My Agent Name",
			expected: "my-agent-name",
		},
		{
			name:     "already lowercase with spaces",
			input:    "my agent name",
			expected: "my-agent-name",
		},
		{
			name:     "single word uppercase",
			input:    "Agent",
			expected: "agent",
		},
		{
			name:     "single word lowercase",
			input:    "agent",
			expected: "agent",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "multiple consecutive spaces",
			input:    "My  Agent  Name",
			expected: "my--agent--name",
		},
		{
			name:     "leading and trailing spaces",
			input:    " My Agent Name ",
			expected: "-my-agent-name-",
		},
		{
			name:     "mixed case without spaces",
			input:    "MyAgentName",
			expected: "myagentname",
		},
		{
			name:     "all uppercase with spaces",
			input:    "MY AGENT NAME",
			expected: "my-agent-name",
		},
		{
			name:     "single space",
			input:    " ",
			expected: "-",
		},
		{
			name:     "complex case with numbers and mixed case",
			input:    "Datadog Agent v7.1 Test",
			expected: "datadog-agent-v7.1-test",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := sanitizeString(testCase.input)
			require.Equal(t, testCase.expected, result, "sanitizeString(%q) should return %q, but got %q", testCase.input, testCase.expected, result)
		})
	}
}
