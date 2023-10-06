// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cri && !windows

// Note: CRI is supported on Windows. However, these test don't work on Windows
// because the `kubernetes/pkg/kubelet/cri/remote/fake` Windows build only works
// with TCP endpoints, and we don't support them, we just support npipes.

package cri

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fakeremote "github.com/DataDog/datadog-agent/internal/third_party/kubernetes/pkg/kubelet/cri/remote/fake"
)

func TestCRIUtilInit(t *testing.T) {
	fakeRuntime, endpoint := createAndStartFakeRemoteRuntime(t)
	defer fakeRuntime.Stop()
	socketFile := strings.TrimPrefix(endpoint, "unix://")
	fileInfo, err := os.Stat(socketFile)
	require.NoError(t, err)
	assert.Equal(t, fileInfo.Mode()&os.ModeSocket, os.ModeSocket)
	util := &CRIUtil{
		queryTimeout:      1 * time.Second,
		connectionTimeout: 1 * time.Second,
		socketPath:        socketFile,
	}
	err = util.init()
	require.NoError(t, err)
	assert.Equal(t, "fakeRuntime", util.GetRuntime())
	assert.Equal(t, "0.1.0", util.GetRuntimeVersion())
}

func TestCRIUtilListContainerStats(t *testing.T) {
	fakeRuntime, endpoint := createAndStartFakeRemoteRuntime(t)
	defer fakeRuntime.Stop()
	socketFile := strings.TrimPrefix(endpoint, "unix://")
	util := &CRIUtil{
		queryTimeout:      1 * time.Second,
		connectionTimeout: 1 * time.Second,
		socketPath:        socketFile,
	}
	err := util.init()
	require.NoError(t, err)
	_, err = util.ListContainerStats()
	require.NoError(t, err)
}

// createAndStartFakeRemoteRuntime creates and starts fakeremote.RemoteRuntime.
// It returns the RemoteRuntime, endpoint on success.
// Users should call fakeRuntime.Stop() to cleanup the server.
func createAndStartFakeRemoteRuntime(t *testing.T) (*fakeremote.RemoteRuntime, string) {
	endpoint, err := fakeremote.GenerateEndpoint()
	require.NoError(t, err)

	fakeRuntime := fakeremote.NewFakeRemoteRuntime()

	err = fakeRuntime.Start(endpoint)
	require.NoError(t, err)

	return fakeRuntime, endpoint
}
