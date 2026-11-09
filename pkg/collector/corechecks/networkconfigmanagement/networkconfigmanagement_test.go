// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

package networkconfigmanagement

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	agentconfig "github.com/DataDog/datadog-agent/comp/core/config"
	ncmcomp "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func createTestCheck(t *testing.T) *Check {
	cfg := agentconfig.NewMock(t)
	comp := ncmcomp.Mock(t)
	return newCheck(cfg, comp).(*Check)
}

// Configuration test data

var validConfig = []byte(`
ip_address: 10.0.0.1
profile: p2
auth:
  username: admin
  password: password
  port: "22"
  remote: tcp
`)

var invalidConfigMissingIP = []byte(`
auth:
  username: admin
  password: password
  port: "22"
  remote: tcp
`)

var invalidConfigMissingAuth = []byte(`
ip_address: 10.0.0.1
`)

var baseInitConfig = []byte(`
ssh:
  insecure_skip_verify: true
`)

// Unit Tests

func TestCheck_Configure_ValidConfig(t *testing.T) {
	check := createTestCheck(t)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	err := check.Configure(senderManager, integration.FakeConfigHash, validConfig, baseInitConfig, "test", "provider")

	require.NoError(t, err)
	assert.NotNil(t, check.checkContext)
	assert.Equal(t, "10.0.0.1", check.checkContext.Device.IPAddress)
	assert.Equal(t, "admin", check.checkContext.Device.Auth.Username)
}

func TestCheck_Configure_InvalidConfig(t *testing.T) {
	tests := []struct {
		name          string
		config        []byte
		expectedError string
	}{
		{
			name:          "missing IP address",
			config:        invalidConfigMissingIP,
			expectedError: "ip_address is required",
		},
		{
			name:          "missing auth",
			config:        invalidConfigMissingAuth,
			expectedError: "auth is required",
		},
		{
			name:          "malformed YAML",
			config:        []byte("invalid: yaml: content:"),
			expectedError: "yaml: mapping values are not allowed in this context",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := createTestCheck(t)
			senderManager := mocksender.CreateDefaultDemultiplexer(t)

			err := check.Configure(senderManager, integration.FakeConfigHash, tt.config, baseInitConfig, "test", "provider")

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}

func TestCheck_Run_Success(t *testing.T) {
	check := createTestCheck(t)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	err := check.Configure(senderManager, integration.FakeConfigHash, validConfig, baseInitConfig, "test", "provider")
	require.NoError(t, err)
	err = check.Run()

	assert.NoError(t, err)
}
