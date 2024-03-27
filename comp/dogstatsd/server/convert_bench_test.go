// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"go.uber.org/fx"
)

func buildRawSample(tagCount int, multipleValues bool) []byte {
	tags := "tag0:val0"
	for i := 1; i < tagCount; i++ {
		tags += fmt.Sprintf(",tag%d:val%d", i, i)
	}

	if multipleValues {
		return []byte(fmt.Sprintf("daemon:666:777|h|@0.5|#%s", tags))
	}
	return []byte(fmt.Sprintf("daemon:666|h|@0.5|#%s", tags))
}

// used to store the result and avoid optimizations
var (
	benchSamples []metrics.MetricSample
)

func runParseMetricBenchmark(b *testing.B, multipleValues bool) {
	deps := newServerDeps(b)
	parser := newParser(deps.Config, newFloat64ListPool(), 1, deps.WMeta)

	conf := enrichConfig{
		defaultHostname:           "default-hostname",
		entityIDPrecedenceEnabled: true,
	}

	for i := 1; i < 1000; i *= 4 {
		b.Run(fmt.Sprintf("%d-tags", i), func(sb *testing.B) {
			rawSample := buildRawSample(i, multipleValues)
			sb.ResetTimer()
			samples := make([]metrics.MetricSample, 0, 2)

			for n := 0; n < sb.N; n++ {

				parsed, err := parser.parseMetricSample(rawSample)
				if err != nil {
					continue
				}

				benchSamples = enrichMetricSample(samples, parsed, "", "", conf)
			}
		})
	}
}

func BenchmarkParseMetric(b *testing.B) {
	runParseMetricBenchmark(b, false)
}

func BenchmarkParseMultipleMetric(b *testing.B) {
	runParseMetricBenchmark(b, true)
}

type ServerDeps struct {
	fx.In
	Config config.Component
	WMeta  optional.Option[workloadmeta.Component]
}

func newServerDeps(t testing.TB, options ...fx.Option) ServerDeps {
	return fxutil.Test[ServerDeps](t, core.MockBundle(), workloadmeta.MockModule(), fx.Supply(workloadmeta.NewParams()), fx.Options(options...))
}
