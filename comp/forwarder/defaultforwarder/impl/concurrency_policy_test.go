// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
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

func newTestConcurrencyDomainFromConfig(config config.Component, log log.Component) concurrencyDomain {
	return concurrencyPolicyFromConfig(config, log).newDomain("test")
}
