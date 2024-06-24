// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogexporter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient"
	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type testComponent struct {
	*pkgagent.Agent
}

func (c testComponent) SetOTelAttributeTranslator(attrstrans *attributes.Translator) {
	c.Agent.OTLPReceiver.SetOTelAttributeTranslator(attrstrans)
}

func (c testComponent) ReceiveOTLPSpans(ctx context.Context, rspans ptrace.ResourceSpans, httpHeader http.Header) source.Source {
	return c.Agent.OTLPReceiver.ReceiveResourceSpans(ctx, rspans, httpHeader)
}

func (c testComponent) SendStatsPayload(p *pb.StatsPayload) {
	c.Agent.StatsWriter.SendPayload(p)
}

var _ traceagent.Component = (*testComponent)(nil)

func TestTraceExporter(t *testing.T) {
	got := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", req.Header.Get("DD-Api-Key"))
		got <- req.Header.Get("Content-Type")
		rw.WriteHeader(http.StatusAccepted)
	}))

	defer server.Close()
	cfg := Config{
		API: APIConfig{
			Key: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		TagsConfig: TagsConfig{
			Hostname: "test-host",
		},
		Traces: TracesConfig{
			TCPAddrConfig: confignet.TCPAddrConfig{
				Endpoint: server.URL,
			},
			IgnoreResources: []string{},
		},
	}

	params := exportertest.NewNopSettings()
	tcfg := config.New()
	tcfg.TraceWriter.FlushPeriodSeconds = 0.1
	tcfg.Endpoints[0].APIKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tcfg.Endpoints[0].Host = server.URL
	ctx := context.Background()
	traceagent := pkgagent.NewAgent(ctx, tcfg, telemetry.NewNoopCollector(), &ddgostatsd.NoOpClient{})

	f := NewFactory(testComponent{traceagent}, nil, nil, nil, metricsclient.NewStatsdClientWrapper(&ddgostatsd.NoOpClient{}))
	exporter, err := f.CreateTracesExporter(ctx, params, &cfg)
	assert.NoError(t, err)

	go traceagent.Run()

	err = exporter.ConsumeTraces(ctx, simpleTraces())
	assert.NoError(t, err)
	timeout := time.After(2 * time.Second)
	select {
	case out := <-got:
		require.Equal(t, "application/x-protobuf", out)
	case <-timeout:
		t.Fatal("Timed out")
	}
	require.NoError(t, exporter.Shutdown(ctx))
}

func TestNewTracesExporter(t *testing.T) {
	cfg := &Config{}
	cfg.API.Key = "ddog_32_characters_long_api_key1"
	params := exportertest.NewNopSettings()
	tcfg := config.New()
	tcfg.Endpoints[0].APIKey = "ddog_32_characters_long_api_key1"
	ctx := context.Background()
	traceagent := pkgagent.NewAgent(ctx, tcfg, telemetry.NewNoopCollector(), &ddgostatsd.NoOpClient{})

	// The client should have been created correctly
	f := NewFactory(testComponent{traceagent}, nil, nil, nil, metricsclient.NewStatsdClientWrapper(&ddgostatsd.NoOpClient{}))
	exp, err := f.CreateTracesExporter(context.Background(), params, cfg)
	assert.NoError(t, err)
	assert.NotNil(t, exp)
}

func simpleTraces() ptrace.Traces {
	return genTraces([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}, nil)
}

func genTraces(traceID pcommon.TraceID, attrs map[string]any) ptrace.Traces {
	traces := ptrace.NewTraces()
	rspans := traces.ResourceSpans().AppendEmpty()
	span := rspans.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(traceID)
	span.SetSpanID([8]byte{0, 0, 0, 0, 1, 2, 3, 4})
	if attrs == nil {
		return traces
	}
	//nolint:errcheck
	rspans.Resource().Attributes().FromRaw(attrs)
	return traces
}
