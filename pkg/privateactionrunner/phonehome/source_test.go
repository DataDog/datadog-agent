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
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/stretchr/testify/require"
)

type statusService struct {
	rcservice.Component
	rc *pbgo.OrgStatusResponse
}

func (s *statusService) GetOrgStatus() *pbgo.OrgStatusResponse { return s.rc }

type scopeService struct {
	scope  PARScopeObservation
	wanted bool
}

func (s *scopeService) GetPARScope() PARScopeObservation { return s.scope }
func (s *scopeService) SetPARDiscoveryDemand(v bool)     { s.wanted = v }

func TestReaderSeparatesLastRCStatusAndPARScope(t *testing.T) {
	t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "false")
	cfg := configmock.New(t)
	for k, v := range map[string]any{"api_key": "key", "site": "datadoghq.com", "dd_url": "https://api.datadoghq.com", "remote_configuration.rc_dd_url": "https://config.datadoghq.com", "remote_configuration.enabled": true} {
		cfg.SetInTest(k, v)
	}
	now := time.Now()
	service := &statusService{}
	scope := &scopeService{}
	source := NewSource(cfg, service, scope)
	require.Equal(t, "rc_unchecked", source.Read(now).RCReason)
	for _, enabled := range []bool{false, true} {
		for _, authorized := range []bool{false, true} {
			t.Run(fmt.Sprintf("%t_%t", enabled, authorized), func(t *testing.T) {
				service.rc = &pbgo.OrgStatusResponse{Enabled: enabled, Authorized: authorized}
				scope.scope = PARScopeObservation{Reason: "scope_present", CheckedAt: now}
				p := source.Read(now)
				require.Equal(t, enabled && authorized, p.RCReady)
				require.Equal(t, enabled && authorized, p.ScopeReady)
			})
		}
	}
	p := source.Read(now.Add(time.Hour))
	require.True(t, p.RCReady, "RC returns its last success, without expiry")
	require.False(t, p.ScopeReady, "AAA grants still expire")
	scope.scope = PARScopeObservation{}
	require.False(t, NewSource(cfg, service, nil).Read(now).ScopeReady)
	source.Demand(true)
	require.True(t, scope.wanted)
	source.Demand(false)
	require.False(t, scope.wanted)
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
	require.Equal(t, "rc_service_unavailable", NewSource(cfg, nil, nil).Read(now).RCReason)
}

func TestReaderRCEndpointFQDN(t *testing.T) {
	t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "false")
	for _, site := range []string{"datadoghq.com", "datadoghq.eu", "us3.datadoghq.com"} {
		for _, fqdn := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fqdn=%t", site, fqdn), func(t *testing.T) {
				cfg := configmock.New(t)
				cfg.SetInTest("api_key", "key")
				cfg.SetInTest("site", site)
				cfg.SetInTest("dd_url", "")
				cfg.SetInTest("convert_dd_site_fqdn.enabled", fqdn)
				cfg.SetInTest("remote_configuration.enabled", true)
				rc := &statusService{rc: &pbgo.OrgStatusResponse{Enabled: true, Authorized: true}}
				for _, endpoint := range []string{"", "https://config." + site, "https://config." + site + "."} {
					cfg.SetInTest("remote_configuration.rc_dd_url", endpoint)
					require.True(t, NewSource(cfg, rc, nil).Read(time.Now()).RCReady, endpoint)
				}
				for _, endpoint := range []string{"http://config." + site + ".", "https://config." + site + ".invalid", "https://config." + site + "/.", "https://config." + site + ".:443", "https://config." + site + "..", "https://config.us5.datadoghq.com."} {
					cfg.SetInTest("remote_configuration.rc_dd_url", endpoint)
					require.Equal(t, "rc_credential_or_endpoint_mismatch", NewSource(cfg, rc, nil).Read(time.Now()).RCReason, endpoint)
				}
			})
		}
	}
}

func TestReaderFreezesLoopbackEndpointsEvenForIdentityReuse(t *testing.T) {
	t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "true")
	cfg := configmock.New(t)
	for k, v := range map[string]any{"api_key": "key", "dd_url": "http://127.0.0.1:1234", "remote_configuration.rc_dd_url": "http://127.0.0.1:5678", "remote_configuration.enabled": true} {
		cfg.SetInTest(k, v)
	}
	now := time.Now()
	source := NewSource(cfg, &statusService{rc: &pbgo.OrgStatusResponse{Enabled: true, Authorized: true}}, nil)
	require.True(t, source.Read(now).RCReady)
	cfg.SetInTest("dd_url", "http://127.0.0.1:4321")
	require.Equal(t, "rc_credential_or_endpoint_mismatch", source.Read(now).RCReason)
}
