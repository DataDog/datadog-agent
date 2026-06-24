// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunnerimpl

import (
	"context"
	"errors"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStopFlushesAndClosesOwnedMetricsClient(t *testing.T) {
	metricsClient := &recordingMetricsClient{}
	runner := newStartedRunnerForStopTest(metricsClient, true)

	err := runner.Stop(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, metricsClient.flushCalls)
	assert.Equal(t, 1, metricsClient.closeCalls)
}

func TestStopDoesNotFlushOrCloseUnownedMetricsClient(t *testing.T) {
	metricsClient := &recordingMetricsClient{}
	runner := newStartedRunnerForStopTest(metricsClient, false)

	err := runner.Stop(context.Background())

	require.NoError(t, err)
	assert.Zero(t, metricsClient.flushCalls)
	assert.Zero(t, metricsClient.closeCalls)
}

func TestStopReturnsMetricsClientCleanupErrors(t *testing.T) {
	metricsClient := &recordingMetricsClient{
		flushErr: errors.New("flush failed"),
		closeErr: errors.New("close failed"),
	}
	runner := newStartedRunnerForStopTest(metricsClient, true)

	err := runner.Stop(context.Background())

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to flush metrics client: flush failed")
	assert.ErrorContains(t, err, "failed to close metrics client: close failed")
	assert.Equal(t, 1, metricsClient.flushCalls)
	assert.Equal(t, 1, metricsClient.closeCalls)
}

func newStartedRunnerForStopTest(metricsClient statsd.ClientInterface, ownsMetricsClient bool) *PrivateActionRunner {
	startChan := make(chan struct{})
	close(startChan)
	return &PrivateActionRunner{
		started:           true,
		startChan:         startChan,
		cancelStart:       func() {},
		metricsClient:     metricsClient,
		ownsMetricsClient: ownsMetricsClient,
	}
}

type recordingMetricsClient struct {
	statsd.NoOpClient
	flushCalls int
	closeCalls int
	flushErr   error
	closeErr   error
}

func (r *recordingMetricsClient) Flush() error {
	r.flushCalls++
	return r.flushErr
}

func (r *recordingMetricsClient) Close() error {
	r.closeCalls++
	return r.closeErr
}
