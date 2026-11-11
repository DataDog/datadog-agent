// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privilegedhelper

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewExecuteRequestKeepsAuthorizationInsideSignedEnvelope(t *testing.T) {
	envelope := SignedEnvelope{
		Data:       []byte("signed task containing command and permissions"),
		HashType:   "SHA256",
		Signatures: []Signature{{KeyType: KeyTypeED25519, KeyID: "key-1", Signature: []byte("signature")}},
	}

	request := NewExecuteRequest(envelope)

	require.Equal(t, ProtocolVersion, request.Version)
	require.Equal(t, envelope, request.Envelope)
	wire, err := json.Marshal(request)
	require.NoError(t, err)
	var outerFields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(wire, &outerFields))
	require.ElementsMatch(t, []string{"version", "envelope"}, mapKeys(outerFields))
}

func mapKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

// TestAgentPolicyWireRoundTripPreservesNilVsEmpty guards the field-level
// nil-vs-empty convention AgentPolicy depends on: omitempty would collapse
// nil and empty slices/maps into the same absent-field wire representation,
// silently turning an explicit per-axis deny-all into "no narrowing" on
// decode. AgentPolicy's fields deliberately omit omitempty to prevent this.
func TestAgentPolicyWireRoundTripPreservesNilVsEmpty(t *testing.T) {
	req := ExecuteRequest{
		Version: ProtocolVersion,
		AgentPolicy: &AgentPolicy{
			AllowedCommands:       []string{"rshell:truncate"},
			AllowedSystemServices: map[string][]string{}, // explicit empty: deny-all for this axis
			// AllowedPaths and ElevatableCommands are left nil: unrestricted by this axis.
		},
	}
	var buf bytes.Buffer
	require.NoError(t, writeMessage(&buf, req))

	var decoded ExecuteRequest
	require.NoError(t, readMessage(&buf, &decoded))

	require.NotNil(t, decoded.AgentPolicy)
	require.Equal(t, []string{"rshell:truncate"}, decoded.AgentPolicy.AllowedCommands)
	require.Nil(t, decoded.AgentPolicy.AllowedPaths, "nil AllowedPaths must survive the wire as nil, not empty")
	require.NotNil(t, decoded.AgentPolicy.AllowedSystemServices, "explicit empty AllowedSystemServices must survive the wire as non-nil")
	require.Empty(t, decoded.AgentPolicy.AllowedSystemServices)
	require.Nil(t, decoded.AgentPolicy.ElevatableCommands)
}

func TestExecuteSignedTaskBuildsVersionedRequest(t *testing.T) {
	// Unix socket paths have a roughly 104-byte limit on macOS. t.TempDir
	// includes the test name and can exceed that limit on GitHub runners.
	tempRoot := ""
	if runtime.GOOS == "darwin" {
		tempRoot = "/tmp"
	}
	dir, err := os.MkdirTemp(tempRoot, "rsh-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	socketPath := filepath.Join(dir, "helper.sock")
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()

	received := make(chan ExecuteRequest, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		var request ExecuteRequest
		if readErr := readMessage(conn, &request); readErr != nil {
			return
		}
		received <- request
		_ = writeMessage(conn, ExecuteResponse{Version: ProtocolVersion, ExitCode: 23})
	}()

	envelope := SignedEnvelope{Data: []byte("signed"), HashType: "SHA256"}
	response, err := (Client{SocketPath: socketPath, Timeout: time.Second}).ExecuteSignedTask(context.Background(), envelope)
	require.NoError(t, err)
	require.Equal(t, 23, response.ExitCode)
	require.Equal(t, NewExecuteRequest(envelope), <-received)
}
