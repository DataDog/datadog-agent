// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package vdi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func testCheck() *checkImpl {
	return &checkImpl{config: instanceConfig{
		Provider:      providerAWSWorkSpaces,
		AWSWorkSpaces: &awsWorkSpacesConfig{Product: "personal"},
	}}
}

func TestConnectionAndChannelTags(t *testing.T) {
	check := testCheck()

	connectionTags := check.tagsForInstance("DCV Server Connections", "console:1")
	require.Contains(t, connectionTags, "dcv_session_id:console")
	require.Contains(t, connectionTags, "dcv_connection_id:1")

	channelTags := check.tagsForInstance("DCV Server Channels", "console:1:wsp::wadapter")
	require.Contains(t, channelTags, "dcv_session_id:console")
	require.Contains(t, channelTags, "dcv_connection_id:1")
	require.Contains(t, channelTags, "dcv_channel:wsp::wadapter")
}

func TestSessionAndImagingTags(t *testing.T) {
	check := testCheck()

	sessionTags := check.tagsForInstance("DCV Server Sessions", "console")
	require.Contains(t, sessionTags, "dcv_session_id:console")

	imagingTags := check.tagsForInstance("DCV Server Imaging", "console:nvenc")
	require.Contains(t, imagingTags, "dcv_session_id:console")
	require.Contains(t, imagingTags, "dcv_encoder:nvenc")
}

func TestTotalHasNoIdentityTags(t *testing.T) {
	tags := testCheck().tagsForInstance("DCV Server Connections", "_Total")
	require.ElementsMatch(t, []string{
		"vdi_provider:aws_workspaces",
		"vdi_protocol:dcv",
		"workspaces_product:personal",
	}, tags)
}
