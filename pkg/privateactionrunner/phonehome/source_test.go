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
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/stretchr/testify/require"
)

type statusService struct {
	rcservice.Component
	rc     *pbgo.OrgStatusResponse
	scope  remote.PARScopeObservation
	wanted bool
}

func (s *statusService) GetOrgStatus() *pbgo.OrgStatusResponse   { return s.rc }
func (s *statusService) GetPARScope() remote.PARScopeObservation { return s.scope }
func (s *statusService) SetPARDiscoveryDemand(v bool)            { s.wanted = v }

func TestReaderSeparatesLastRCStatusAndPARScope(t *testing.T) {
	t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "false")
	cfg := configmock.New(t)
	for k, v := range map[string]any{"api_key": "key", "site": "datadoghq.com", "dd_url": "https://api.datadoghq.com", "remote_configuration.rc_dd_url": "https://config.datadoghq.com", "remote_configuration.enabled": true} {
		cfg.SetInTest(k, v)
	}
	now := time.Now()
	service := &statusService{}
	source := NewSource(cfg, service)
	require.Equal(t, "rc_unchecked", source.Read(now).RCReason)
	for _, enabled := range []bool{false, true} {
		for _, authorized := range []bool{false, true} {
			t.Run(fmt.Sprintf("%t_%t", enabled, authorized), func(t *testing.T) {
				service.rc = &pbgo.OrgStatusResponse{Enabled: enabled, Authorized: authorized}
				service.scope = remote.PARScopeObservation{Reason: "scope_present", CheckedAt: now}
				p := source.Read(now)
				require.Equal(t, enabled && authorized, p.RCReady)
				require.Equal(t, enabled && authorized, p.ScopeReady)
			})
		}
	}
	p := source.Read(now.Add(time.Hour))
	require.True(t, p.RCReady, "RC returns its last success, without expiry")
	require.False(t, p.ScopeReady, "AAA grants still expire")
	service.scope = (&remote.CoreAgentService{}).GetPARScope()
	p = source.Read(now)
	require.True(t, p.RCReady)
	require.False(t, p.ScopeReady)
	require.Equal(t, "par_scope_unchecked", p.ScopeReason)
	for _, setting := range []string{"api_key", "remote_configuration.api_key", "remote_configuration.key", "remote_configuration.rc_dd_url"} {
		old := cfg.Get(setting)
		cfg.SetInTest(setting, "different")
		require.False(t, source.Read(now).RCReady, setting)
		cfg.SetInTest(setting, old)
	}
	cfg.SetInTest("remote_configuration.enabled", false)
	require.Equal(t, "rc_locally_disabled", source.Read(now).RCReason)
	cfg.SetInTest("remote_configuration.enabled", true)
	require.Equal(t, "rc_service_unavailable", NewSource(cfg, nil).Read(now).RCReason)
}

func TestReaderFreezesLoopbackEndpointsEvenForIdentityReuse(t *testing.T) {
	t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "true")
	cfg := configmock.New(t)
	for k, v := range map[string]any{"api_key": "key", "dd_url": "http://127.0.0.1:1234", "remote_configuration.rc_dd_url": "http://127.0.0.1:5678", "remote_configuration.enabled": true} {
		cfg.SetInTest(k, v)
	}
	now := time.Now()
	source := NewSource(cfg, &statusService{rc: &pbgo.OrgStatusResponse{Enabled: true, Authorized: true}})
	require.True(t, source.Read(now).RCReady)
	cfg.SetInTest("dd_url", "http://127.0.0.1:4321")
	require.Equal(t, "rc_credential_or_endpoint_mismatch", source.Read(now).RCReason)
}
