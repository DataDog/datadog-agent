// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package ipcimpl

import (
	"os"
	"path"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestSuccessfulCreateAndSetAuthToken(t *testing.T) {
	// Create a new config
	mockConfig := configmock.New(t)
	tmpDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create an auth_token file
	authTokenDir := path.Join(tmpDir, "auth_token_dir")
	err = os.Mkdir(authTokenDir, 0700)
	require.NoError(t, err)
	authTokenLocation := path.Join(authTokenDir, "auth_token")
	mockConfig.SetWithoutSource("auth_token_file_path", authTokenLocation)

	// Create an ipc_cert_file
	ipcCertFileLocation := path.Join(tmpDir, "ipc_cert_file")
	mockConfig.SetWithoutSource("ipc_cert_file_path", ipcCertFileLocation)

	// Check that CreateAndSetAuthToken returns no error
	reqs := Requires{
		Log:  logmock.New(t),
		Conf: mockConfig,
	}
	comp, err := NewReadWriteComponent(reqs)
	assert.NoError(t, err)

	// Check that the auth_token content is the same as the one in the file
	authTokenFileContent, err := os.ReadFile(authTokenLocation)
	require.NoError(t, err)
	assert.Equal(t, comp.Comp.GetAuthToken(), string(authTokenFileContent))

	// Check that the IPC cert and key have been initialized with the correct values
	assert.NotNil(t, comp.Comp.GetTLSClientConfig().RootCAs)
	assert.NotNil(t, comp.Comp.GetTLSServerConfig().Certificates)
}

func TestSuccessfulLoadAuthToken(t *testing.T) {
	// Create a new config
	mockConfig := configmock.New(t)
	tmpDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create an auth_token file
	authTokenLocation := path.Join(tmpDir, "auth_token")
	mockConfig.SetWithoutSource("auth_token_file_path", authTokenLocation)

	// Check that CreateAndSetAuthToken returns no error
	reqs := Requires{
		Log:  logmock.New(t),
		Conf: mockConfig,
	}
	RWComp, err := NewReadWriteComponent(reqs)
	assert.NoError(t, err)

	// Check that SetAuthToken returns no error
	ROComp, err := NewReadOnlyComponent(reqs)
	assert.NoError(t, err)

	// Check that the auth_token content is the same as the old one
	assert.Equal(t, RWComp.Comp.GetAuthToken(), ROComp.Comp.GetAuthToken())
	assert.True(t, RWComp.Comp.GetTLSClientConfig().RootCAs.Equal(ROComp.Comp.GetTLSClientConfig().RootCAs))
	assert.EqualValues(t, RWComp.Comp.GetTLSServerConfig().Certificates, ROComp.Comp.GetTLSServerConfig().Certificates)
}

// This test check that if CreateAndSetAuthToken blocks, the function timeout
func TestDeadline(t *testing.T) {
	// Create a new config
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("auth_init_timeout", 1*time.Second)

	// Create a lock file to simulate contention on ipc_cert_file
	tmpDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	ipcCertFileLocation := path.Join(tmpDir, "ipc_cert_file")
	mockConfig.SetWithoutSource("ipc_cert_file_path", ipcCertFileLocation)
	lockFile := flock.New(ipcCertFileLocation + ".lock")
	err = lockFile.Lock()
	require.NoError(t, err)
	defer lockFile.Unlock()
	defer os.Remove(ipcCertFileLocation + ".lock")

	// Check that CreateAndSetAuthToken times out when the auth_token file is locked
	start := time.Now()
	reqs := Requires{
		Log:  logmock.New(t),
		Conf: mockConfig,
	}
	_, err = NewReadWriteComponent(reqs)
	duration := time.Since(start)
	assert.Error(t, err)
	assert.LessOrEqual(t, duration, mockConfig.GetDuration("auth_init_timeout")+time.Second)
}
