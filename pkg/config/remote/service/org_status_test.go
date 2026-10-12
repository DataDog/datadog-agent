// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestOrgStatusSnapshot(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, authorized := range []bool{false, true} {
			t.Run(fmt.Sprintf("%t_%t", enabled, authorized), func(t *testing.T) {
				clk := clock.NewMock()
				client := &mockAPI{}
				s := newTestService(t, client, &mockCoreAgentUptane{}, clk, WithOrgStatusRefreshInterval(time.Minute, "test"))
				require.Equal(t, OrgStatusUnchecked, s.GetOrgStatus().State)
				require.False(t, s.GetOrgStatus().Ready(clk.Now()))
				response := &pbgo.OrgStatusResponse{Enabled: enabled, Authorized: authorized}
				client.On("FetchOrgStatus", mock.Anything).Return(response, nil).Once()
				s.orgStatusPoller.poll(client, s.rcType)
				snapshot := s.GetOrgStatus()
				require.Equal(t, OrgStatusObserved, snapshot.State)
				require.Equal(t, enabled, snapshot.Enabled)
				require.Equal(t, authorized, snapshot.Authorized)
				require.Equal(t, enabled && authorized, snapshot.Ready(clk.Now()))
				require.True(t, snapshot.Matches("abc", "https://localhost"))
				require.False(t, snapshot.Matches("wrong", "https://localhost"))
				require.False(t, snapshot.Matches("abc", "https://different.site"))
				response.Enabled = !enabled
				snapshot.Enabled = !enabled
				require.Equal(t, enabled, s.GetOrgStatus().Enabled, "both input and output are copies")
				encoded, err := json.Marshal(snapshot)
				require.NoError(t, err)
				require.NotContains(t, string(encoded), "abc")
				require.NotContains(t, string(encoded), "localhost")
				clk.Add(2 * time.Minute)
				require.Equal(t, OrgStatusStale, s.GetOrgStatus().State)
				require.False(t, snapshot.Ready(clk.Now()), "saved snapshots expire too")
				client.AssertNumberOfCalls(t, "FetchOrgStatus", 1)
			})
		}
	}
	var absent *CoreAgentService
	require.Equal(t, OrgStatusUnavailable, absent.GetOrgStatus().State)
}

func TestOrgStatusFreshnessCap(t *testing.T) {
	clk := clock.NewMock()
	client := &mockAPI{}
	s := newTestService(t, client, &mockCoreAgentUptane{}, clk, WithOrgStatusRefreshInterval(time.Hour, "test"))
	client.On("FetchOrgStatus", mock.Anything).Return(&pbgo.OrgStatusResponse{Enabled: true, Authorized: true}, nil).Once()
	s.orgStatusPoller.poll(client, s.rcType)
	require.Equal(t, time.Hour, s.orgStatusPoller.refreshInterval, "freshness must not change polling cadence")
	require.Equal(t, 5*time.Minute, s.GetOrgStatus().ValidUntil.Sub(clk.Now()))
	clk.Add(5 * time.Minute)
	require.Equal(t, OrgStatusStale, s.GetOrgStatus().State)
	client.AssertNumberOfCalls(t, "FetchOrgStatus", 1)
}

func TestOrgStatusFailureAndGeneration(t *testing.T) {
	clk := clock.NewMock()
	client := &mockAPI{}
	s := newTestService(t, client, &mockCoreAgentUptane{}, clk)
	client.On("FetchOrgStatus", mock.Anything).Return(&pbgo.OrgStatusResponse{Enabled: true, Authorized: true}, nil).Once()
	s.orgStatusPoller.poll(client, s.rcType)
	first := s.GetOrgStatus()
	client.On("FetchOrgStatus", mock.Anything).Return((*pbgo.OrgStatusResponse)(nil), api.ErrUnauthorized).Once()
	s.orgStatusPoller.poll(client, s.rcType)
	failed := s.GetOrgStatus()
	require.Equal(t, OrgStatusFetchFailed, failed.State)
	require.True(t, failed.Enabled)
	require.True(t, failed.Authorized)
	require.False(t, failed.Ready(clk.Now()))
	require.Greater(t, failed.Generation, first.Generation)
	require.True(t, s.orgStatusPoller.getPreviousStatus().Authorized, "diagnostics retain last success")
	client.On("FetchOrgStatus", mock.Anything).Return(&pbgo.OrgStatusResponse{Enabled: true, Authorized: true}, nil).Once()
	s.orgStatusPoller.poll(client, s.rcType)
	restored := s.GetOrgStatus()
	require.True(t, restored.Ready(clk.Now()))
	require.Equal(t, failed.Generation, restored.Generation)
	require.NoError(t, s.Stop())
	require.Equal(t, OrgStatusUnavailable, s.GetOrgStatus().State)
}

