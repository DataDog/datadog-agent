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
	for k, v := range map[string]interface{}{"api_key": "key", "site": "datadoghq.com", "dd_url": "", "remote_configuration.enabled": true} {
		cfg.SetInTest(k, v)
	}
	rc := &statusService{}
	source := NewSource(cfg, rc)
	require.Equal(t, "rc_unchecked", source.Read(time.Now()).Reason)
	rc.rc = &pbgo.OrgStatusResponse{Enabled: true, Authorized: true}
	require.True(t, source.Read(time.Now().Add(time.Hour)).Ready, "RC last success has no expiry")
	for _, setting := range []string{"api_key", "remote_configuration.api_key", "remote_configuration.key", "remote_configuration.rc_dd_url"} {
		old := cfg.Get(setting)
		cfg.SetInTest(setting, "different")
		require.False(t, source.Read(time.Now()).Ready)
		cfg.SetInTest(setting, old)
	}
	cfg.SetInTest("remote_configuration.enabled", false)
	require.Equal(t, "rc_locally_disabled", source.Read(time.Now()).Reason)
	cfg.SetInTest("remote_configuration.enabled", true)
	require.Equal(t, "rc_service_unavailable", NewSource(cfg, nil).Read(time.Now()).Reason)
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
					require.True(t, NewSource(cfg, rc).Read(time.Now()).Ready, endpoint)
				}
				for _, endpoint := range []string{"http://config." + site + ".", "https://config." + site + ".invalid", "https://config." + site + "/.", "https://config." + site + ".:443", "https://config." + site + "..", "https://config.us5.datadoghq.com."} {
					cfg.SetInTest("remote_configuration.rc_dd_url", endpoint)
					require.Equal(t, "rc_credential_or_endpoint_mismatch", NewSource(cfg, rc).Read(time.Now()).Reason, endpoint)
				}
			})
		}
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
	require.Equal(t, "rc_credential_or_endpoint_mismatch", source.Read(time.Now()).Reason)
}
