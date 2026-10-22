// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	mock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestConcurrencyPolicyFromConfig(t *testing.T) {
	mockConfig := mock.New(t)
	mockConfig.SetInTest("forwarder_num_workers", 3)
	mockConfig.SetInTest("forwarder_max_concurrent_requests", 5)

	p := concurrencyPolicyFromConfig(mockConfig, logmock.New(t))
	require.IsType(t, staticConcurrencyPolicy{}, p)
	assert.Equal(t, 3, p.numWorkers())
	assert.Equal(t, 3, p.maxConns())
	assert.Equal(t, int64(5), p.(staticConcurrencyPolicy).requests)

	d := p.newDomain("remote")
	assert.Equal(t, 3, d.numWorkers())
	assert.Equal(t, 3, d.maxConns())
	assert.NotSame(t, d.newRequestSemaphore(), d.newRequestSemaphore())
}

func TestConcurrencyPolicyFromConfigHTTP1MaxConnections(t *testing.T) {
	for _, tc := range []struct {
		name           string
		protocol       string
		maxConnections int
		workers        int
		requests       int
		want           int
	}{
		{"http1 unset keeps one connection per worker", "http1", 0, 3, 5, 3},
		{"http1 negative keeps one connection per worker", "http1", -1, 3, 5, 3},
		{"http1 opt-in", "http1", 4, 1, 10, 4},
		{"http1 opt-in with several workers is per domain", "http1", 4, 3, 5, 4},
		{"http1 below workers is raised to workers", "http1", 2, 3, 5, 3},
		{"http1 above workers x requests is lowered", "http1", 50, 3, 5, 15},
		{"auto ignores it", "auto", 4, 3, 5, 3},
		{"invalid protocol ignores it", "h2c", 4, 3, 5, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := mock.New(t)
			mockConfig.SetInTest("forwarder_http_protocol", tc.protocol)
			mockConfig.SetInTest("forwarder_num_workers", tc.workers)
			mockConfig.SetInTest("forwarder_max_concurrent_requests", tc.requests)
			if tc.maxConnections != 0 {
				mockConfig.SetInTest("forwarder_http1_max_connections", tc.maxConnections)
			}

			p := concurrencyPolicyFromConfig(mockConfig, logmock.New(t))
			assert.Equal(t, tc.workers, p.numWorkers())
			assert.Equal(t, tc.want, p.maxConns())
			assert.Equal(t, tc.want, p.newDomain("remote").maxConns())

			local := newLocalConcurrencyDomainFromConfig(mockConfig, logmock.New(t))
			assert.Equal(t, 1, local.maxConns())
		})
	}
}

func TestHTTP1MaxConnectionsDefault(t *testing.T) {
	mockConfig := mock.New(t)
	assert.Equal(t, 0, mockConfig.GetInt("forwarder_http1_max_connections"))
}

func TestNewLocalConcurrencyDomainFromConfig(t *testing.T) {
	mockConfig := mock.New(t)
	mockConfig.SetInTest("forwarder_num_workers", 3)
	mockConfig.SetInTest("forwarder_max_concurrent_requests", 5)

	d := newLocalConcurrencyDomainFromConfig(mockConfig, logmock.New(t))
	assert.Equal(t, 1, d.numWorkers())
	assert.Equal(t, 1, d.maxConns())
	assert.Equal(t, int64(5), d.(staticConcurrencyPolicy).requests)
}

func TestConcurrencyPolicyFromConfigInvalidRequests(t *testing.T) {
	mockConfig := mock.New(t)
	mockConfig.SetInTest("forwarder_max_concurrent_requests", 0)

	p := concurrencyPolicyFromConfig(mockConfig, logmock.New(t))
	assert.Equal(t, int64(1), p.(staticConcurrencyPolicy).requests)
}

func newTestConcurrencyDomain(workers int, requests int64) concurrencyDomain {
	return staticConcurrencyPolicy{
		workers:     workers,
		connections: workers,
		requests:    requests,
	}.newDomain("test")
}