func TestOrgStatusLateCredentialResponse(t *testing.T) {
	clk := clock.NewMock()
	client := &mockAPI{}
	s := newTestService(t, client, &mockCoreAgentUptane{}, clk)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	client.On("FetchOrgStatus", mock.Anything).Run(func(mock.Arguments) { close(entered); <-release }).Return(&pbgo.OrgStatusResponse{Enabled: true, Authorized: true}, nil).Once()
	go func() { s.orgStatusPoller.poll(client, s.rcType); close(done) }()
	<-entered
	client.On("UpdateAPIKey", "other").Return().Once()
	client.On("UpdateAPIKey", "abc").Return().Once()
	s.orgStatusPoller.updateAPIKey(client, "other")
	s.orgStatusPoller.updateAPIKey(client, "abc")
	close(release)
	<-done
	require.Equal(t, OrgStatusUnchecked, s.GetOrgStatus().State, "even restoring the original key cannot admit the old response")
	client.On("FetchOrgStatus", mock.Anything).Return(&pbgo.OrgStatusResponse{Enabled: true, Authorized: true}, nil).Once()
	s.orgStatusPoller.poll(client, s.rcType)
	require.True(t, s.GetOrgStatus().Ready(clk.Now()))
	client.On("UpdatePARJWT", "jwt").Return().Once()
	s.UpdatePARJWT("jwt")
	client.On("FetchOrgStatus", mock.Anything).Return(&pbgo.OrgStatusResponse{Enabled: true, Authorized: true}, nil).Once()
	s.orgStatusPoller.poll(client, s.rcType)
	require.False(t, s.GetOrgStatus().Matches("abc", "https://localhost"))
}

func TestOrgStatusConfigChanges(t *testing.T) {
	for _, setting := range []string{"api_key", "remote_configuration.api_key", "remote_configuration.rc_dd_url", "remote_configuration.key", "remote_configuration.enabled", "site"} {
		t.Run(setting, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("run_path", t.TempDir())
			store := &mockCoreAgentUptane{}
			s, err := NewService(cfg, "test", "https://localhost", "host", getHostTags, &telemetryReporter{}, agentVersion, WithAPIKey("old"), uptaneFactoryOption(store))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, s.Stop()) })
			client := &mockAPI{}
			s.api = client
			client.On("FetchOrgStatus", mock.Anything).Return(&pbgo.OrgStatusResponse{Enabled: true, Authorized: true}, nil)
			s.orgStatusPoller.poll(client, s.rcType)
			before := s.GetOrgStatus()
			if setting == "api_key" {
				client.On("UpdateAPIKey", "new").Return().Once()
				store.On("StoredOrgUUID").Return("", errors.New("test stop before UUID request")).Once()
			}
			var value any = "new"
			if setting == "remote_configuration.enabled" {
				value = false
			}
			cfg.Set(setting, value, model.SourceAgentRuntime)
			after := s.GetOrgStatus()
			require.Greater(t, after.Generation, before.Generation)
			require.False(t, after.Ready(time.Now()))
			s.orgStatusPoller.poll(client, s.rcType)
			if setting == "api_key" {
				require.True(t, s.GetOrgStatus().Matches("new", "https://localhost"))
				require.True(t, s.GetOrgStatus().Ready(time.Now()))
			} else {
				require.Equal(t, OrgStatusConfigChanged, s.GetOrgStatus().State)
				require.False(t, s.GetOrgStatus().Ready(time.Now()))
			}
		})
	}
}

func TestOrgStatusExpiryDuringFetchChangesGeneration(t *testing.T) {
	clk := clock.NewMock()
	client := &mockAPI{}
	s := newTestService(t, client, &mockCoreAgentUptane{}, clk, WithOrgStatusRefreshInterval(time.Minute, "test"))
	client.On("FetchOrgStatus", mock.Anything).Return(&pbgo.OrgStatusResponse{Enabled: true, Authorized: true}, nil).Once()
	s.orgStatusPoller.poll(client, s.rcType)
	first := s.GetOrgStatus()
	clk.Add(90 * time.Second)
	client.On("FetchOrgStatus", mock.Anything).Run(func(mock.Arguments) { clk.Add(time.Minute) }).Return(&pbgo.OrgStatusResponse{Enabled: true, Authorized: true}, nil).Once()
	s.orgStatusPoller.poll(client, s.rcType)
	require.True(t, s.GetOrgStatus().Ready(clk.Now()))
	require.Greater(t, s.GetOrgStatus().Generation, first.Generation)
}
