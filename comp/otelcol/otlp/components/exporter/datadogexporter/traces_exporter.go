// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter"

import (
	"context"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
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
	rspans := td.ResourceSpans()
	header := make(http.Header)
	header[headerComputedStats] = []string{"true"}
	for i := 0; i < rspans.Len(); i++ {
		rspan := rspans.At(i)
		exp.agent.OTLPReceiver.ReceiveResourceSpans(ctx, rspan, header)
	}

	return nil
}
