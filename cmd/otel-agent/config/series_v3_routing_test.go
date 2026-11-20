// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && zlib

package config

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	forwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	metricscompressionimpl "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// TestDDOTSeriesV3Routing verifies the intake the serializer picks for series under the
// DDOT config: v3 for the exporter's Datadog endpoint (api.<site>) with or without a proxy,
// v2 for non-Datadog destinations and when use_v3_api.series.enabled is false.
func TestDDOTSeriesV3Routing(t *testing.T) {
	tests := []struct {
		name   string
		config string
		env    map[string]string
		proxy  bool
		want   transaction.Endpoint
	}{
		{name: "default endpoint", config: "testdata/config_default.yaml", want: endpoints.V3SeriesEndpoint},
		{name: "DD_PROXY_HTTPS", config: "testdata/config_default.yaml", env: map[string]string{"DD_PROXY_HTTPS": "http://proxy.example.test:3128"}, proxy: true, want: endpoints.V3SeriesEndpoint},
		{name: "HTTPS_PROXY", config: "testdata/config_default.yaml", env: map[string]string{"HTTPS_PROXY": "http://proxy.example.test:3128"}, proxy: true, want: endpoints.V3SeriesEndpoint},
		{name: "exporter proxy_url", config: "testdata/config_proxy.yaml", proxy: true, want: endpoints.V3SeriesEndpoint},
		{name: "custom endpoint", config: "testdata/config_custom_metrics_endpoint.yaml", want: endpoints.SeriesEndpoint},
		{name: "non-Datadog site", config: "testdata/config_non_datadog_site.yaml", want: endpoints.SeriesEndpoint},
		{name: "v3 disabled", config: "testdata/config_default.yaml", env: map[string]string{"DD_USE_V3_API_SERIES_ENABLED": "false"}, want: endpoints.SeriesEndpoint},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configmock.New(t)
			// CI runners set HTTP(S)_PROXY: start every case without a proxy.
			unsetProxyEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			c, err := NewConfigComponent(context.Background(), "", []string{tt.config})
			require.NoError(t, err)

			require.Equal(t, tt.proxy, c.GetString("proxy.https") != "", "proxy.https = %q", c.GetString("proxy.https"))
			assert.Equal(t, []transaction.Endpoint{tt.want}, sentSeriesEndpoints(t, c))
		})
	}
}

// unsetProxyEnv unsets the proxy env vars for the test; LoadProxyFromEnv treats an empty one as set.
func unsetProxyEnv(t *testing.T) {
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy", "DD_PROXY_HTTP", "DD_PROXY_HTTPS", "DD_PROXY_NO_PROXY"} {
		t.Setenv(k, "") // restores the original value after the test
		os.Unsetenv(k)
	}
}

// sentSeriesEndpoints sends one series through a serializer built from c and returns
// the endpoints of the transactions it submits.
func sentSeriesEndpoints(t *testing.T, c coreconfig.Component) []transaction.Endpoint {
	descriptors, err := configutils.GetMultipleEndpoints(c)
	require.NoError(t, err)
	resolvers, err := resolver.NewSingleDomainResolvers2(descriptors)
	require.NoError(t, err)
	var rs []resolver.DomainResolver
	for _, r := range resolvers {
		rs = append(rs, r)
	}

	var sent []transaction.Endpoint
	f := &forwarderimpl.MockedForwarder{}
	f.On("GetDomainResolvers").Return(rs)
	f.On("SubmitTransaction", mock.Anything).Run(func(args mock.Arguments) {
		sent = append(sent, args.Get(0).(*transaction.HTTPTransaction).Endpoint)
	}).Return(nil)

	compressor := metricscompressionimpl.NewComponent(metricscompressionimpl.Requires{Cfg: c}).Comp
	// zstd is built only with the zlib tag; without it the compressor is a noop.
	require.Equal(t, compression.ZstdEncoding, compressor.ContentEncoding(), "DDOT compresses metrics with zstd")
	s := serializer.NewSerializer(f, nil, compressor, c, logmock.New(t), "test-host")
	serie := &metrics.Serie{Name: "test.metric", MType: metrics.APIGaugeType, Points: []metrics.Point{{Ts: 1759241515, Value: 1}}, Host: "test-host"}
	require.NoError(t, s.SendIterableSeries(&serieSource{series: []*metrics.Serie{serie}}))
	return sent
}

type serieSource struct {
	series []*metrics.Serie
	pos    int
}

func (s *serieSource) MoveNext() bool          { s.pos++; return s.pos <= len(s.series) }
func (s *serieSource) Current() *metrics.Serie { return s.series[s.pos-1] }
func (s *serieSource) Count() uint64           { return uint64(len(s.series)) }
