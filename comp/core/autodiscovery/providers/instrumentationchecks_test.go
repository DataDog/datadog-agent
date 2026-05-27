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
)

// mockInstrumentationClient implements InstrumentationCheckClient.
type mockInstrumentationClient struct {
	configs types.ConfigResponse
	err     error
}

func (m *mockInstrumentationClient) GetInstrumentationConfigs(_ context.Context) (types.ConfigResponse, error) {
	return m.configs, m.err
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
		wantHeartbeat    bool // whether heartbeat should be non-zero after Collect
	}{
		{
			name: "success with configs",
			client: &mockInstrumentationClient{
				configs: types.ConfigResponse{Configs: []integration.Config{
					{Name: "check1"},
					{Name: "check2"},
				}},
			},
			degradedDuration: defaultDegradedDeadline,
			wantConfigs:      []integration.Config{{Name: "check1"}, {Name: "check2"}},
			wantFlushed:      false,
			wantHeartbeat:    true,
		},
		{
			name: "success resets flushedConfigs and updates heartbeat",
			client: &mockInstrumentationClient{
				configs: types.ConfigResponse{Configs: []integration.Config{{Name: "check1"}}},
			},
			degradedDuration: defaultDegradedDeadline,
			flushedConfigs:   true,
			wantConfigs:      []integration.Config{{Name: "check1"}},
			wantFlushed:      false,
			wantHeartbeat:    true,
		},
		{
			name:             "empty response still updates heartbeat",
			client:           &mockInstrumentationClient{configs: types.ConfigResponse{Configs: []integration.Config{}}},
			degradedDuration: defaultDegradedDeadline,
			wantConfigs:      []integration.Config{},
			wantFlushed:      false,
			wantHeartbeat:    true,
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
