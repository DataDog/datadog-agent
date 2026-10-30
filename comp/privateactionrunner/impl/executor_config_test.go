// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package privateactionrunnerimpl

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	parconstants "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/executor"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/stretchr/testify/require"
)

type investigationLoopbackTransport struct{ collector *url.URL }

func (tr investigationLoopbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	copy.URL.Scheme, copy.URL.Host = tr.collector.Scheme, tr.collector.Host
	return http.DefaultTransport.RoundTrip(copy)
}

// Diagnostic only: even injecting a sender is insufficient without executor Stop wiring.
func TestInvestigationExecutorStopDoesNotFlushTelemetry(t *testing.T) {
	var requests atomic.Int32
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1) }))
	defer collector.Close()
	collectorURL, err := url.Parse(collector.URL)
	require.NoError(t, err)
	sender := telemetry.NewTelemetry(&http.Client{Transport: investigationLoopbackTransport{collector: collectorURL}}, "fake-api-key", "datadoghq.com", "private-action-runner")
	stopped := false
	defer func() {
		if !stopped {
			sender.Stop()
		}
	}()
	span, _ := telemetry.StartSpanFromUint64IDs(context.Background(), "action.run", 123, 456)
	span.Finish(nil)
	done := make(chan struct{})
	close(done)
	runner := &PrivateActionRunner{
		started: true, startChan: done, executorDone: done, cancelStart: func() {},
		logger: logmock.New(t), telemetry: sender,
	}
	require.NoError(t, runner.StopExecutor(context.Background()))
	require.Zero(t, requests.Load(), "StopExecutor does not stop/flush even an injected sender")
	t.Log("StopExecutor returned with zero collector requests despite a finished span and an injected sender")
	sender.Stop()
	stopped = true
	require.Equal(t, int32(1), requests.Load(), "explicit sender Stop must reach the collector")
}

func TestDisabledExecutorDoesNotResolveIdentityOrInitializeActions(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
			"private_action_runner.enabled":       enabled,
			"private_action_runner.split_enabled": false,
			"private_action_runner.self_enroll":   true,
			"private_action_runner.private_key":   "invalid-key",
		})
		t.Setenv("DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED", "false")
		// Dependencies are deliberately absent: disabled startup must not use them.
		runner := &PrivateActionRunner{coreConfig: cfg}
		_, resolved, err := runner.configureExecutor(context.Background(), context.Background())
		require.NoError(t, err)
		require.Nil(t, resolved)
		require.NotNil(t, runner.executorServer)
		require.Nil(t, runner.encryptionStore)
		require.Nil(t, runner.keysManager)
	}
}

func TestRejectedEnrollmentStopsSplitExecutorWithoutActions(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusBadRequest, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.WriteHeader(status)
			}))
			defer srv.Close()
			t.Setenv(parconstants.InternalUseDDURLForOPMSEnvVar, "true")
			cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
				"dd_url":                                   srv.URL,
				"api_key":                                  "test-api-key",
				"app_key":                                  "test-app-key",
				"private_action_runner.enabled":            true,
				"private_action_runner.split_enabled":      true,
				"private_action_runner.self_enroll":        true,
				"private_action_runner.identity_file_path": filepath.Join(t.TempDir(), "identity.json"),
			})
			hostname, _ := hostnamemock.NewMock("test-host")
			runner := &PrivateActionRunner{coreConfig: cfg, hostnameGetter: hostname, logger: logmock.New(t)}

			_, err := runner.getRunnerConfig(context.Background())
			require.ErrorIs(t, err, opms.ErrEnrollmentRejected)
			require.NotContains(t, err.Error(), "backoff")
			_, resolved, err := runner.configureExecutor(context.Background(), context.Background())
			require.NoError(t, err)
			require.Nil(t, resolved)
			require.NotNil(t, runner.executorServer)
			require.Nil(t, runner.encryptionStore)
			snapshot, err := executor.ControlPlaneConfig(cfg, resolved)
			require.NoError(t, err)
			require.False(t, snapshot.SplitMode)

			stopped := make(chan struct{})
			runner.shutdowner = shutdownFunc(func() error { close(stopped); return nil })
			runner.startChan = make(chan struct{})
			require.NoError(t, runner.Start(context.Background()))
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("monolithic runner did not request a clean shutdown")
			}
			require.Equal(t, 3, calls)
		})
	}
}

type shutdownFunc func() error

func (f shutdownFunc) Shutdown() error { return f() }

func TestExecutorSnapshotUsesMonolithPersistedIdentity(t *testing.T) {
	key, _, err := util.GenerateKeys()
	require.NoError(t, err)
	encoded, err := key.MarshalJSON()
	require.NoError(t, err)
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"private_action_runner.urn":                util.MakeRunnerURN("us1", 123, "configured-runner"),
		"private_action_runner.private_key":        base64.RawURLEncoding.EncodeToString(encoded),
		"private_action_runner.identity_file_path": filepath.Join(t.TempDir(), "identity.json"),
	})
	persistedKey, _, err := util.GenerateKeys()
	require.NoError(t, err)
	persisted := &enrollment.Result{
		URN:        util.MakeRunnerURN("us1", 42, "persisted-runner"),
		PrivateKey: persistedKey.Key.(*ecdsa.PrivateKey), Hostname: "test-host",
	}
	require.NoError(t, enrollment.PersistIdentity(context.Background(), cfg, persisted))
	hostname, _ := hostnamemock.NewMock("test-host")
	runner := &PrivateActionRunner{coreConfig: cfg, hostnameGetter: hostname}
	resolved, err := runner.getRunnerConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, persisted.URN, resolved.Urn)
	snapshot, err := executor.ControlPlaneConfig(cfg, resolved)
	require.NoError(t, err)
	require.Equal(t, persisted.URN, snapshot.Identity.Urn)
	snapshotKey, err := util.Base64ToJWK(snapshot.Identity.PrivateKey)
	require.NoError(t, err)
	require.True(t, persisted.PrivateKey.Equal(snapshotKey.Key))
}
