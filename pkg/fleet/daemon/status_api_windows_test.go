// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	"context"
	"fmt"
	"os/user"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/version"
)

type testStatusProvider struct {
	response StatusAPIResponse
}

func (t *testStatusProvider) GetStatus() StatusAPIResponse {
	return t.response
}

// currentUserSecurityDescriptor renders the real DACL template against the test
// process's own SID. Using the current user rather than ddagentuser keeps the test
// independent of which accounts exist on the machine, while still exercising the
// template — a malformed one fails ListenPipe.
func currentUserSecurityDescriptor(t *testing.T) string {
	t.Helper()

	usr, err := user.Current()
	require.NoError(t, err)
	// On Windows user.Current().Uid is the SID string.
	return fmt.Sprintf(statusPipeSecurityDescriptorTemplate, usr.Uid)
}

func startTestStatusAPI(t *testing.T, pipeName string, response StatusAPIResponse) string {
	t.Helper()

	pipePath := `\\.\pipe\` + pipeName
	api, err := newStatusAPIWithSecurityDescriptor(
		&testStatusProvider{response: response},
		pipePath,
		currentUserSecurityDescriptor(t),
	)
	require.NoError(t, err)
	require.NoError(t, api.Start(context.Background()))
	t.Cleanup(func() { _ = api.Stop(context.Background()) })

	return pipePath
}

func TestStatusAPIRoundTrip(t *testing.T) {
	diskSpace := uint64(12884901888)
	pipePath := startTestStatusAPI(t, "DD_INSTALLER_STATUS_TEST_ROUNDTRIP", StatusAPIResponse{
		InstallerVersion:   version.AgentVersion,
		AvailableDiskSpace: &diskSpace,
	})

	response, err := newStatusAPIClient(pipePath).Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, version.AgentVersion, response.InstallerVersion)
	require.NotNil(t, response.AvailableDiskSpace)
	assert.Equal(t, diskSpace, *response.AvailableDiskSpace)
}

// A daemon that could not determine the free space must leave the field unset
// rather than report 0, which would read as a full disk.
func TestStatusAPIOmitsUnknownDiskSpace(t *testing.T) {
	pipePath := startTestStatusAPI(t, "DD_INSTALLER_STATUS_TEST_NODISK", StatusAPIResponse{InstallerVersion: "7.76.0"})

	response, err := newStatusAPIClient(pipePath).Status(context.Background())
	require.NoError(t, err)
	assert.Nil(t, response.AvailableDiskSpace)
}

// The fallback descriptor is what the daemon runs with whenever the ddagentuser SID
// cannot be resolved, so it has to at least be valid SDDL — otherwise the daemon
// fails to start on exactly the hosts where the lookup is broken.
func TestStatusAPIDefaultSecurityDescriptorIsValid(t *testing.T) {
	api, err := newStatusAPIWithSecurityDescriptor(
		&testStatusProvider{},
		`\\.\pipe\DD_INSTALLER_STATUS_TEST_DEFAULTSD`,
		statusPipeDefaultSecurityDescriptor,
	)
	require.NoError(t, err)
	_ = api.Stop(context.Background())
}

// The status pipe must not collide with the privileged local API pipe: they have
// different DACLs and only one of them is safe to expose to the Agent user.
func TestStatusPipeIsNotTheLocalAPIPipe(t *testing.T) {
	assert.NotEqual(t, namedPipePath, statusNamedPipePath)
}
