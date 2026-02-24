// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build test

package datadogexporter

import (
	"compress/zlib"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil"
	implgzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata/payload"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/otel"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

var sourceProvider serializerexporter.SourceProviderFunc = func(_ context.Context) (string, error) {
	return "test", nil
}

// mockLogsAgentPipeline implements logsagentpipeline.Component, used for testing logs agent exporter
type mockLogsAgentPipeline struct{}

func (*mockLogsAgentPipeline) GetPipelineProvider() pipeline.Provider { return &mockProvider{} }

var _ logsagentpipeline.Component = (*mockLogsAgentPipeline)(nil)

type mockProvider struct{}

var _ pipeline.Provider = (*mockProvider)(nil)

func (*mockProvider) Start()                                  {}
func (*mockProvider) Stop()                                   {}
func (*mockProvider) NextPipelineChan() chan *message.Message { return make(chan *message.Message, 10) }
func (*mockProvider) GetOutputChan() chan *message.Message    { return make(chan *message.Message, 10) }
func (*mockProvider) NextPipelineChanWithMonitor() (chan *message.Message, *metrics.CapacityMonitor) {
	return make(chan *message.Message), metrics.NewCapacityMonitor("test", "test-instance")
}
func (*mockProvider) Flush(_ context.Context) {}

func createTestResAttrs() pcommon.Resource {
	res := pcommon.NewResource()
	res.Attributes().PutBool("datadog.host.use_as_metadata", true)
	res.Attributes().PutStr("datadog.host.name", "test-host")
	res.Attributes().PutStr("os.description", "test-os")
	return res
}

func createTestCfg(t *testing.T, serverAddr string) *datadogconfig.Config {
	cfg := datadogconfig.Config{
		API: datadogconfig.APIConfig{
			Key: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		TagsConfig: datadogconfig.TagsConfig{
			Hostname: "test-host",
		},
		Traces: datadogconfig.TracesExporterConfig{
			TCPAddrConfig: confignet.TCPAddrConfig{
				Endpoint: serverAddr,
			},
		},
		Metrics: datadogconfig.MetricsConfig{
			TCPAddrConfig: confignet.TCPAddrConfig{
				Endpoint: serverAddr,
			},
			HistConfig: datadogconfig.HistogramConfig{
				Mode: "distributions",
			},
			DeltaTTL: 3600,
			SumConfig: datadogconfig.SumConfig{
				CumulativeMonotonicMode:        datadogconfig.CumulativeMonotonicSumModeToDelta,
				InitialCumulativeMonotonicMode: datadogconfig.InitialValueModeAuto,
			},
			SummaryConfig: datadogconfig.SummaryConfig{
				Mode: datadogconfig.SummaryModeGauges,
			},
		},
		Logs: datadogconfig.LogsConfig{
			TCPAddrConfig: confignet.TCPAddrConfig{
				Endpoint: serverAddr,
			},
		},
		HostMetadata: datadogconfig.HostMetadataConfig{
			Enabled:        true,
			HostnameSource: datadogconfig.HostnameSourceConfigOrSystem,
			ReporterPeriod: 30 * time.Minute,
		},
		HostnameDetectionTimeout: 25 * time.Second,
	}
	require.NoError(t, cfg.Validate())
	return &cfg
}

func createTestServer(t *testing.T, c chan payload.HostMetadata) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/intake/" {
			return
		}

		reader, err := zlib.NewReader(r.Body)
		require.NoError(t, err)

		body, err := io.ReadAll(reader)
		require.NoError(t, err)

		var recvMetadata payload.HostMetadata
		err = json.Unmarshal(body, &recvMetadata)
		require.NoError(t, err)

		c <- recvMetadata
	}))
}

