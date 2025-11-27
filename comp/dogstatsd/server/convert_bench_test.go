// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"fmt"
	"strings"
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func buildRawSample(tagCount int, multipleValues bool) []byte {
	var builder strings.Builder
	builder.WriteString("tag0:val0")
	for i := 1; i < tagCount; i++ {
		fmt.Fprintf(&builder, ",tag%d:val%d", i, i)
	}
	tags := builder.String()

	if multipleValues {
		return []byte("daemon:666:777|h|@0.5|#" + tags)
	}
	return []byte("daemon:666|h|@0.5|#" + tags)
}

// used to store the result and avoid optimizations
var (
	benchSamples []metrics.MetricSample
)

func runParseMetricBenchmark(b *testing.B, multipleValues bool) {
	deps := newServerDeps(b)
	stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
	parser := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)

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

				benchSamples = enrichMetricSample(samples, parsed, "", 0, "", conf, nil)
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
	Config    config.Component
	WMeta     option.Option[workloadmeta.Component]
	Telemetry telemetry.Component
}

func newServerDeps(t testing.TB) ServerDeps {
	return fxutil.Test[ServerDeps](t,
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		fx.Provide(func(t testing.TB) config.Component { return config.NewMock(t) }),
		telemetryimpl.MockModule(),
		hostnameimpl.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)
}
