// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build test

package datadogexporter

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient"
	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	gzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/otel"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/protobuf/proto"
)

type testComponent struct {
	*pkgagent.Agent
	wg *sync.WaitGroup
}

func (c testComponent) SetOTelAttributeTranslator(attrstrans *attributes.Translator) {
	c.Agent.OTLPReceiver.SetOTelAttributeTranslator(attrstrans)
}

func (c testComponent) ReceiveOTLPSpans(ctx context.Context, rspans ptrace.ResourceSpans, httpHeader http.Header, hostFromAttributesHandler attributes.HostFromAttributesHandler) (source.Source, error) {
	return c.Agent.OTLPReceiver.ReceiveResourceSpans(ctx, rspans, httpHeader, hostFromAttributesHandler)
}

func (c testComponent) SendStatsPayload(p *pb.StatsPayload) {
	c.Agent.StatsWriter.SendPayload(p)
}

func (c testComponent) GetHTTPHandler(endpoint string) http.Handler {
	if v, ok := c.Agent.Receiver.Handlers[endpoint]; ok {
		return v
	}
	return nil
}

var _ traceagent.Component = (*testComponent)(nil)

func TestTraceExporter(t *testing.T) {
	t.Run("ReceiveResourceSpansV1", func(t *testing.T) {
		testTraceExporter(false, t)
	})

	t.Run("ReceiveResourceSpansV2", func(t *testing.T) {
		testTraceExporter(true, t)
	})
}

func testTraceExporter(enableReceiveResourceSpansV2 bool, t *testing.T) {
	got := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", req.Header.Get("DD-Api-Key"))
		got <- req.Header.Get("Content-Type")
		rw.WriteHeader(http.StatusAccepted)
	}))

	defer server.Close()
	cfg := datadogconfig.Config{
		API: datadogconfig.APIConfig{
			Key: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		TagsConfig: datadogconfig.TagsConfig{
			Hostname: "test-host",
		},
		Traces: datadogconfig.TracesExporterConfig{
			TCPAddrConfig: confignet.TCPAddrConfig{
				Endpoint: server.URL,
			},
			TracesConfig: datadogconfig.TracesConfig{
				IgnoreResources: []string{},
			},
		},
	}

	params := exportertest.NewNopSettings(Type)
	tcfg := config.New()
	tcfg.ReceiverEnabled = false
	tcfg.TraceWriter.FlushPeriodSeconds = 0.1
	tcfg.Endpoints[0].APIKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tcfg.Endpoints[0].Host = server.URL
	if !enableReceiveResourceSpansV2 {
		tcfg.Features["disable_receive_resource_spans_v2"] = struct{}{}
	}
	ctx := context.Background()
	traceagent := pkgagent.NewAgent(ctx, tcfg, telemetry.NewNoopCollector(), &ddgostatsd.NoOpClient{}, gzip.NewComponent())

	telemetryComp := fxutil.Test[coretelemetry.Mock](t, telemetryimpl.MockModule())
	store := serializerexporter.TelemetryStore{
		DDOTTraces: telemetryComp.NewGauge(
			"runtime",
			"datadog_agent_ddot_traces",
			[]string{"version", "command", "host", "task_arn"},
			"Usage metric of OTLP traces in DDOT",
		),

		DDOTGWUsage: telemetryComp.NewGauge(
			"runtime",
			"datadog_agent_ddot_gateway_usage",
			[]string{"version", "command"},
			"Usage metric for GW deployments with DDOT",
		),
	}
	f := NewFactory(testComponent{traceagent, nil}, nil, nil, nil, metricsclient.NewStatsdClientWrapper(&ddgostatsd.NoOpClient{}), otel.NewDisabledGatewayUsage(), store)
	exporter, err := f.CreateTraces(ctx, params, &cfg)
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

	usageMetric, err := telemetryComp.GetGaugeMetric("runtime", "datadog_agent_ddot_traces")
	require.NoError(t, err)
	require.Len(t, usageMetric, 1)
	assert.Equal(t, map[string]string{"host": "test-host", "command": "otelcol", "version": "latest", "task_arn": ""}, usageMetric[0].Tags())
	assert.Equal(t, 1.0, usageMetric[0].Value())
}

func TestNewTracesExporter(t *testing.T) {
	t.Run("ReceiveResourceSpansV1", func(t *testing.T) {
		testNewTracesExporter(false, t)
	})

	t.Run("ReceiveResourceSpansV2", func(t *testing.T) {
		testNewTracesExporter(true, t)
	})
}

func testNewTracesExporter(enableReceiveResourceSpansV2 bool, t *testing.T) {
	cfg := &datadogconfig.Config{}
	cfg.API.Key = "ddog_32_characters_long_api_key1"
	params := exportertest.NewNopSettings(Type)
	tcfg := config.New()
	tcfg.Endpoints[0].APIKey = "ddog_32_characters_long_api_key1"
	ctx := context.Background()
	tcfg.ReceiverEnabled = false
	if !enableReceiveResourceSpansV2 {
		tcfg.Features["disable_receive_resource_spans_v2"] = struct{}{}
	}
	traceagent := pkgagent.NewAgent(ctx, tcfg, telemetry.NewNoopCollector(), &ddgostatsd.NoOpClient{}, gzip.NewComponent())

	// The client should have been created correctly
	telemetryComp := fxutil.Test[coretelemetry.Mock](t, telemetryimpl.MockModule())
	store := serializerexporter.TelemetryStore{
		DDOTTraces: telemetryComp.NewGauge(
			"runtime",
			"datadog_agent_ddot_traces",
			[]string{"version", "command", "host", "task_arn"},
			"Usage metric of OTLP traces in DDOT",
		),
	}
	f := NewFactory(testComponent{traceagent, nil}, nil, nil, nil, metricsclient.NewStatsdClientWrapper(&ddgostatsd.NoOpClient{}), otel.NewDisabledGatewayUsage(), store)
	exp, err := f.CreateTraces(context.Background(), params, cfg)
	assert.NoError(t, err)
	assert.NotNil(t, exp)
}

