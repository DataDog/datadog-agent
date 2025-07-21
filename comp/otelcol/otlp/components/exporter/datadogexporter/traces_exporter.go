// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter"

import (
	"context"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	"github.com/DataDog/datadog-agent/pkg/util/otel"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type traceExporter struct {
	params        exporter.Settings
	cfg           *datadogconfig.Config
	ctx           context.Context      // ctx triggers shutdown upon cancellation
	traceagentcmp traceagent.Component // agent processes incoming traces
	gatewayUsage  otel.GatewayUsage
	usageMetric   telemetry.Gauge
}

func newTracesExporter(
	ctx context.Context,
	params exporter.Settings,
	cfg *datadogconfig.Config,
	traceagentcmp traceagent.Component,
	gatewayUsage otel.GatewayUsage,
	usageMetric telemetry.Gauge,
) *traceExporter {
	exp := &traceExporter{
		params:        params,
		cfg:           cfg,
		ctx:           ctx,
		traceagentcmp: traceagentcmp,
		gatewayUsage:  gatewayUsage,
		usageMetric:   usageMetric,
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
	rspans := td.ResourceSpans()
	hosts := make(map[string]struct{})
	ecsFargateArns := make(map[string]struct{})
	header := make(http.Header)
	header[headerComputedStats] = []string{"true"}
	for i := 0; i < rspans.Len(); i++ {
		rspan := rspans.At(i)
		src := exp.traceagentcmp.ReceiveOTLPSpans(ctx, rspan, header, exp.gatewayUsage.GetHostFromAttributesHandler())
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
	if exp.usageMetric == nil {
		return
	}
	buildInfo := exp.params.BuildInfo
	for host := range hosts {
		exp.usageMetric.Set(1.0, buildInfo.Version, buildInfo.Command, host, "")
	}
	for taskArn := range ecsFargateArns {
		exp.usageMetric.Set(1.0, buildInfo.Version, buildInfo.Command, "", taskArn)
	}
}
