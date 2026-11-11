// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && zlib

package serializer

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	metricscompressionimpl "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	metricsserializer "github.com/DataDog/datadog-agent/pkg/serializer/internal/metrics"
)

func TestEndpointMetricRoutingPayloads(t *testing.T) {
	for _, version := range []string{"v2", "v3"} {
		for _, sketch := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/sketch=%t", version, sketch), func(t *testing.T) {
				const primary = "https://primary.example"
				const secondary = "https://secondary.example"
				const empty = "https://empty.example"
				const noMatch = "https://no-match.example"
				cfg := configmock.New(t)
				cfg.SetInTest("dd_url", primary)
				cfg.SetInTest("api_key", "fake-key")
				cfg.SetInTest("additional_endpoints", map[string][]string{secondary: {"fake-key"}, empty: {"fake-key"}, noMatch: {"fake-key"}})
				cfg.SetInTest("use_v3_api.series.enabled", strconv.FormatBool(version == "v3"))
				cfg.SetInTest("serializer_experimental_use_v3_api.sketches.endpoints", []string{})
				if version == "v3" {
					cfg.SetInTest("serializer_experimental_use_v3_api.sketches.endpoints", []string{primary, secondary, empty, noMatch})
				}
				cfg.SetInTest("serializer_experimental_use_v3_api.sketches.shadow_sample_rate", 0)
				cfg.SetInTest("serializer_compressor_kind", "zstd")
				cfg.SetInTest(configutils.MetricsEndpointRoutingKey+".metric_allowlist", map[string][]string{secondary: {"critical.slo", "normalized_metric", "literal-name"}, empty: {}, noMatch: {"never.sent"}})
				cfg.SetInTest(configutils.MetricsEndpointRoutingKey+".metric_prefix_allowlist", map[string][]string{secondary: {"billing."}})
				descriptors, err := configutils.GetMultipleEndpoints(cfg)
				require.NoError(t, err)
				resolvers, err := resolver.NewSingleDomainResolvers2(descriptors)
				require.NoError(t, err)
				var rs []resolver.DomainResolver
				for _, r := range resolvers {
					rs = append(rs, r)
				}
				f := &forwarder.MockedForwarder{}
				f.On("GetDomainResolvers").Return(rs)
				compressor := metricscompressionimpl.NewComponent(metricscompressionimpl.Requires{Cfg: cfg}).Comp
				routing, err := configutils.ReadMetricsEndpointRouting(cfg)
				require.NoError(t, err)
				s := NewSerializerWithEndpointRouting(f, nil, compressor, cfg, logmock.New(t), "host", routing)
				// Config changes must not affect an existing serializer.
				cfg.SetInTest(configutils.MetricsEndpointRoutingKey+".metric_allowlist", map[string][]string{})
				cfg.SetInTest(configutils.MetricsEndpointRoutingKey+".metric_prefix_allowlist", map[string][]string{})
				received := map[string][]byte{}
				f.On("SubmitTransaction", mock.Anything).Run(func(args mock.Arguments) {
					tx := args.Get(0).(*transaction.HTTPTransaction)
					payload, err := s.Strategy.Decompress(tx.Payload.GetContent())
					require.NoError(t, err)
					name := tx.Resolver.(resolver.DomainResolver).GetConfigName()
					received[name] = append(received[name], payload...)
					kind := metricsKindSeries
					if sketch {
						kind = metricsKindSketches
					}
					require.Equal(t, metricsEndpointFor(kind, version == "v3"), tx.Endpoint)
					require.Equal(t, "application/x-protobuf", tx.Headers.Get("Content-Type"))
				}).Return(nil)
				allNames := []string{"critical.slo", "billing.latency", "normalized-metric", "other.metric", "literal-name"}
				if sketch {
					source := metrics.NewSketchesSourceTest()
					for i, name := range allNames {
						ss := metricsserializer.Makeseries(i)
						ss.Name = name
						source.Append(ss)
					}
					err = s.SendSketch(source)
				} else {
					var series metrics.Series
					for _, name := range allNames {
						series = append(series, &metrics.Serie{Name: name, MType: metrics.APIGaugeType, Points: []metrics.Point{{Ts: 1759241515, Value: 2}}, Host: "host"})
					}
					err = s.SendIterableSeries(metricsserializer.CreateSerieSource(series))
				}
				require.NoError(t, err)
				// Fixture names occur only in metric-name fields.
				for i, name := range allNames {
					require.Contains(t, string(received[primary]), name)
					require.NotContains(t, string(received[noMatch]), name)
					if i < 3 {
						require.Contains(t, string(received[secondary]), name)
					} else {
						require.NotContains(t, string(received[secondary]), name)
					}
				}
				require.NotContains(t, received, empty)
				f.AssertExpectations(t)
			})
		}
	}
}
