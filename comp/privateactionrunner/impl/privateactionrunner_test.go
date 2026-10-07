// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunnerimpl

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

type blockingWorkloadAuthorizer struct {
	attempt chan context.Context
	stopped chan struct{}
}

func (a *blockingWorkloadAuthorizer) GetWorkloadAuthorization(ctx context.Context, _, _ string) (*common.WorkloadAuthorization, error) {
	a.attempt <- ctx
	<-ctx.Done()
	close(a.stopped)
	return nil, ctx.Err()
}

func TestWorkloadMigrationOutlivesStartupAndStopsWithRunner(t *testing.T) {
	for _, executor := range []bool{false, true} {
		name := "runner"
		if executor {
			name = "executor"
		}
		t.Run(name, func(t *testing.T) {
			cfg := mock.New(t)
			cfg.Set(setup.PARIdentityFilePath, filepath.Join(t.TempDir(), "identity.json"), model.SourceAgentRuntime)
			key, _, err := util.GenerateKeys()
			require.NoError(t, err)
			require.NoError(t, enrollment.PersistIdentity(context.Background(), cfg, &enrollment.Result{
				PrivateKey: key.Key.(*ecdsa.PrivateKey), URN: util.MakeRunnerURN("us1", 123, "runner-1"),
				Hostname: "test-host", APIKeyHash: enrollment.HashAPIKey("legacy-key"),
			}))
			authorizer := &blockingWorkloadAuthorizer{attempt: make(chan context.Context, 1), stopped: make(chan struct{})}
			runner := newStartedRunnerForStopTest(nil, false)
			runner.coreConfig = cfg
			runner.workloadAuthorizer = authorizer
			startup, cancelStartup := context.WithCancel(context.Background())
			defer cancelStartup()
			runner.startWorkloadRefresh(startup)
			t.Cleanup(runner.workloadRefreshCancel)
			var attempt context.Context
			select {
			case attempt = <-authorizer.attempt:
			case <-time.After(5 * time.Second):
				t.Fatal("migration did not start")
			}
			cancelStartup()
			require.NoError(t, attempt.Err(), "startup cancellation must not stop migration")
			if executor {
				require.NoError(t, runner.StopExecutor(context.Background()))
			} else {
				require.NoError(t, runner.Stop(context.Background()))
			}
			select {
			case <-authorizer.stopped:
			case <-time.After(5 * time.Second):
				t.Fatal("runner shutdown did not cancel migration")
			}
		})
	}
}

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
		{name: "container env", containerized: true, envValue: "true", want: true},
		{name: "container config only", configEnabled: true, containerized: true},
		{name: "disabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, splitDeploymentEnabled(tt.configEnabled, tt.containerized, tt.envValue))
		})
	}
}

func TestSplitDeploymentSupported(t *testing.T) {
	tests := []struct {
		name                  string
		goos                  string
		containerized         bool
		fipsEnabled           bool
		processManagerEnabled bool
		want                  bool
	}{
		{name: "linux host", goos: "linux", want: true},
		{name: "linux container", goos: "linux", containerized: true, want: true},
		{name: "windows host", goos: "windows", processManagerEnabled: true, want: true},
		{name: "windows without process manager", goos: "windows"},
		{name: "windows container", goos: "windows", containerized: true, processManagerEnabled: true},
		{name: "linux FIPS host", goos: "linux", fipsEnabled: true},
		{name: "unsupported host platform", goos: "darwin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, splitDeploymentSupported(tt.goos, tt.containerized, tt.fipsEnabled, tt.processManagerEnabled))
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
