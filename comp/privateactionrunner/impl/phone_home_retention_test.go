// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package privateactionrunnerimpl

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
)

func TestPhoneHomeRetainedHelperServesPendingNotDisabled(t *testing.T) {
	for _, stage := range []string{"pending", "outcome", "active_and_outcome"} {
		for _, brokenIPC := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/broken_ipc_%t", stage, brokenIPC), func(t *testing.T) { testPhoneHomeRetainer(t, stage, brokenIPC) })
		}
	}
}

func testPhoneHomeRetainer(t *testing.T, stage string, brokenIPC bool) {
	t.Helper()
	t.Setenv(app.PhoneHomePOCEnvVar, "true")
	t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED", "true")
	// Keep the Unix socket below macOS's path-length limit.
	dir, err := os.MkdirTemp("", "par-retain-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	identityPath := filepath.Join(dir, "identity.json")
	socketPath := filepath.Join(dir, "executor.sock")
	if brokenIPC {
		obstruction := filepath.Join(dir, "blocked")
		require.NoError(t, os.WriteFile(obstruction, nil, 0600))
		socketPath = filepath.Join(obstruction, "executor.sock")
	}
	pendingPath, blockedPath := "", ""
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		posts.Add(1)
		// Deterministic write failures after commit, without UID-dependent chmod.
		if stage == "pending" {
			require.NoError(t, os.Rename(pendingPath, pendingPath+".prepared"))
		}
		require.NoError(t, os.Mkdir(blockedPath, 0700))
		if stage == "active_and_outcome" {
			require.NoError(t, os.Mkdir(identityPath, 0700))
		}
		fmt.Fprint(w, `{"data":{"type":"createRunnerResponse","id":"runner","attributes":{"org_id":42,"runner_id":"runner"}}}`)
	}))
	defer srv.Close()
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"api_key": "fake-key", "dd_url": srv.URL, "private_action_runner.enabled": true, "private_action_runner.self_enroll": true,
		"private_action_runner.split_enabled": true, "private_action_runner.api_key_only_enrollment": true,
		"private_action_runner.identity_file_path": identityPath, "private_action_runner.executor.socket_path": socketPath,
	})
	a, err := enrollment.ReservePhoneHomeAttempt(cfg, "test-host", time.Now(), nil)
	require.NoError(t, err)
	pendingPath = identityPath + ".pending-" + a.ID
	blockedPath = pendingPath
	if stage != "pending" {
		blockedPath = filepath.Join(enrollment.PhoneHomeAttemptPath(cfg), a.ID, "outcome.json")
	}
	hostname, _ := hostnamemock.NewMock("test-host")
	ipc := ipcmock.New(t)
	mockClock := clock.NewMock()
	stopped := make(chan struct{})
	var once sync.Once
	runner := &PrivateActionRunner{coreConfig: cfg, hostnameGetter: hostname, logger: logmock.New(t), ipc: ipc, startChan: make(chan struct{}), persistenceClock: mockClock,
		shutdowner: shutdownFunc(func() error { once.Do(func() { close(stopped) }); return nil })}
	require.NoError(t, runner.StartExecutor(context.Background()))
	if brokenIPC {
		require.Nil(t, runner.executorDone)
	} else {
		require.NotNil(t, runner.executorDone)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		require.NoError(t, runner.StopExecutor(ctx))
	})
	outcome, err := enrollment.ReadPhoneHomeOutcome(cfg, a)
	if stage == "pending" {
		require.NoError(t, err)
		require.True(t, outcome.HelperRetained)
	} else {
		require.Error(t, err, "journal is still obstructed; the helper must remain alive")
	}
	health, err := runner.executorServer.Health(context.Background(), &pb.HealthRequest{})
	require.NoError(t, err)
	require.False(t, health.Ready)
	cert, err := x509.ParseCertificate(ipc.GetTLSServerConfig().Certificates[0].Certificate[0])
	require.NoError(t, err)
	ctx := peer.NewContext(context.Background(), &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{cert}}, PeerCertificates: []*x509.Certificate{cert}}}})
	snapshot, err := runner.executorServer.GetControlPlaneConfig(ctx, &pb.GetControlPlaneConfigRequest{})
	require.Equal(t, codes.Unavailable, status.Code(err))
	require.Nil(t, snapshot)
	mockClock.Add(10 * time.Minute)
	select {
	case <-stopped:
		t.Fatal("credential holder must not idle out")
	default:
	}
	// Remove the obstruction; let the helper atomically publish its complete
	// in-memory record. Restoring the old prepare file could race that write.
	require.NoError(t, os.Remove(blockedPath))
	mockClock.Add(time.Minute)
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("retainer did not save identity and stop")
	}
	outcome, err = enrollment.ReadPhoneHomeOutcome(cfg, a)
	require.NoError(t, err)
	require.True(t, outcome.PersistencePending)
	require.False(t, outcome.HelperRetained)
	switch stage {
	case "pending":
		require.NoFileExists(t, identityPath, "retainer only saves pending credentials")
	case "outcome":
		require.FileExists(t, identityPath, "active identity is saved but not yet acknowledged")
	case "active_and_outcome":
		require.DirExists(t, identityPath)
		require.NoError(t, os.Remove(identityPath))
	}
	data, readErr := os.ReadFile(pendingPath)
	require.NoError(t, readErr)
	var metadata struct {
		URN string `json:"urn"`
	}
	require.NoError(t, json.Unmarshal(data, &metadata))
	require.NotEmpty(t, metadata.URN, "returned identity must survive the retention retry")
	require.Equal(t, identityPath, cfg.GetString("private_action_runner.identity_file_path"))
	require.NoError(t, enrollment.RecoverPhoneHomeIdentity(context.Background(), cfg, "test-host"))
	require.FileExists(t, identityPath)
	identity, err := enrollment.GetIdentityFromPreviousEnrollment(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, identity)
	require.Contains(t, identity.URN, "runner")
	require.EqualValues(t, 1, posts.Load())
}
