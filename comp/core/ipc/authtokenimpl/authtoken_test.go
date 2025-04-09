// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package ipcimpl

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/ipc"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestBothModes(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth_token")
	ipcPath := filepath.Join(dir, "ipc_cert")
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("auth_token_file_path", authPath)
	mockConfig.SetWithoutSource("ipc_cert_file_path", ipcPath)

	reqs := dependencies{
		Log:    logmock.New(t),
		Conf:   mockConfig,
		Params: ipc.ForOneShot(),
	}
	provides := newOptionalIPC(reqs)
	_, ok := provides.Get()
	require.False(t, ok)

	// Simulate a daemon created the auth artifact
	{
		daemonReqs := reqs
		daemonReqs.Params = ipc.ForDaemon()
		provides := newOptionalIPC(daemonReqs)
		comp, ok := provides.Get()
		require.True(t, ok)

		// Check that the auth token is set
		assert.Equal(t, util.GetAuthToken(), comp.Get())

		// Check that the IPC certificate is set
		assert.Equal(t, util.GetTLSClientConfig(), comp.GetTLSClientConfig())
		assert.Equal(t, util.GetTLSServerConfig(), comp.GetTLSServerConfig())
	}

	// re-create the component
	provides = newOptionalIPC(reqs)
	comp, ok := provides.Get()
	require.True(t, ok)

	// Check that the auth token is set
	assert.Equal(t, util.GetAuthToken(), comp.Get())

	// Check that the IPC certificate is set
	assert.Equal(t, util.GetTLSClientConfig(), comp.GetTLSClientConfig())
	assert.Equal(t, util.GetTLSServerConfig(), comp.GetTLSServerConfig())
}
