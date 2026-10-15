// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	mock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestConcurrencyPolicyFromConfig(t *testing.T) {
	mockConfig := mock.New(t)
	mockConfig.SetInTest("forwarder_num_workers", 3)
	mockConfig.SetInTest("forwarder_max_concurrent_requests", 5)

	p := concurrencyPolicyFromConfig(mockConfig, logmock.New(t))
	assert.Equal(t, 3, p.numWorkers())
	assert.Equal(t, 3, p.maxConns())
	assert.Equal(t, int64(5), p.requests)
	assert.NotSame(t, p.newRequestSemaphore(), p.newRequestSemaphore())

	local := p.forLocal()
	assert.Equal(t, 1, local.numWorkers())
	assert.Equal(t, 1, local.maxConns())
	assert.Equal(t, int64(5), local.(staticConcurrencyPolicy).requests)
}

func TestConcurrencyPolicyFromConfigInvalidRequests(t *testing.T) {
	mockConfig := mock.New(t)
	mockConfig.SetInTest("forwarder_max_concurrent_requests", 0)

	p := concurrencyPolicyFromConfig(mockConfig, logmock.New(t))
	assert.Equal(t, int64(1), p.requests)
}

func newTestConcurrencyPolicy(workers int, requests int64) staticConcurrencyPolicy {
	return staticConcurrencyPolicy{
		workers:     workers,
		connections: workers,
		requests:    requests,
	}
}