func createTestFactory(t *testing.T, serverAddr string) exporter.Factory {
	params := exportertest.NewNopSettings(Type)
	scfg := &serializerexporter.ExporterConfig{}
	scfg.API.Site = serverAddr
	scfg.Metrics.Metrics.TCPAddrConfig.Endpoint = serverAddr
	srlz, fwd, err := serializerexporter.InitSerializer(params.Logger, scfg, &sourceProvider)
	require.NoError(t, err)
	require.NoError(t, fwd.Start())

	tcfg := config.New()
	tcfg.ReceiverEnabled = false
	tcfg.TraceWriter.FlushPeriodSeconds = 0.1
	tcfg.Endpoints[0].APIKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tcfg.Endpoints[0].Host = serverAddr
	ctx := context.Background()
	traceagent := pkgagent.NewAgent(ctx, tcfg, telemetry.NewNoopCollector(), &ddgostatsd.NoOpClient{}, implgzip.NewComponent())
	go traceagent.Run()

	return NewFactory(testComponent{traceagent, nil}, srlz, &mockLogsAgentPipeline{}, sourceProvider, metricsclient.NewStatsdClientWrapper(&ddgostatsd.NoOpClient{}), otel.NewDisabledGatewayUsage(), serializerexporter.TelemetryStore{})
}

func TestHostMetadata_FromTraces(t *testing.T) {
	c := make(chan payload.HostMetadata)
	server := createTestServer(t, c)
	defer server.Close()

	ctx := context.Background()
	f := createTestFactory(t, server.URL)
	exporter, err := f.CreateTraces(ctx, exportertest.NewNopSettings(Type), createTestCfg(t, server.URL))
	assert.NoError(t, err)

	res := createTestResAttrs()
	traces := simpleTraces()
	res.MoveTo(traces.ResourceSpans().At(0).Resource())
	err = exporter.ConsumeTraces(ctx, traces)
	assert.NoError(t, err)

	hm := <-c
	assert.Equal(t, "test-host", hm.InternalHostname)
	assert.Equal(t, "otelcol-contrib", hm.Flavor)
	assert.Equal(t, payload.Meta{Hostname: "test-host"}, *hm.Meta)
	assert.Equal(t, map[string]string{"hostname": "test-host", "os": "test-os"}, hm.Platform())
}

func TestHostMetadata_FromMetrics(t *testing.T) {
	c := make(chan payload.HostMetadata)
	server := createTestServer(t, c)
	defer server.Close()

	ctx := context.Background()
	f := createTestFactory(t, server.URL)
	exporter, err := f.CreateMetrics(ctx, exportertest.NewNopSettings(Type), createTestCfg(t, server.URL))
	assert.NoError(t, err)

	res := createTestResAttrs()
	md := pmetric.NewMetrics()
	rms := md.ResourceMetrics()
	rm := rms.AppendEmpty()
	res.MoveTo(rm.Resource())
	ilms := rm.ScopeMetrics()
	ilm := ilms.AppendEmpty()
	metricsArray := ilm.Metrics()
	met := metricsArray.AppendEmpty()
	met.SetName("test_metric")
	met.SetEmptyGauge()
	gdps := met.Gauge().DataPoints()
	gdp := gdps.AppendEmpty()
	gdp.SetIntValue(100)

	err = exporter.ConsumeMetrics(ctx, md)
	assert.NoError(t, err)

	hm := <-c
	assert.Equal(t, "test-host", hm.InternalHostname)
	assert.Equal(t, "otelcol-contrib", hm.Flavor)
	assert.Equal(t, payload.Meta{Hostname: "test-host"}, *hm.Meta)
	assert.Equal(t, map[string]string{"hostname": "test-host", "os": "test-os"}, hm.Platform())
}

func TestHostMetadata_FromLogs(t *testing.T) {
	c := make(chan payload.HostMetadata)
	server := createTestServer(t, c)
	defer server.Close()

	ctx := context.Background()
	f := createTestFactory(t, server.URL)
	exporter, err := f.CreateLogs(ctx, exportertest.NewNopSettings(Type), createTestCfg(t, server.URL))
	assert.NoError(t, err)

	res := createTestResAttrs()
	logs := testutil.GenerateLogsOneLogRecord()
	res.MoveTo(logs.ResourceLogs().At(0).Resource())
	err = exporter.ConsumeLogs(ctx, logs)
	assert.NoError(t, err)

	hm := <-c
	assert.Equal(t, "test-host", hm.InternalHostname)
	assert.Equal(t, "otelcol-contrib", hm.Flavor)
	assert.Equal(t, payload.Meta{Hostname: "test-host"}, *hm.Meta)
	assert.Equal(t, map[string]string{"hostname": "test-host", "os": "test-os"}, hm.Platform())
}
