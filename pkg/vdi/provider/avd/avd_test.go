// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package avd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/vdi/model"
)

func TestRDPInventoryKeepsDisconnectedSessionsAndExcludesNonDesktopSessions(t *testing.T) {
	logon := time.Unix(100, 0)
	result := FromWindowsInventory(model.WindowsInventory{
		SourceStatus: model.SourceStatus{Status: model.StatusOK},
		Sessions: []model.WindowsSession{
			{WindowsSessionID: 0, Protocol: model.ProtocolRDP, State: "active"},
			{WindowsSessionID: 1, Protocol: "console", User: "admin", State: "active"},
			{WindowsSessionID: 2, Protocol: model.ProtocolRDP, State: "listen"},
			{WindowsSessionID: 3, Protocol: model.ProtocolRDP, User: "alice", Domain: "EXAMPLE", State: "active", LogonAt: &logon},
			{WindowsSessionID: 4, Protocol: model.ProtocolRDP, User: "bob@example.com", Domain: "EXAMPLE", State: "disconnected"},
			{WindowsSessionID: 5, State: "active", User: "unknown-protocol"},
		},
	})
	require.Equal(t, model.StatusPartial, result.Status)
	require.Len(t, result.Sessions, 2)
	require.Equal(t, "3", result.Sessions[0].ID)
	require.Equal(t, "EXAMPLE\\alice", result.Sessions[0].User)
	require.Equal(t, &logon, result.Sessions[0].LogonAt)
	require.Equal(t, "disconnected", result.Sessions[1].State)
	require.Equal(t, "bob@example.com", result.Sessions[1].User)
	for _, session := range result.Sessions {
		require.Equal(t, model.ProtocolRDP, session.Protocol)
	}
}

func TestFailedInventoryDoesNotExposeStaleIdentities(t *testing.T) {
	result := FromWindowsInventory(model.WindowsInventory{
		SourceStatus: model.SourceStatus{Status: model.StatusError, Error: "access denied"},
		Sessions:     []model.WindowsSession{{WindowsSessionID: 3, Protocol: model.ProtocolRDP, User: "stale", State: "active"}},
	})
	require.Equal(t, model.StatusError, result.Status)
	require.Equal(t, "access denied", result.Error)
	require.Empty(t, result.Sessions)
}

func TestMissingUserDoesNotDiscardKnownRDPSession(t *testing.T) {
	result := FromWindowsInventory(model.WindowsInventory{
		SourceStatus: model.SourceStatus{Status: model.StatusPartial, Error: "user query failed"},
		Sessions:     []model.WindowsSession{{WindowsSessionID: 3, Protocol: model.ProtocolRDP, State: "active"}},
	})
	require.Equal(t, model.StatusPartial, result.Status)
	require.Len(t, result.Sessions, 1)
	require.Empty(t, result.Sessions[0].User)
}

func TestDisconnectedDesktopSurvivesMissingClientProtocol(t *testing.T) {
	result := FromWindowsInventory(model.WindowsInventory{
		SourceStatus: model.SourceStatus{Status: model.StatusOK},
		Sessions: []model.WindowsSession{
			{WindowsSessionID: 1, Name: "Console", Protocol: "console", User: "local-admin", State: "disconnected"},
			{WindowsSessionID: 2, Protocol: "console", User: "alice", Domain: "AzureAD", State: "disconnected"},
			{WindowsSessionID: 3, Protocol: model.ProtocolRDP, User: "bob", State: "active"},
			{WindowsSessionID: 4, Name: "Services", Protocol: "console", State: "disconnected"},
		},
	})
	require.Equal(t, model.StatusOK, result.Status)
	require.Len(t, result.Sessions, 2)
	require.Equal(t, "2", result.Sessions[0].ID)
	require.Equal(t, "disconnected", result.Sessions[0].State)
	require.Equal(t, "AzureAD\\alice", result.Sessions[0].User)
	require.Equal(t, "3", result.Sessions[1].ID)
}

func TestDisconnectedDesktopWithUnknownEligibilityMakesInventoryPartial(t *testing.T) {
	for _, protocol := range []string{"", "console"} {
		t.Run("protocol="+protocol, func(t *testing.T) {
			result := FromWindowsInventory(model.WindowsInventory{
				SourceStatus: model.SourceStatus{Status: model.StatusOK},
				Sessions: []model.WindowsSession{
					{WindowsSessionID: 3, Protocol: protocol, State: "disconnected"},
					{WindowsSessionID: 4, Protocol: model.ProtocolRDP, User: "alice", State: "active"},
				},
			})

			require.Equal(t, model.StatusPartial, result.Status)
			require.Contains(t, result.Error, "eligibility could not be determined")
			require.Len(t, result.Sessions, 1)
			require.Equal(t, "4", result.Sessions[0].ID)
		})
	}
}
