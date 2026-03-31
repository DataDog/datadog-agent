// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogtelextensionimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigValidate_ValidDefaults(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	require.NoError(t, cfg.Validate())
}

func TestConfigValidate_NegativePort(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.TaggerServerPort = -1
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tagger_server_port")
}

func TestConfigValidate_PortTooHigh(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.TaggerServerPort = 65536
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tagger_server_port")
}

func TestConfigValidate_PortZeroAllowed(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.TaggerServerPort = 0
	require.NoError(t, cfg.Validate())
}

func TestConfigValidate_PortMaxAllowed(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.TaggerServerPort = 65535
	require.NoError(t, cfg.Validate())
}

func TestConfigValidate_NegativeMetadataInterval(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.MetadataInterval = -1
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid metadata_interval")
}

func TestConfigValidate_AutoFixMaxMessageSize(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.TaggerMaxMessageSize = 0
	require.NoError(t, cfg.Validate())
	assert.Equal(t, 4*1024*1024, cfg.TaggerMaxMessageSize)
}

func TestConfigValidate_AutoFixConcurrentSync(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.TaggerMaxConcurrentSync = 0
	require.NoError(t, cfg.Validate())
	assert.Equal(t, 5, cfg.TaggerMaxConcurrentSync)
}

func TestCreateDefaultConfig(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	require.NotNil(t, cfg.EnableMetadataCollection)
	assert.True(t, *cfg.EnableMetadataCollection)
	assert.Equal(t, 300, cfg.MetadataInterval)
	assert.False(t, cfg.EnableTaggerServer)
	assert.Equal(t, 0, cfg.TaggerServerPort)
	assert.Equal(t, "localhost", cfg.TaggerServerAddr)
	assert.Equal(t, 4*1024*1024, cfg.TaggerMaxMessageSize)
	assert.Equal(t, 5, cfg.TaggerMaxConcurrentSync)
	assert.False(t, cfg.StandaloneMode)
	// Standalone-mode fields default to zero/nil so they do not
	// override the DD agent config when not explicitly set.
	assert.Equal(t, "", cfg.Hostname)
	assert.Equal(t, "", cfg.SecretBackendCommand)
	assert.Nil(t, cfg.SecretBackendArguments)
	assert.Equal(t, 0, cfg.SecretBackendTimeout)
	assert.Equal(t, 0, cfg.SecretBackendOutputMaxSize)
	assert.Equal(t, "", cfg.KubernetesKubeletHost)
	assert.Nil(t, cfg.KubeletTLSVerify)
	assert.Equal(t, 0, cfg.KubernetesHTTPKubeletPort)
	assert.Equal(t, 0, cfg.KubernetesHTTPSKubeletPort)
}

func TestConfigValidate_StandaloneFields_Valid(t *testing.T) {
	falseVal := false
	cfg := createDefaultConfig().(*Config)
	cfg.Hostname = "my-host"
	cfg.SecretBackendCommand = "/usr/local/bin/secret-provider"
	cfg.SecretBackendArguments = []string{"--arg1"}
	cfg.SecretBackendTimeout = 30
	cfg.SecretBackendOutputMaxSize = 4096
	cfg.KubernetesKubeletHost = "10.0.0.1"
	cfg.KubeletTLSVerify = &falseVal
	cfg.KubernetesHTTPKubeletPort = 10255
	cfg.KubernetesHTTPSKubeletPort = 10250
	require.NoError(t, cfg.Validate())
}

func TestConfigValidate_KubeletTLSVerify_NilIsValid(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.KubeletTLSVerify = nil
	require.NoError(t, cfg.Validate())
}

func TestConfigValidate_KubeletTLSVerify_ExplicitFalse(t *testing.T) {
	falseVal := false
	cfg := createDefaultConfig().(*Config)
	cfg.KubeletTLSVerify = &falseVal
	require.NoError(t, cfg.Validate())
	assert.False(t, *cfg.KubeletTLSVerify)
}
