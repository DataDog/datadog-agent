// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package backend holds files related to forwarder backends for security profiles
package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// hasMRFEndpoint reports whether any endpoint is a Multi-Region Failover endpoint.
func hasMRFEndpoint(endpoints *logsconfig.Endpoints) bool {
	for _, endpoint := range endpoints.Endpoints {
		if endpoint.IsMRF {
			return true
		}
	}
	return false
}

// TestActivityDumpRemoteStorageEndpointsDropsMRF ensures activity dump endpoints never
// include a Multi-Region Failover endpoint. The activity dump backend posts to every
// endpoint unconditionally and there is no MRF intake for the secdump API, so a
// synthesized MRF endpoint (e.g. cws-intake.logs.mrf.<site>) would 404 on every dump.
func TestActivityDumpRemoteStorageEndpointsDropsMRF(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("multi_region_failover.enabled", true)
	cfg.SetInTest("multi_region_failover.site", "datadoghq.eu")
	cfg.SetInTest("multi_region_failover.api_key", "0123456789abcdef0123456789abcdef")

	endpoints, err := activityDumpRemoteStorageEndpoints("cws-intake.", "secdump", logsconfig.DefaultIntakeProtocol, "cloud-workload-security")
	require.NoError(t, err)
	require.NotNil(t, endpoints)

	assert.NotEmpty(t, endpoints.Endpoints, "the main endpoint must be preserved")
	assert.False(t, hasMRFEndpoint(endpoints), "MRF endpoints must be filtered out of activity dump endpoints")
}

// TestActivityDumpRemoteStorageEndpointsWithoutMRF ensures the filtering does not disturb
// the normal (MRF-disabled) path.
func TestActivityDumpRemoteStorageEndpointsWithoutMRF(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("multi_region_failover.enabled", false)

	endpoints, err := activityDumpRemoteStorageEndpoints("cws-intake.", "secdump", logsconfig.DefaultIntakeProtocol, "cloud-workload-security")
	require.NoError(t, err)
	require.NotNil(t, endpoints)

	assert.NotEmpty(t, endpoints.Endpoints, "the main endpoint must be present")
	assert.False(t, hasMRFEndpoint(endpoints))
}
