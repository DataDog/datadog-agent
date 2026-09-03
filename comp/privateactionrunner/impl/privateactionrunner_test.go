// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunnerimpl

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutorIdleTimeout(t *testing.T) {
	for _, tt := range []struct {
		name        string
		idleSeconds int
		want        time.Duration
	}{
		{name: "disabled", idleSeconds: 0, want: 0},
		{name: "negative is disabled", idleSeconds: -1, want: 0},
		{name: "uses configured duration", idleSeconds: 60, want: time.Minute},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, executorIdleTimeout(tt.idleSeconds))
		})
	}
}

func TestSplitDeploymentEnabled(t *testing.T) {
	tests := []struct {
		name          string
		configEnabled bool
		containerized bool
		envValue      string
		want          bool
	}{
		{name: "host config", configEnabled: true, want: true},
		{name: "container env", configEnabled: true, containerized: true, envValue: "true", want: true},
		{name: "container config only", configEnabled: true, containerized: true},
		{name: "disabled", envValue: "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, splitDeploymentEnabled(tt.configEnabled, tt.containerized, tt.envValue))
		})
	}
}

func TestSplitDeploymentSupported(t *testing.T) {
	tests := []struct {
		name        string
		goos        string
		fipsEnabled bool
		want        bool
	}{
		{name: "linux", goos: "linux", want: true},
		{name: "linux FIPS", goos: "linux", fipsEnabled: true},
		{name: "unsupported platform", goos: "windows"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, splitDeploymentSupported(tt.goos, tt.fipsEnabled))
		})
	}
}

func TestStopCleansUpMetricsClient(t *testing.T) {
	tests := []struct {
		name           string
		ownsClient     bool
		flushErr       error
		closeErr       error
		wantErrs       []string
		wantFlushCalls int
		wantCloseCalls int
	}{
		{
			name:           "flushes and closes owned metrics client",
			ownsClient:     true,
			wantFlushCalls: 1,
			wantCloseCalls: 1,
		},
		{
			name: "does not flush or close unowned metrics client",
		},
		{
			name:       "returns metrics client cleanup errors",
			ownsClient: true,
			flushErr:   errors.New("flush failed"),
			closeErr:   errors.New("close failed"),
			wantErrs: []string{
				"failed to flush metrics client: flush failed",
				"failed to close metrics client: close failed",
			},
			wantFlushCalls: 1,
			wantCloseCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metricsClient := &recordingMetricsClient{
				flushErr: tt.flushErr,
				closeErr: tt.closeErr,
			}
			runner := newStartedRunnerForStopTest(metricsClient, tt.ownsClient)

			err := runner.Stop(context.Background())

			if len(tt.wantErrs) == 0 {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				for _, wantErr := range tt.wantErrs {
					assert.ErrorContains(t, err, wantErr)
				}
			}
			assert.Equal(t, tt.wantFlushCalls, metricsClient.flushCalls)
			assert.Equal(t, tt.wantCloseCalls, metricsClient.closeCalls)
		})
	}
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
