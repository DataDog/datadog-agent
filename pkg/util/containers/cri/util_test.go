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
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/cri-client/pkg/fake"
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

func TestCRIUtilExecSync(t *testing.T) {
	client := &recordingRuntimeServiceClient{
		resp: &criv1.ExecSyncResponse{
			Stdout:   []byte("stdout"),
			Stderr:   []byte("stderr"),
			ExitCode: 7,
		},
	}
	util := &CRIUtil{
		queryTimeout: 1 * time.Second,
		clientV1:     client,
	}

	stdout, stderr, exitCode, err := util.ExecSync(context.Background(), "container-id", []string{"cat", "/etc/redis/redis.conf"}, 3*time.Second)

	require.NoError(t, err)
	assert.Equal(t, []byte("stdout"), stdout)
	assert.Equal(t, []byte("stderr"), stderr)
	assert.Equal(t, int32(7), exitCode)
	require.NotNil(t, client.execSyncReq)
	assert.Equal(t, "container-id", client.execSyncReq.ContainerId)
	assert.Equal(t, []string{"cat", "/etc/redis/redis.conf"}, client.execSyncReq.Cmd)
	assert.Equal(t, int64(3), client.execSyncReq.Timeout)
}

func TestCRIUtilExecSyncReturnsClientError(t *testing.T) {
	expectedErr := errors.New("runtime unavailable")
	util := &CRIUtil{
		queryTimeout: 1 * time.Second,
		clientV1: &recordingRuntimeServiceClient{
			err: expectedErr,
		},
	}

	stdout, stderr, exitCode, err := util.ExecSync(context.Background(), "container-id", []string{"cat", "/etc/redis/redis.conf"}, time.Second)

	require.ErrorIs(t, err, expectedErr)
	assert.Nil(t, stdout)
	assert.Nil(t, stderr)
	assert.Equal(t, int32(0), exitCode)
}

// createAndStartFakeRemoteRuntime creates and starts fakeremote.RemoteRuntime.
// It returns the RemoteRuntime, endpoint on success.
// Users should call fakeRuntime.Stop() to cleanup the server.
func createAndStartFakeRemoteRuntime(t *testing.T) (*fake.RemoteRuntime, string) {
	endpoint, err := fake.GenerateEndpoint()
	require.NoError(t, err)

	fakeRuntime := fake.NewFakeRemoteRuntime()

	err = fakeRuntime.Start(endpoint)
	require.NoError(t, err)

	return fakeRuntime, endpoint
}

type recordingRuntimeServiceClient struct {
	criv1.RuntimeServiceClient

	execSyncReq *criv1.ExecSyncRequest
	resp        *criv1.ExecSyncResponse
	err         error
}

func (c *recordingRuntimeServiceClient) ExecSync(_ context.Context, req *criv1.ExecSyncRequest, _ ...grpc.CallOption) (*criv1.ExecSyncResponse, error) {
	c.execSyncReq = req
	return c.resp, c.err
}
