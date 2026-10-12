// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package phonehome

import (
	"fmt"
	"testing"
	"time"

	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	remote "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/stretchr/testify/require"
)

type statusService struct {
	rcservice.Component
	rc     remote.OrgStatus
	scope  remote.PARScopeObservation
	wanted bool
}

func (s *statusService) GetOrgStatus() remote.OrgStatus          { return s.rc }
func (s *statusService) GetPARScope() remote.PARScopeObservation { return s.scope }
func (s *statusService) SetPARDiscoveryDemand(v bool)            { s.wanted = v }

func TestReaderFreezesLoopbackEndpointsEvenForIdentityReuse(t *testing.T) {
	t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "true")
	cfg := configmock.New(t)
	cfg.SetInTest("remote_configuration.enabled", true)
	cfg.SetInTest("api_key", "key")
	cfg.SetInTest("dd_url", "http://127.0.0.1:1234")
	cfg.SetInTest("remote_configuration.rc_dd_url", "http://127.0.0.1:5678")
	now := time.Now()
	service := &statusService{rc: remote.MockOrgStatus("key", "http://127.0.0.1:5678", remote.OrgStatus{State: remote.OrgStatusObserved, Enabled: true, Authorized: true, ObservedAt: now, ValidUntil: now.Add(time.Minute), Generation: 1})}
	source := NewSource(cfg, service)
	require.True(t, source.Read(now).RCReady)
	cfg.SetInTest("dd_url", "http://127.0.0.1:4321")
	require.False(t, source.Read(now).RCReady)
	require.Equal(t, "rc_credential_or_endpoint_mismatch", source.Read(now).RCReason)
}

func TestReaderSeparatesRCAndScopeAndFailsClosed(t *testing.T) {
	t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "false")
	cfg := configmock.New(t)
	cfg.SetInTest("site", "datadoghq.com")
	cfg.SetInTest("dd_url", "https://api.datadoghq.com")
	cfg.SetInTest("remote_configuration.rc_dd_url", "https://config.datadoghq.com")
	cfg.SetInTest("api_key", "key")
	cfg.SetInTest("remote_configuration.enabled", true)
	now := time.Now()
	service := &statusService{}
	source := NewSource(cfg, service)
	for _, enabled := range []bool{false, true} {
		for _, authorized := range []bool{false, true} {
			t.Run(fmt.Sprintf("%t_%t", enabled, authorized), func(t *testing.T) {
				service.rc = remote.MockOrgStatus("key", "https://config.datadoghq.com", remote.OrgStatus{State: remote.OrgStatusObserved, Enabled: enabled, Authorized: authorized, ObservedAt: now, ValidUntil: now.Add(time.Minute), Generation: 1})
				service.scope = remote.MockPARScope("key", "https://api.datadoghq.com", remote.PARScopeObservation{Reason: "scope_present", CheckedAt: now, ValidUntil: now.Add(time.Minute), RCGeneration: 1})
				p := source.Read(now)
				require.Equal(t, enabled && authorized, p.RCReady, p.RCReason)
				require.Equal(t, enabled && authorized, p.ScopeReady)
			})
		}
	}
	service.scope = (&remote.CoreAgentService{}).GetPARScope() // actual PR-2 producer is unknown
	p := source.Read(now)
	require.True(t, p.RCReady)
	require.False(t, p.ScopeReady)
	require.Equal(t, "par_scope_unchecked", p.ScopeReason)
	for _, state := range []remote.OrgStatusState{remote.OrgStatusUnchecked, remote.OrgStatusFetchFailed, remote.OrgStatusStale, remote.OrgStatusUnavailable, remote.OrgStatusConfigChanged} {
		service.rc.State = state
		require.False(t, source.Read(now).RCReady)
	}
	service.rc.State = remote.OrgStatusObserved
	require.False(t, source.Read(now.Add(2*time.Minute)).RCReady)
	cfg.SetInTest("api_key", "different")
	require.Equal(t, "rc_credential_or_endpoint_mismatch", source.Read(now).RCReason)
	cfg.SetInTest("api_key", "key")
	cfg.SetInTest("remote_configuration.rc_dd_url", "https://config.datadoghq.eu")
	require.False(t, source.Read(now).RCReady)
	cfg.SetInTest("remote_configuration.enabled", false)
	require.Equal(t, "rc_locally_disabled", source.Read(now).RCReason)
	cfg.SetInTest("remote_configuration.enabled", true)
	require.Equal(t, "rc_service_unavailable", NewSource(cfg, nil).Read(now).RCReason)
}
