// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package providers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
)

// mockInstrumentationClient implements InstrumentationCheckClient.
type mockInstrumentationClient struct {
	configs   types.InstrumentationConfigResponse
	err       error
	status    types.InstrumentationStatusResponse
	statusErr error
}

func (m *mockInstrumentationClient) GetInstrumentationConfigs(_ context.Context) (types.InstrumentationConfigResponse, error) {
	return m.configs, m.err
}

func (m *mockInstrumentationClient) GetInstrumentationStatus(_ context.Context) (types.InstrumentationStatusResponse, error) {
	return m.status, m.statusErr
}

func TestInstrumentationChecksCollect(t *testing.T) {
	remoteErr := dderrors.NewRemoteServiceError("cluster-agent", "500 Internal Server Error")
	timeoutErr := dderrors.NewTimeoutError("cluster-agent", errors.New("context deadline exceeded"))
	genericErr := errors.New("unexpected error")

	tests := []struct {
		name             string
		client           *mockInstrumentationClient
		degradedDuration time.Duration
		heartbeat        time.Time
		flushedConfigs   bool
		wantConfigs      []integration.Config
		wantErr          bool
		wantErrVal       error
		wantFlushed      bool
		wantHeartbeat    bool
		wantConfigHash   uint64
	}{
		{
			name: "success with configs",
			client: &mockInstrumentationClient{
				configs: types.InstrumentationConfigResponse{
					ConfigHash: 12345,
					Configs: []integration.Config{
						{Name: "check1"},
						{Name: "check2"},
					}},
			},
			degradedDuration: defaultDegradedDeadline,
			wantConfigs:      []integration.Config{{Name: "check1"}, {Name: "check2"}},
			wantFlushed:      false,
			wantHeartbeat:    true,
			wantConfigHash:   12345,
		},
		{
			name: "success resets flushedConfigs and updates heartbeat",
			client: &mockInstrumentationClient{
				configs: types.InstrumentationConfigResponse{ConfigHash: 99999, Configs: []integration.Config{{Name: "check1"}}},
			},
			degradedDuration: defaultDegradedDeadline,
			flushedConfigs:   true,
			wantConfigs:      []integration.Config{{Name: "check1"}},
			wantFlushed:      false,
			wantHeartbeat:    true,
			wantConfigHash:   99999,
		},
		{
			name:             "empty response still updates heartbeat",
			client:           &mockInstrumentationClient{configs: types.InstrumentationConfigResponse{ConfigHash: 1, Configs: []integration.Config{}}},
			degradedDuration: defaultDegradedDeadline,
			wantConfigs:      []integration.Config{},
			wantFlushed:      false,
			wantHeartbeat:    true,
			wantConfigHash:   1,
		},
		{
			name:             "remote error within infinite degraded mode returns error",
			client:           &mockInstrumentationClient{err: remoteErr},
			degradedDuration: defaultDegradedDeadline, // 0 = infinite degraded mode
			wantErr:          true,
			wantErrVal:       remoteErr,
		},
		{
			name:             "timeout error within degraded mode returns error",
			client:           &mockInstrumentationClient{err: timeoutErr},
			degradedDuration: 5 * time.Minute,
			heartbeat:        time.Now(), // recent heartbeat = within window
			wantErr:          true,
			wantErrVal:       timeoutErr,
		},
		{
			name:             "remote error outside degraded mode flushes configs on first call",
			client:           &mockInstrumentationClient{err: remoteErr},
			degradedDuration: 5 * time.Minute,
			heartbeat:        time.Now().Add(-10 * time.Minute), // outside window
			flushedConfigs:   false,
			wantErr:          false,
			wantFlushed:      true,
		},
		{
			name:             "remote error outside degraded mode propagates error after flush",
			client:           &mockInstrumentationClient{err: remoteErr},
			degradedDuration: 5 * time.Minute,
			heartbeat:        time.Now().Add(-10 * time.Minute), // outside window
			flushedConfigs:   true,
			wantErr:          true,
			wantErrVal:       remoteErr,
		},
		{
			name:             "non-retriable error flushes on first call",
			client:           &mockInstrumentationClient{err: genericErr},
			degradedDuration: defaultDegradedDeadline,
			flushedConfigs:   false,
			wantErr:          false,
			wantFlushed:      true,
		},
		{
			name:             "non-retriable error propagates after flush",
			client:           &mockInstrumentationClient{err: genericErr},
			degradedDuration: defaultDegradedDeadline,
			flushedConfigs:   true,
			wantErr:          true,
			wantErrVal:       genericErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &InstrumentationChecksConfigProvider{
				dcaClient:        tt.client,
				degradedDuration: tt.degradedDuration,
				heartbeat:        tt.heartbeat,
				flushedConfigs:   tt.flushedConfigs,
			}

			configs, err := provider.Collect(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrVal != nil {
					assert.Equal(t, tt.wantErrVal, err)
				}
				assert.Nil(t, configs)
				return
			}

			require.NoError(t, err)
			require.Len(t, configs, len(tt.wantConfigs))
			for _, cfg := range configs {
				assert.Equal(t, names.InstrumentationChecks, cfg.Provider)
			}
			assert.Equal(t, tt.wantFlushed, provider.flushedConfigs)
			assert.Equal(t, tt.wantConfigHash, provider.configHash)
			if tt.wantHeartbeat {
				assert.False(t, provider.heartbeat.IsZero())
			}
		})
	}
}

func TestInstrumentationChecksCollectNilClientInitFails(t *testing.T) {
	provider := &InstrumentationChecksConfigProvider{
		dcaClient:        nil,
		degradedDuration: defaultDegradedDeadline,
	}

	configs, err := provider.Collect(context.Background())
	assert.Nil(t, configs)
	assert.Error(t, err)
}

func TestInstrumentationChecksIsUpToDate(t *testing.T) {
	tests := []struct {
		name           string
		client         clusteragent.InstrumentationCheckClient
		configHash     uint64
		flushedConfigs bool
		wantResult     bool
	}{
		{
			name:       "timestamps match — up to date",
			client:     &mockInstrumentationClient{status: types.InstrumentationStatusResponse{ConfigHash: 100}},
			configHash: 100,
			wantResult: true,
		},
		{
			name:       "server timestamp is newer — not up to date",
			client:     &mockInstrumentationClient{status: types.InstrumentationStatusResponse{ConfigHash: 200}},
			configHash: 100,
			wantResult: false,
		},
		{
			name:       "no changes yet — both zero",
			client:     &mockInstrumentationClient{status: types.InstrumentationStatusResponse{ConfigHash: 0}},
			configHash: 0,
			wantResult: true,
		},
		{
			name:           "flushed configs — force Collect even if timestamps match",
			client:         &mockInstrumentationClient{status: types.InstrumentationStatusResponse{ConfigHash: 100}},
			configHash:     100,
			flushedConfigs: true,
			wantResult:     false,
		},
		{
			name:       "status endpoint error — fall through to Collect",
			client:     &mockInstrumentationClient{statusErr: errors.New("connection refused")},
			configHash: 100,
			wantResult: false,
		},
		{
			name:       "nil client — fall through to Collect",
			client:     nil,
			configHash: 0,
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &InstrumentationChecksConfigProvider{
				dcaClient:      tt.client,
				configHash:     tt.configHash,
				flushedConfigs: tt.flushedConfigs,
			}
			upToDate, err := provider.IsUpToDate(context.Background())
			assert.NoError(t, err)
			assert.Equal(t, tt.wantResult, upToDate)
		})
	}
}
