// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter"

import (
	"context"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type traceExporter struct {
	params exporter.CreateSettings
	cfg    *Config
	ctx    context.Context // ctx triggers shutdown upon cancellation
	agent  *agent.Agent    // agent processes incoming traces
}

func newTracesExporter(
	ctx context.Context,
	params exporter.CreateSettings,
	cfg *Config,
	agent *agent.Agent,
) *traceExporter {
	return &traceExporter{
		params: params,
		cfg:    cfg,
		ctx:    ctx,
		agent:  agent,
	}
}

var _ consumer.ConsumeTracesFunc = (*traceExporter)(nil).consumeTraces

// headerComputedStats specifies the HTTP header which indicates whether APM stats
// have already been computed for a payload.
const headerComputedStats = "Datadog-Client-Computed-Stats"

func (exp *traceExporter) consumeTraces(
	ctx context.Context,
	td ptrace.Traces,
) (err error) {
	if exp.cfg.HostMetadata.Enabled {
		exp.params.Logger.Error("datadog::host_metadata::enabled should not be set in OTel Agent")
	}
	rspans := td.ResourceSpans()
	hosts := make(map[string]struct{})
	tags := make(map[string]struct{})
	header := make(http.Header)
	header[headerComputedStats] = []string{"true"}
	for i := 0; i < rspans.Len(); i++ {
		rspan := rspans.At(i)
		src := exp.agent.OTLPReceiver.ReceiveResourceSpans(ctx, rspan, header)
		switch src.Kind {
		case source.HostnameKind:
			hosts[src.Identifier] = struct{}{}
		case source.AWSECSFargateKind:
			tags[src.Tag()] = struct{}{}
		case source.InvalidKind:
		}
	}

	return nil
}
