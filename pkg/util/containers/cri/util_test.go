// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cri

package cri

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fakeremote "k8s.io/kubernetes/pkg/kubelet/remote/fake"
)

func TestCRIUtilInit(t *testing.T) {
	fakeRuntime, endpoint := createAndStartFakeRemoteRuntime(t)
	defer fakeRuntime.Stop()
	socketFile := endpoint[7:] // remove unix://
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
	assert.Equal(t, "fakeRuntime", util.Runtime)
	assert.Equal(t, "0.1.0", util.RuntimeVersion)
}

func TestCRIUtilListContainerStats(t *testing.T) {
	fakeRuntime, endpoint := createAndStartFakeRemoteRuntime(t)
	defer fakeRuntime.Stop()
	socketFile := endpoint[7:] // remove unix://
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
