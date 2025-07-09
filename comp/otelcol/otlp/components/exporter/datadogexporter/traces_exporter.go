// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter"

import (
	"context"
	"net/http"
	"time"

	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/otel"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

type traceExporter struct {
	params        exporter.Settings
	cfg           *datadogconfig.Config
	ctx           context.Context      // ctx triggers shutdown upon cancellation
	traceagentcmp traceagent.Component // agent processes incoming traces
	gatewayUsage  otel.GatewayUsage
	s             serializer.MetricSerializer
}

func newTracesExporter(
	ctx context.Context,
	params exporter.Settings,
	cfg *datadogconfig.Config,
	traceagentcmp traceagent.Component,
	gatewayUsage otel.GatewayUsage,
	s serializer.MetricSerializer,
) *traceExporter {
	return &traceExporter{
		params:        params,
		cfg:           cfg,
		ctx:           ctx,
		traceagentcmp: traceagentcmp,
		gatewayUsage:  gatewayUsage,
		s:             s,
	}
}

var _ consumer.ConsumeTracesFunc = (*traceExporter)(nil).consumeTraces

// headerComputedStats specifies the HTTP header which indicates whether APM stats
// have already been computed for a payload.
const headerComputedStats = "Datadog-Client-Computed-Stats"

// consumeTraces implements the consumer.ConsumeTracesFunc interface
func (exp *traceExporter) consumeTraces(
	ctx context.Context,
	td ptrace.Traces,
) (err error) {
	rspans := td.ResourceSpans()
	hosts := make(map[string]struct{})
	ecsFargateTags := make(map[string]struct{})
	header := make(http.Header)
	header[headerComputedStats] = []string{"true"}
	for i := 0; i < rspans.Len(); i++ {
		rspan := rspans.At(i)
		src := exp.traceagentcmp.ReceiveOTLPSpans(ctx, rspan, header, exp.gatewayUsage.GetHostFromAttributesHandler())
		switch src.Kind {
		case source.HostnameKind:
			hosts[src.Identifier] = struct{}{}
		case source.AWSECSFargateKind:
			ecsFargateTags[src.Tag()] = struct{}{}
		case source.InvalidKind:
		}
	}

	exp.exportUsageMetrics(ctx, hosts, ecsFargateTags)
	return nil
}

func (exp *traceExporter) exportUsageMetrics(ctx context.Context, hosts map[string]struct{}, ecsFargateTags map[string]struct{}) {
	if exp.s == nil {
		return
	}

	var buildTags []string
	if exp.params.BuildInfo.Version != "" {
		buildTags = append(buildTags, "version:"+exp.params.BuildInfo.Version)
	}
	if exp.params.BuildInfo.Command != "" {
		buildTags = append(buildTags, "command:"+exp.params.BuildInfo.Command)
	}

	series := make(metrics.Series, 0, len(hosts)+len(ecsFargateTags))
	timestamp := float64(time.Now().Unix())
	for host := range hosts {
		series = append(series, &metrics.Serie{
			Name:           "datadog.agent.ddot.traces",
			Points:         []metrics.Point{{Value: 1, Ts: timestamp}},
			Tags:           tagset.CompositeTagsFromSlice(buildTags),
			Host:           host,
			MType:          metrics.APIGaugeType,
			SourceTypeName: "System",
			Source:         metrics.MetricSourceOpenTelemetryCollectorUnknown,
		})
	}
	for ecsFargateTag := range ecsFargateTags {
		series = append(series, &metrics.Serie{
			Name:           "datadog.agent.ddot.traces",
			Points:         []metrics.Point{{Value: 1, Ts: timestamp}},
			Tags:           tagset.CompositeTagsFromSlice(append(buildTags, ecsFargateTag)),
			Host:           "",
			MType:          metrics.APIGaugeType,
			SourceTypeName: "System",
			Source:         metrics.MetricSourceOpenTelemetryCollectorUnknown,
		})
	}

	var err error
	metrics.Serialize(
		metrics.NewIterableSeries(func(_ *metrics.Serie) {}, 200, 4000),
		metrics.NewIterableSketches(func(_ *metrics.SketchSeries) {}, 0, 0),
		func(seriesSink metrics.SerieSink, _ metrics.SketchesSink) {
			for _, serie := range series {
				seriesSink.Append(serie)
			}
		},
		func(serieSource metrics.SerieSource) {
			err = exp.s.SendIterableSeries(serieSource)
		},
		func(_ metrics.SketchesSource) {},
	)
	if err != nil {
		exp.params.Logger.Error("Error posting hostname/ECS Fargate tags series", zap.Error(err))
	}
}
