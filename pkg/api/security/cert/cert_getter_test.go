// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func initMockConf(t *testing.T) (model.Config, string) {
	testDir := t.TempDir()

	f, err := os.CreateTemp(testDir, "fake-datadog-yaml-")
	require.NoError(t, err)
	t.Cleanup(func() {
		f.Close()
	})

	mockConfig := configmock.New(t)
	mockConfig.SetConfigFile(f.Name())
	mockConfig.SetWithoutSource("auth_token", "")

	return mockConfig, filepath.Join(testDir, "auth_token")
}

func TestCreateOrFetchAuthTokenValidGen(t *testing.T) {
	config, _ := initMockConf(t)
	ipccert, ipckey, err := FetchOrCreateIPCCert(context.Background(), config)
	require.NoError(t, err)

	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(ipccert)
	assert.True(t, ok)

	_, err = tls.X509KeyPair(ipccert, ipckey)
	assert.NoError(t, err)
}

func TestFetchAuthToken(t *testing.T) {
	config, _ := initMockConf(t)

	// Creating a cert
	ipcCert, ipcKey, err := FetchOrCreateIPCCert(context.Background(), config)
	require.NoError(t, err)

	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(ipcCert)
	assert.True(t, ok)

	_, err = tls.X509KeyPair(ipcCert, ipcKey)
	assert.NoError(t, err)

	// Trying to fetch after creating cert: must succeed
	fetchedCert, fetchedKey, err := FetchOrCreateIPCCert(context.Background(), config)
	require.NoError(t, err)
	require.Equal(t, string(ipcCert), string(fetchedCert))
	require.Equal(t, string(ipcKey), string(fetchedKey))
}

func TestFetchAuthTokenWithAuthTokenFilePath(t *testing.T) {
	config, _ := initMockConf(t)

	// Setting custom auth_token filepath
	dname, err := os.MkdirTemp("", "auth_token_dir")
	require.NoError(t, err)
	config.SetWithoutSource("auth_token_file_path", filepath.Join(dname, "auth_token"))

	// Creating a cert
	ipcCert, ipcKey, err := FetchOrCreateIPCCert(context.Background(), config)
	require.NoError(t, err)

	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(ipcCert)
	assert.True(t, ok)

	_, err = tls.X509KeyPair(ipcCert, ipcKey)
	assert.NoError(t, err)

	// Checking that the cert have been created next to the auth_token_file path
	_, err = os.Stat(filepath.Join(dname, defaultCertFileName))
	require.NoError(t, err)

	// Trying to fetch after creating cert: must succeed
	fetchedCert, fetchedKey, err := FetchOrCreateIPCCert(context.Background(), config)
	require.NoError(t, err)
	require.Equal(t, string(ipcCert), string(fetchedCert))
	require.Equal(t, string(ipcKey), string(fetchedKey))
}

func TestFetchAuthTokenWithIPCCertFilePath(t *testing.T) {
	config, _ := initMockConf(t)

	// Setting custom auth_token filepath
	authTokenDirName, err := os.MkdirTemp("", "auth_token_dir")
	require.NoError(t, err)
	config.SetWithoutSource("auth_token_file_path", filepath.Join(authTokenDirName, "custom_auth_token"))

	// Setting custom IPC cert filepath
	ipcDirName, err := os.MkdirTemp("", "ipc_cert_dir")
	require.NoError(t, err)
	config.SetWithoutSource("ipc_cert_file_path", filepath.Join(ipcDirName, "custom_ipc_cert"))

	// Creating a cert
	ipcCert, ipcKey, err := FetchOrCreateIPCCert(context.Background(), config)
	require.NoError(t, err)

	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(ipcCert)
	assert.True(t, ok)

	_, err = tls.X509KeyPair(ipcCert, ipcKey)
	assert.NoError(t, err)

	// Checking that the cert have been created at the custom IPC cert filepath
	_, err = os.Stat(filepath.Join(ipcDirName, "custom_ipc_cert"))
	require.NoError(t, err)

	// Trying to fetch after creating cert: must succeed
	fetchedCert, fetchedKey, err := FetchOrCreateIPCCert(context.Background(), config)
	require.NoError(t, err)
	require.Equal(t, string(ipcCert), string(fetchedCert))
	require.Equal(t, string(ipcKey), string(fetchedKey))
}
