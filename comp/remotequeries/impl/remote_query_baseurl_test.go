// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

const remoteQueryTestSite = "datad0g.com"

func remoteQueryTestConfig(t testing.TB) config.Component {
	t.Helper()
	cfg := configmock.New(t)
	cfg.SetInTest("site", remoteQueryTestSite)
	return cfg
}

// A request has no destination. The core Agent adds the site's public intake URL
// to the unchanged Python bridge contract, independently of metrics routing.
func TestExecuteStreamResolvesResultDeliveryBaseURLFromLocalSite(t *testing.T) {
	for _, tc := range []struct {
		name string
		site string
		want string
	}{
		{name: "staging", site: "datad0g.com", want: "https://api.datad0g.com/api/unstable/its-agent-intake"},
		{name: "production", site: "datadoghq.com", want: "https://api.datadoghq.com/api/unstable/its-agent-intake"},
		{name: "EU", site: "datadoghq.eu", want: "https://api.datadoghq.eu/api/unstable/its-agent-intake"},
		{name: "US3", site: "us3.datadoghq.com", want: "https://api.us3.datadoghq.com/api/unstable/its-agent-intake"},
		{name: "US7", site: "us7.datadoghq.com", want: "https://api.us7.datadoghq.com/api/unstable/its-agent-intake"},
		{name: "US8", site: "us8.datadoghq.com", want: "https://api.us8.datadoghq.com/api/unstable/its-agent-intake"},
		{name: "normalized", site: "  AP1.Datadoghq.com ", want: "https://api.ap1.datadoghq.com/api/unstable/its-agent-intake"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := newSyntheticForwardRunner(nil)
			cfg := configmock.New(t)
			cfg.SetInTest("site", tc.site)
			cfg.SetInTest("dd_url", "https://metrics.example.invalid")
			service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, cfg)
			req, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, "SELECT 1", false, syntheticForwardDelivery())
			require.NoError(t, err)

			result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

			require.Nil(t, result.Error)
			var wire remoteQueryProduceJSONPagesRequestJSON
			require.NoError(t, json.Unmarshal([]byte(runner.streamSeen), &wire))
			require.NotNil(t, wire.ResultDelivery)
			assert.Equal(t, tc.want, wire.ResultDelivery.BaseURL)
			assert.Equal(t, syntheticForwardUploadID, wire.ResultDelivery.UploadID)
		})
	}
}

func TestResultDeliveryBaseURLDefaultAndMissingConfig(t *testing.T) {
	baseURL, err := NewRemoteQueryExecuteService(nil, true, configmock.New(t)).resultDeliveryBaseURL()
	require.NoError(t, err)
	assert.Equal(t, "https://api.datadoghq.com/api/unstable/its-agent-intake", baseURL)
	_, err = NewRemoteQueryExecuteService(nil, true, nil).resultDeliveryBaseURL()
	require.Error(t, err)
	_, err = (*RemoteQueryExecuteService)(nil).resultDeliveryBaseURL()
	require.Error(t, err)
}

func TestExecuteStreamRejectsUnsupportedLocalSiteBeforeDispatch(t *testing.T) {
	for _, site := range []string{"", " ", "evil.com", "datadoghq.com.evil.com", "datadoghq.com/path", "datadoghq.com:443", "https://datadoghq.com"} {
		t.Run(site, func(t *testing.T) {
			runner := newSyntheticForwardRunner(nil)
			cfg := configmock.New(t)
			cfg.SetInTest("site", site)
			service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{runner}}, true, cfg)
			req, err := NewRemoteQueryExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, "SELECT 1", false, syntheticForwardDelivery())
			require.NoError(t, err)

			result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

			require.NotNil(t, result.Error)
			assert.Equal(t, statusExecutorUnavailable, result.Error.Code)
			assert.Equal(t, 0, runner.streamCalls)
			assert.Equal(t, 0, runner.resolveCalls)
		})
	}
}
