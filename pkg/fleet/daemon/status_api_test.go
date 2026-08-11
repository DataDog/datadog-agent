// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"context"
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

// The transport differs per platform (unix socket / named pipe) but what travels
// over it does not, so these two run against whichever startTestStatusAPI the build
// picked up.

func TestStatusAPIRoundTrip(t *testing.T) {
	diskSpace := uint64(12884901888)
	path := startTestStatusAPI(t, StatusAPIResponse{
		InstallerVersion:   version.AgentVersion,
		AvailableDiskSpace: &diskSpace,
	})

	response, err := newStatusAPIClient(path).Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, version.AgentVersion, response.InstallerVersion)
	require.NotNil(t, response.AvailableDiskSpace)
	assert.Equal(t, diskSpace, *response.AvailableDiskSpace)
}

// A daemon that could not determine the free space must leave the field unset
// rather than report 0, which would read as a full disk.
func TestStatusAPIOmitsUnknownDiskSpace(t *testing.T) {
	path := startTestStatusAPI(t, StatusAPIResponse{InstallerVersion: "7.76.0"})

	response, err := newStatusAPIClient(path).Status(context.Background())
	require.NoError(t, err)
	assert.Nil(t, response.AvailableDiskSpace)
}
