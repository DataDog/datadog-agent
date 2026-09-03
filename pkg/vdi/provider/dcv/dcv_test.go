// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package dcv

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	vdimodel "github.com/DataDog/datadog-agent/pkg/vdi/model"
)

func TestParseSessions(t *testing.T) {
	sessions, err := parseSessions([]byte("Session: 'console' (owner:Administrator type:console name:'Console')\r\nSession: graphics-1 (owner:CORP\\user type:virtual name:'Graphics')\r\n"))
	require.NoError(t, err)
	require.Equal(t, []listedSession{
		{id: "console", owner: "Administrator"},
		{id: "graphics-1", owner: `CORP\user`},
	}, sessions)
}

func TestParseSessionsRejectsUnexpectedOutput(t *testing.T) {
	_, err := parseSessions([]byte("not a session"))
	require.ErrorContains(t, err, "unexpected line")

	_, err = parseSessions([]byte("Session: --help (owner:user type:console)"))
	require.ErrorContains(t, err, "option prefix")

	_, err = parseSessions([]byte("Session: bad\x00id (owner:user type:console)"))
	require.ErrorContains(t, err, "control character")

	_, err = parseSessions([]byte("Session: 'unterminated (owner:user type:console)"))
	require.ErrorContains(t, err, "invalid quoted session id")
}

func TestValidateCommandArgs(t *testing.T) {
	require.NoError(t, validateCommandArgs([]string{"list-sessions"}))
	require.NoError(t, validateCommandArgs([]string{"list-connections", "console", "--json"}))

	for _, args := range [][]string{
		{"delete-session", "console"},
		{"list-connections", "--help", "--json"},
		{"list-connections", "console"},
		{"list-connections", "console", "--json", "--extra"},
	} {
		require.Error(t, validateCommandArgs(args))
	}
}

func TestParseConnections(t *testing.T) {
	connections, err := parseConnections([]byte(`[
  {
    "id": 1,
    "username": "test-user@corp.amazonworkspaces.com",
    "user-agent": "DCV Client (2026.0.11738), System: Darwin 24 arm64",
    "client-mode": "classic",
    "client-address": "10.16.93.145:35756",
    "connection-time": "2026-08-11T19:21:14.251957Z",
    "last-interaction-time": "2026-08-11T19:45:57.594810Z",
    "first-frame-time": "2026-08-11T19:21:14.785005Z",
    "transport": "quic",
    "future-field": true
  }
]`))
	require.NoError(t, err)
	require.Len(t, connections, 1)
	require.Equal(t, "1", connections[0].ID)
	require.Equal(t, "test-user@corp.amazonworkspaces.com", connections[0].AuthenticatedUser)
	require.Equal(t, "quic", connections[0].Transport)
	require.Equal(t, "classic", connections[0].ClientMode)
	require.NotNil(t, connections[0].ConnectedAt)
	require.NotNil(t, connections[0].LastInteractionAt)
	require.NotNil(t, connections[0].FirstFrameAt)
}

func TestParseConnectionsRejectsInvalidData(t *testing.T) {
	_, err := parseConnections([]byte(`[{"id":"one"}]`))
	require.Error(t, err)

	_, err = parseConnections([]byte(`[{"id":1,"connection-time":"not-a-time"}]`))
	require.ErrorContains(t, err, "connection-time")

	_, err = parseConnections([]byte(`[] {}`))
	require.ErrorContains(t, err, "trailing JSON value")
}

type fakeRunner struct {
	responses map[string][]byte
	errors    map[string]error
	calls     []string
}

func (r *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	key := strings.Join(args, "|")
	r.calls = append(r.calls, key)
	return r.responses[key], r.errors[key]
}

func TestCollectorUsesOnlyFixedReadOnlyArguments(t *testing.T) {
	runner := &fakeRunner{responses: map[string][]byte{
		"list-sessions":                   []byte("Session: 'console' (owner:Administrator type:console)\n"),
		"list-connections|console|--json": []byte(`[{"id":1,"username":"user"}]`),
	}, errors: map[string]error{}}
	collector := NewCollector(runner)
	collector.now = func() time.Time { return time.Unix(100, 0) }

	result := collector.Collect(context.Background())
	require.Equal(t, vdimodel.StatusOK, result.Status)
	require.Equal(t, []string{"list-sessions", "list-connections|console|--json"}, runner.calls)
	require.Equal(t, "user", result.Sessions[0].Connections[0].AuthenticatedUser)

	// A second request within the short TTL must not execute another command.
	result = collector.Collect(context.Background())
	require.Equal(t, vdimodel.StatusOK, result.Status)
	require.Len(t, runner.calls, 2)
}

func TestCollectorReturnsPartialProviderResults(t *testing.T) {
	runner := &fakeRunner{responses: map[string][]byte{
		"list-sessions":               []byte("Session: one (owner:a type:console)\nSession: two (owner:b type:console)\n"),
		"list-connections|one|--json": []byte(`[{"id":1,"username":"user-one"}]`),
	}, errors: map[string]error{
		"list-connections|two|--json": errors.New("access denied"),
	}}
	result := NewCollector(runner).Collect(context.Background())
	require.Equal(t, vdimodel.StatusPartial, result.Status)
	require.ErrorContains(t, errors.New(result.Error), "access denied")
	require.Len(t, result.Sessions, 2)
	require.Equal(t, "one", result.Sessions[0].ID)
	require.Len(t, result.Sessions[0].Connections, 1)
	require.Equal(t, "two", result.Sessions[1].ID)
	require.Empty(t, result.Sessions[1].Connections)
}
