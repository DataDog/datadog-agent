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

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// The monolithic runner and par-control both dequeue from OPMS, so exactly one
// of them may run. In split mode the launch path starts both (the systemd/procmgr
// units are unconditional), and this gate is what keeps them from double-dequeuing:
// the monolith exits cleanly and leaves OPMS to par-control.
func TestNewComponentStandsDownInSplitMode(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		split   bool
		wantErr error
	}{
		{name: "disabled runner", wantErr: privateactionrunner.ErrNotEnabled},
		{name: "split mode stands down", enabled: true, split: true, wantErr: privateactionrunner.ErrSplitDeployment},
		{name: "disabled wins over split", split: true, wantErr: privateactionrunner.ErrNotEnabled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest(privateactionrunner.PAREnabled, tt.enabled)
			cfg.SetInTest(privateactionrunner.PARSplitEnabled, tt.split)

			_, err := NewComponent(Requires{Config: cfg, Log: logmock.New(t)})

			assert.ErrorIs(t, err, tt.wantErr)
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
