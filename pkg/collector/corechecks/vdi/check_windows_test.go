// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package vdi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	vdimodel "github.com/DataDog/datadog-agent/pkg/vdi/model"
)

func testCheck() *checkImpl {
	return &checkImpl{config: instanceConfig{
		Provider:      vdimodel.ProviderAWSWorkSpaces,
		AWSWorkSpaces: &awsWorkSpacesConfig{Product: "personal"},
	}}
}

func TestConnectionAndExtensionChannelTags(t *testing.T) {
	check := testCheck()
	connections := map[connectionKey]vdimodel.Connection{
		{sessionID: "console", connectionID: "1"}: {
			ID:                "1",
			AuthenticatedUser: "test-user@corp.amazonworkspaces.com",
			Transport:         "quic",
			ClientMode:        "classic",
			UserAgent:         "DCV Client (2026.0.11738), System: Darwin 24 arm64",
		},
	}

	connectionTags, key := check.tagsForInstance("DCV Server Connections", "console:1", connections, nil)
	require.Equal(t, &connectionKey{sessionID: "console", connectionID: "1"}, key)
	require.Subset(t, connectionTags, []string{
		"vdi_session_id:console",
		"vdi_connection_id:1",
		"vdi_connection_user:test-user@corp.amazonworkspaces.com",
		"dcv_transport:quic",
		"dcv_client_mode:classic",
		"dcv_client_version:2026.0.11738",
		"dcv_client_os:darwin",
		"dcv_client_arch:arm64",
	})

	channelTags, _ := check.tagsForInstance("DCV Server Channels", "console:1:wsp::wadapter", connections, nil)
	require.Contains(t, channelTags, "vdi_connection_user:test-user@corp.amazonworkspaces.com")
	require.Contains(t, channelTags, "dcv_channel:wsp::wadapter")
}

func TestMultipleConnectionsMapIndependently(t *testing.T) {
	check := testCheck()
	connections := map[connectionKey]vdimodel.Connection{
		{sessionID: "console", connectionID: "1"}: {AuthenticatedUser: "user-one"},
		{sessionID: "console", connectionID: "2"}: {AuthenticatedUser: "user-two"},
	}

	first, _ := check.tagsForInstance("DCV Server Connections", "console:1", connections, nil)
	second, _ := check.tagsForInstance("DCV Server Connections", "console:2", connections, nil)
	require.Contains(t, first, "vdi_connection_user:user-one")
	require.NotContains(t, first, "vdi_connection_user:user-two")
	require.Contains(t, second, "vdi_connection_user:user-two")
	require.NotContains(t, second, "vdi_connection_user:user-one")
}

func TestSessionMetricsNeverInheritConnectionUser(t *testing.T) {
	check := testCheck()
	connections := map[connectionKey]vdimodel.Connection{
		{sessionID: "console", connectionID: "1"}: {AuthenticatedUser: "user"},
		{sessionID: "console", connectionID: "2"}: {AuthenticatedUser: "user"},
	}
	tags, _ := check.tagsForInstance("DCV Server Sessions", "console", connections, nil)
	require.Contains(t, tags, "vdi_session_id:console")
	require.NotContains(t, tags, "vdi_connection_user:user")
}

func TestImagingIdentityUsesUnambiguousSession(t *testing.T) {
	check := testCheck()
	tags, _ := check.tagsForInstance("DCV Server Imaging", "console:nvenc", nil, map[string]struct{}{"console": {}})
	require.Contains(t, tags, "vdi_session_id:console")
	require.Contains(t, tags, "dcv_encoder:nvenc")
	require.NotContains(t, tags, "vdi_connection_user:user")
}

func TestUnavailableInventoryNeverIndexesStaleIdentity(t *testing.T) {
	provider := vdimodel.ProviderInventory{Sessions: []vdimodel.Session{{
		ID: "console",
		Connections: []vdimodel.Connection{{
			ID:                "1",
			AuthenticatedUser: "stale-user",
		}},
	}}}
	require.Empty(t, indexConnections(provider, false))
}

func TestInventoryStaleTTLCanBeImmediate(t *testing.T) {
	zero := 0
	check := testCheck()
	check.config.InventoryStaleTTL = &zero
	check.startedAt = time.Unix(100, 0)
	require.Equal(t, servicecheck.ServiceCheckCritical, check.unavailableInventoryStatus(time.Unix(101, 0)))
}

func TestTotalInstanceFilter(t *testing.T) {
	require.False(t, isNotTotal("_Total"))
	require.False(t, isNotTotal("_total"))
	require.True(t, isNotTotal("console"))
	require.True(t, isNotTotal("console:1"))
}

func TestUserAgentParsing(t *testing.T) {
	version, osName, arch := parseUserAgent("DCV Client (2026.0.11738), System: Darwin (...) arm64")
	require.Equal(t, "2026.0.11738", version)
	require.Equal(t, "darwin", osName)
	require.Equal(t, "arm64", arch)

	version, osName, arch = parseUserAgent("unknown")
	require.Empty(t, version)
	require.Empty(t, osName)
	require.Empty(t, arch)
}
