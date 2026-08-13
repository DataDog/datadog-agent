// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunnerimpl

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	parutil "github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
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

func TestGetRunnerConfigDiscardsCorruptIdentity(t *testing.T) {
	// Must match bootstrap-par-control: recover instead of wedging startup.
	identityPath := filepath.Join(t.TempDir(), "identity.json")
	require.NoError(t, os.WriteFile(identityPath, []byte("not-json"), 0o600))

	privateJWK, _, err := parutil.GenerateKeys()
	require.NoError(t, err)
	encodedKey, err := privateJWK.MarshalJSON()
	require.NoError(t, err)
	urn := parutil.MakeRunnerURN("us1", 123, "test-runner")

	hostnameGetter, _ := hostnamemock.NewMock("test-host")
	runner := &PrivateActionRunner{
		coreConfig: coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"private_action_runner.enabled":            true,
			"private_action_runner.identity_file_path": identityPath,
			"private_action_runner.self_enroll":        false,
			"private_action_runner.urn":                urn,
			"private_action_runner.private_key":        base64.RawURLEncoding.EncodeToString(encodedKey),
		}),
		hostnameGetter: hostnameGetter,
		logger:         logmock.New(t),
	}

	cfg, err := runner.getRunnerConfig(context.Background())

	require.NoError(t, err)
	assert.Equal(t, urn, cfg.Urn)
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
