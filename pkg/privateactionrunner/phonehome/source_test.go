// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package phonehome

import (
	"fmt"
	phonehomedef "github.com/DataDog/datadog-agent/comp/privateactionrunner/phonehome/def"
	"testing"
	"time"

	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/stretchr/testify/require"
)

type statusService struct {
	rcservice.Component
	rc     *pbgo.OrgStatusResponse
	onRead func()
}

func (s *statusService) GetOrgStatus() *pbgo.OrgStatusResponse {
	if s.onRead != nil {
		s.onRead()
	}
	return s.rc
}

func TestReaderRetainsLastSuccessfulRCStatus(t *testing.T) {
	t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "false")
	cfg := configmock.New(t)
	for k, v := range map[string]interface{}{"api_key": "key", "site": "datadoghq.com", "dd_url": "", "remote_configuration.enabled": true, "private_action_runner.enabled": true} {
		cfg.SetInTest(k, v)
	}
	rc := &statusService{}
	source := NewSource(cfg, rc)
	require.Equal(t, phonehomedef.ReasonRCUnchecked, source.Read(time.Now()).Reason)
	rc.rc = &pbgo.OrgStatusResponse{Enabled: true, Authorized: true}
	require.True(t, source.Read(time.Now().Add(time.Hour)).Ready, "RC last success has no expiry")
	for _, setting := range []string{"api_key", "remote_configuration.api_key", "remote_configuration.key", "remote_configuration.rc_dd_url", "private_action_runner.par_dd_url"} {
		old := cfg.Get(setting)
		cfg.SetInTest(setting, "different")
		require.False(t, source.Read(time.Now()).Ready)
		cfg.SetInTest(setting, old)
	}
	cfg.SetInTest("remote_configuration.enabled", false)
	require.Equal(t, phonehomedef.ReasonRCLocallyDisabled, source.Read(time.Now()).Reason)
	cfg.SetInTest("remote_configuration.enabled", true)
	require.Equal(t, phonehomedef.ReasonRCServiceUnavailable, NewSource(cfg, nil).Read(time.Now()).Reason)
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
				cfg.SetInTest("private_action_runner.enabled", true)
				rc := &statusService{rc: &pbgo.OrgStatusResponse{Enabled: true, Authorized: true}}
				for _, endpoint := range []string{"", "https://config." + site, "https://config." + site + "."} {
					cfg.SetInTest("remote_configuration.rc_dd_url", endpoint)
					require.True(t, NewSource(cfg, rc).Read(time.Now()).Ready, endpoint)
					require.Equal(t, "https://api."+site, enrollmentEndpoint(cfg))
				}
				for _, endpoint := range []string{"https://rc-proxy.example.com:8443/prefix", "https://rc-proxy.example.com:8443/prefix/", "https://config.us5.datadoghq.com."} {
					cfg.SetInTest("remote_configuration.rc_dd_url", endpoint)
					cfg.SetInTest("private_action_runner.par_dd_url", "")
					require.True(t, NewSource(cfg, rc).Read(time.Now()).Ready, "custom RC routing alone must not block PAR")
					require.Equal(t, "https://api."+site, enrollmentEndpoint(cfg))
					cfg.SetInTest("private_action_runner.par_dd_url", "https://par-proxy.example.com/api-prefix")
					require.True(t, NewSource(cfg, rc).Read(time.Now()).Ready, endpoint)
					require.Equal(t, "https://par-proxy.example.com/api-prefix", enrollmentEndpoint(cfg))
				}
			})
		}
	}
}

func TestReaderRejectsInvalidPAROverride(t *testing.T) {
	for _, endpoint := range []string{"ftp://proxy.example.com", "https://user:secret@proxy.example.com", "https://proxy.example.com?query", "https://proxy.example.com:65536", "https://[::1]:99999"} {
		cfg := configmock.New(t)
		cfg.SetInTest("api_key", "key")
		cfg.SetInTest("remote_configuration.enabled", true)
		cfg.SetInTest("private_action_runner.par_dd_url", endpoint)
		rc := &statusService{rc: &pbgo.OrgStatusResponse{Enabled: true, Authorized: true}}
		require.Equal(t, phonehomedef.ReasonInvalidEndpoint, NewSource(cfg, rc).Read(time.Now()).Reason)
	}
}

func TestReaderIgnoresMetricsEndpoint(t *testing.T) {
	t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "false")
	for _, endpoint := range []string{"https://app.us3.datadoghq.com", "https://metrics-proxy.example.com"} {
		cfg := configmock.New(t)
		cfg.SetInTest("api_key", "key")
		cfg.SetInTest("site", "datadoghq.eu")
		cfg.SetInTest("dd_url", endpoint)
		cfg.SetInTest("remote_configuration.enabled", true)
		rc := &statusService{rc: &pbgo.OrgStatusResponse{Enabled: true, Authorized: true}}
		source := NewSource(cfg, rc)
		require.True(t, source.Read(time.Now()).Ready)
		require.Equal(t, "https://api.datadoghq.eu", enrollmentEndpoint(cfg))
		cfg.SetInTest("dd_url", "https://another-metrics-proxy.example.com")
		require.True(t, source.Read(time.Now()).Ready, "metrics routing changes must not affect PAR readiness")
	}
}

func TestReaderFreezesLoopbackEndpoints(t *testing.T) {
	t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "true")
	cfg := configmock.New(t)
	for k, v := range map[string]interface{}{"api_key": "key", "dd_url": "http://127.0.0.1:1234", "remote_configuration.rc_dd_url": "http://127.0.0.1:5678", "remote_configuration.enabled": true} {
		cfg.SetInTest(k, v)
	}
	source := NewSource(cfg, &statusService{rc: &pbgo.OrgStatusResponse{Enabled: true, Authorized: true}})
	require.True(t, source.Read(time.Now()).Ready)
	cfg.SetInTest("dd_url", "http://127.0.0.1:4321")
	require.Equal(t, phonehomedef.ReasonRCCredentialOrEndpointMismatch, source.Read(time.Now()).Reason)
}
