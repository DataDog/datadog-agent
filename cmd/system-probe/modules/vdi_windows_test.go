// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package modules

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/vdi/model"
)

func TestVDIInventoryTimestampFollowsEnumeration(t *testing.T) {
	now := time.Date(2026, time.November, 24, 12, 0, 0, 0, time.UTC)
	lastInput := now.Add(time.Second)
	module := newVDIModule(func() ([]model.WindowsSession, error) {
		now = lastInput
		return []model.WindowsSession{{WindowsSessionID: 4, Protocol: model.ProtocolRDP, State: "active", LastInputAt: &lastInput}}, nil
	})
	module.now = func() time.Time { return now }
	result := module.inventory()

	require.Equal(t, lastInput, result.CollectedAt)
	require.Len(t, result.Providers[model.ProviderAzureVirtualDesktop].Sessions, 1)
	require.Equal(t, &lastInput, result.Providers[model.ProviderAzureVirtualDesktop].Sessions[0].LastInputAt)
}

func TestVDIInventoryIncludesRDPSessions(t *testing.T) {
	module := newVDIModule(func() ([]model.WindowsSession, error) {
		return []model.WindowsSession{{WindowsSessionID: 4, Protocol: model.ProtocolRDP, User: "alice", State: "active"}}, nil
	})
	result := module.inventory()
	require.Equal(t, model.StatusOK, result.Windows.Status)
	require.Len(t, result.Providers[model.ProviderAzureVirtualDesktop].Sessions, 1)
	require.Equal(t, "4", result.Providers[model.ProviderAzureVirtualDesktop].Sessions[0].ID)
}

func TestVDIInventoryReportsWTSFailure(t *testing.T) {
	module := newVDIModule(func() ([]model.WindowsSession, error) { return nil, errors.New("WTS unavailable") })
	result := module.inventory()
	require.Equal(t, model.StatusError, result.Windows.Status)
	require.Equal(t, "WTS unavailable", result.Windows.Error)
	require.Equal(t, model.StatusError, result.Providers[model.ProviderAzureVirtualDesktop].Status)
	require.Empty(t, result.Providers[model.ProviderAzureVirtualDesktop].Sessions)
}
