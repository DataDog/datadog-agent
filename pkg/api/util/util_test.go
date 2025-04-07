// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"path"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestIsIPv6(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedOutput bool
	}{
		{
			name:           "IPv4",
			input:          "192.168.0.1",
			expectedOutput: false,
		},
		{
			name:           "IPv6",
			input:          "2600:1f19:35d4:b900:527a:764f:e391:d369",
			expectedOutput: true,
		},
		{
			name:           "zero compressed IPv6",
			input:          "2600:1f19:35d4:b900::1",
			expectedOutput: true,
		},
		{
			name:           "IPv6 loopback",
			input:          "::1",
			expectedOutput: true,
		},
		{
			name:           "short hostname with only hexadecimal digits",
			input:          "cafe",
			expectedOutput: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, IsIPv6(tt.input), tt.expectedOutput)
		})
	}
}

func reinitGlobalVars() {
	tokenLock.Lock()
	defer tokenLock.Unlock()
	clientTLSConfig = &tls.Config{
		InsecureSkipVerify: true,
	}
	serverTLSConfig = &tls.Config{}
	initSource = uninitialized
}

func TestSuccessfulCreateAndSetAuthToken(t *testing.T) {
	reinitGlobalVars()

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
	err = CreateAndSetAuthToken(mockConfig)
	assert.NoError(t, err)

	// Check that the auth_token content is the same as the one in the file
	authTokenFileContent, err := os.ReadFile(authTokenLocation)
	require.NoError(t, err)
	assert.Equal(t, GetAuthToken(), string(authTokenFileContent))

	// Check that the IPC cert and key have been initialized
	assert.True(t, IsInitialized())

	// Check that the IPC cert and key have been initialized with the correct source
	assert.Equal(t, createAndSetAuthToken, initSource)

	// Check that the IPC cert and key have been initialized with the correct values
	assert.NotNil(t, clientTLSConfig.RootCAs)
	assert.NotNil(t, serverTLSConfig.Certificates)
}

func TestSuccessfulLoadAuthToken(t *testing.T) {
	reinitGlobalVars()

	// Create a new config
	mockConfig := configmock.New(t)
	tmpDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create an auth_token file
	authTokenLocation := path.Join(tmpDir, "auth_token")
	mockConfig.SetWithoutSource("auth_token_file_path", authTokenLocation)

	// Check that CreateAndSetAuthToken returns no error
	err = CreateAndSetAuthToken(mockConfig)
	assert.NoError(t, err)

	// Save currrent state and reinitialize global vars
	createdAuthToken := GetAuthToken()
	createdClientTLSConfig := GetTLSClientConfig()
	createdServerTLSConfig := GetTLSServerConfig()
	reinitGlobalVars()

	// Check that CreateAndSetAuthToken returns no error
	err = CreateAndSetAuthToken(mockConfig)
	assert.NoError(t, err)

	// Check that the auth_token content is the same as the old one
	assert.Equal(t, createdAuthToken, GetAuthToken())
	assert.True(t, createdClientTLSConfig.RootCAs.Equal(GetTLSClientConfig().RootCAs))
	assert.EqualValues(t, createdServerTLSConfig.Certificates, GetTLSServerConfig().Certificates)

	// Reinitialize global vars to check with SetAuthToken
	reinitGlobalVars()

	// Check that SetAuthToken returns no error
	err = SetAuthToken(mockConfig)
	assert.NoError(t, err)

	// Check that the auth_token content is the same as the old one
	assert.Equal(t, createdAuthToken, GetAuthToken())
	assert.True(t, createdClientTLSConfig.RootCAs.Equal(GetTLSClientConfig().RootCAs))
	assert.EqualValues(t, createdServerTLSConfig.Certificates, GetTLSServerConfig().Certificates)
}

// This test check that if CreateAndSetAuthToken blocks, the function timeout
func TestDeadline(t *testing.T) {
	reinitGlobalVars()

	// Create a new config
	mockConfig := configmock.New(t)

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
	err = CreateAndSetAuthToken(mockConfig)
	duration := time.Since(start)
	assert.Error(t, err)
	assert.LessOrEqual(t, duration, mockConfig.GetDuration("auth_init_timeout")+time.Second)
}

func TestStartingServerClientWithUninitializedTLS(t *testing.T) {
	// re initialize the client and server tls config
	initSource = uninitialized
	clientTLSConfig = &tls.Config{
		InsecureSkipVerify: true,
	}

	// create a server with the provided tls server config
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}

	tlsListener := tls.NewListener(l, GetTLSServerConfig())

	go server.Serve(tlsListener) //nolint:errcheck
	defer server.Close()

	// create a http client with the provided tls client config
	_, port, err := net.SplitHostPort(l.Addr().String())
	require.NoError(t, err)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: GetTLSClientConfig(),
		},
	}

	// make a request to the server
	resp, err := client.Get("https://localhost:" + port)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