func simpleTraces() ptrace.Traces {
	return genTraces([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}, nil)
}

func genTraces(traceID pcommon.TraceID, attrs map[string]any) ptrace.Traces {
	traces := ptrace.NewTraces()
	rspans := traces.ResourceSpans().AppendEmpty()
	rspans.Resource().Attributes().PutStr("datadog.host.name", "test-host")
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

func TestNoPanicSendTraceAfterTraceAgentStop(t *testing.T) {
	cfg := datadogconfig.Config{
		API: datadogconfig.APIConfig{
			Key: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}

	params := exportertest.NewNopSettings(Type)
	tcfg := config.New()
	tcfg.ReceiverEnabled = false
	tcfg.TraceWriter.FlushPeriodSeconds = 0.1
	tcfg.Endpoints[0].APIKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	ctx, cancel := context.WithCancel(context.Background())
	traceagent := pkgagent.NewAgent(ctx, tcfg, telemetry.NewNoopCollector(), &ddgostatsd.NoOpClient{}, gzip.NewComponent())

	store := serializerexporter.TelemetryStore{}
	var wg sync.WaitGroup
	f := NewFactory(testComponent{traceagent, &wg}, nil, nil, nil, metricsclient.NewStatsdClientWrapper(&ddgostatsd.NoOpClient{}), otel.NewDisabledGatewayUsage(), store)
	exporter, err := f.CreateTraces(ctx, params, &cfg)
	assert.NoError(t, err)

	wg.Add(1)
	go func() {
		defer wg.Done()
		traceagent.Run()
	}()
	cancel()
	wg.Wait()

	err = exporter.ConsumeTraces(ctx, simpleTraces())
	assert.ErrorContains(t, err, "OTLPReceiver in trace agent is already stopped")
}

func TestNoAgenthost(t *testing.T) {
	payloadChan := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", req.Header.Get("DD-Api-Key"))
		assert.Equal(t, "application/x-protobuf", req.Header.Get("Content-Type"))

		// Read and store the request body
		body, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		payloadChan <- body

		rw.WriteHeader(http.StatusAccepted)
	}))

	defer server.Close()
	cfg := datadogconfig.Config{
		API: datadogconfig.APIConfig{
			Key: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		Traces: datadogconfig.TracesExporterConfig{
			TCPAddrConfig: confignet.TCPAddrConfig{
				Endpoint: server.URL,
			},
			TracesConfig: datadogconfig.TracesConfig{
				IgnoreResources: []string{},
			},
		},
	}

	params := exportertest.NewNopSettings(Type)
	tcfg := config.New()
	tcfg.ReceiverEnabled = false
	tcfg.TraceWriter.FlushPeriodSeconds = 0.1
	tcfg.Endpoints[0].APIKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tcfg.Endpoints[0].Host = server.URL
	tcfg.Hostname = "" // Set hostname to empty string
	ctx := context.Background()
	traceagent := pkgagent.NewAgent(ctx, tcfg, telemetry.NewNoopCollector(), &ddgostatsd.NoOpClient{}, gzip.NewComponent())

	telemetryComp := fxutil.Test[coretelemetry.Mock](t, telemetryimpl.MockModule())
	store := serializerexporter.TelemetryStore{
		DDOTTraces: telemetryComp.NewGauge(
			"runtime",
			"datadog_agent_ddot_traces",
			[]string{"version", "command", "host", "task_arn"},
			"Usage metric of OTLP traces in DDOT",
		),

		DDOTGWUsage: telemetryComp.NewGauge(
			"runtime",
			"datadog_agent_ddot_gateway_usage",
			[]string{"version", "command"},
			"Usage metric for GW deployments with DDOT",
		),
	}
	f := NewFactory(testComponent{traceagent, nil}, nil, nil, nil, metricsclient.NewStatsdClientWrapper(&ddgostatsd.NoOpClient{}), otel.NewDisabledGatewayUsage(), store)
	exporter, err := f.CreateTraces(ctx, params, &cfg)
	assert.NoError(t, err)

	go traceagent.Run()

	err = exporter.ConsumeTraces(ctx, simpleTraces())
	assert.NoError(t, err)

	// Wait for payload to be received
	timeout := time.After(2 * time.Second)
	var payloadBytes []byte
	select {
	case payloadBytes = <-payloadChan:
		// Got the payload
	case <-timeout:
		t.Fatal("Timed out waiting for payload")
	}

	// Decompress and parse the payload
	reader, err := gzip.NewComponent().NewReader(io.NopCloser(bytes.NewReader(payloadBytes)))
	require.NoError(t, err)
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	require.NoError(t, err)

	var agentPayload pb.AgentPayload
	err = proto.Unmarshal(decompressed, &agentPayload)
	require.NoError(t, err)

	assert.Empty(t, agentPayload.HostName, "agent hostname should be empty")
	assert.Len(t, agentPayload.TracerPayloads, 1)
	assert.Equal(t, "test-host", agentPayload.TracerPayloads[0].Hostname)

	require.NoError(t, exporter.Shutdown(ctx))
}
