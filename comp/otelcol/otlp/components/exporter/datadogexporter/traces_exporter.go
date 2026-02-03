// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter"

import (
	"context"
	"net/http"

	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	"github.com/DataDog/datadog-agent/pkg/util/otel"

	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type traceExporter struct {
	params            exporter.Settings
	cfg               *datadogconfig.Config
	ctx               context.Context      // ctx triggers shutdown upon cancellation
	traceagentcmp     traceagent.Component // agent processes incoming traces
	gatewayUsage      otel.GatewayUsage
	coatTracesMetric  telemetry.Gauge
	coatGWUsageMetric telemetry.Gauge
	reporter          *inframetadata.Reporter // reports host metadata from resource attributes and metrics
}

func newTracesExporter(
	ctx context.Context,
	params exporter.Settings,
	cfg *datadogconfig.Config,
	traceagentcmp traceagent.Component,
	gatewayUsage otel.GatewayUsage,
	coatTracesMetric telemetry.Gauge,
	coatGWUsageMetric telemetry.Gauge,
	reporter *inframetadata.Reporter,
) *traceExporter {
	exp := &traceExporter{
		params:            params,
		cfg:               cfg,
		ctx:               ctx,
		traceagentcmp:     traceagentcmp,
		gatewayUsage:      gatewayUsage,
		coatTracesMetric:  coatTracesMetric,
		coatGWUsageMetric: coatGWUsageMetric,
		reporter:          reporter,
	}
	return exp
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
	OTLPIngestDDOTTracesRequests.Inc()
	OTLPIngestDDOTTracesEvents.Add(float64(td.SpanCount()))
	rspans := td.ResourceSpans()
	hosts := make(map[string]struct{})
	ecsFargateArns := make(map[string]struct{})
	header := make(http.Header)
	header[headerComputedStats] = []string{"true"}
	for i := 0; i < rspans.Len(); i++ {
		rspan := rspans.At(i)
		res := rspan.Resource()
		if exp.cfg.HostMetadata.Enabled && exp.reporter != nil {
			err := exp.reporter.ConsumeResource(res)
			if err != nil {
				exp.params.Logger.Warn("failed to consume resource for host metadata", zap.Error(err), zap.Any("resource", res))
			}
		}
		src, err := exp.traceagentcmp.ReceiveOTLPSpans(ctx, rspan, header, exp.gatewayUsage.GetHostFromAttributesHandler())
		if err != nil {
			return err
		}
		switch src.Kind {
		case source.HostnameKind:
			hosts[src.Identifier] = struct{}{}
		case source.AWSECSFargateKind:
			ecsFargateArns[src.Identifier] = struct{}{}
		case source.InvalidKind:
		}
	}
	exp.exportUsageMetrics(hosts, ecsFargateArns)
	return nil
}

// exportUsageMetrics exports usage tracking metrics on traces in DDOT
func (exp *traceExporter) exportUsageMetrics(hosts map[string]struct{}, ecsFargateArns map[string]struct{}) {
	buildInfo := exp.params.BuildInfo

	if exp.coatTracesMetric != nil {
		for host := range hosts {
			exp.coatTracesMetric.Set(1.0, buildInfo.Version, buildInfo.Command, host, "")
		}
		for taskArn := range ecsFargateArns {
			exp.coatTracesMetric.Set(1.0, buildInfo.Version, buildInfo.Command, "", taskArn)
		}
	}

	if exp.coatGWUsageMetric != nil {
		value, _ := exp.gatewayUsage.Gauge()
		exp.coatGWUsageMetric.Set(value, buildInfo.Version, buildInfo.Command)
	}
}
