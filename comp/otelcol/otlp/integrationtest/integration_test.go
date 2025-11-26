// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// TODO(OASIS-79): fix data race then remove !race
//go:build otlp && test && !race

package integrationtest

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	delegatedauthfx "github.com/DataDog/datadog-agent/comp/core/delegatedauth/fx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinylib/msgp/msgp"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	apitrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/fx"
	"google.golang.org/protobuf/proto"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	agentConfig "github.com/DataDog/datadog-agent/cmd/otel-agent/config"
	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	agenttelemetryfx "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/fx"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	logtrace "github.com/DataDog/datadog-agent/comp/core/log/fx-trace"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/comp/core/pid/pidimpl"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	taggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	statsdotel "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/otel"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	logconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	collectorcontribFx "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/fx"
	collectordef "github.com/DataDog/datadog-agent/comp/otelcol/collector/def"
	collectorfx "github.com/DataDog/datadog-agent/comp/otelcol/collector/fx"
	collectorimpl "github.com/DataDog/datadog-agent/comp/otelcol/collector/impl"
	converter "github.com/DataDog/datadog-agent/comp/otelcol/converter/def"
	converterfx "github.com/DataDog/datadog-agent/comp/otelcol/converter/fx"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/logsagentpipelineimpl"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	metricscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-otel"
	tracecomp "github.com/DataDog/datadog-agent/comp/trace"
	traceagentcomp "github.com/DataDog/datadog-agent/comp/trace/agent/impl"
	gzipfx "github.com/DataDog/datadog-agent/comp/trace/compression/fx-gzip"
	traceconfig "github.com/DataDog/datadog-agent/comp/trace/config"
	payloadmodifierfx "github.com/DataDog/datadog-agent/comp/trace/payload-modifier/fx"
	pkgconfigenv "github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func runTestOTelAgent(ctx context.Context, params *subcommands.GlobalParams, pidfilePath string) (*fx.App, error) {
	return fxutil.TestRunWithApp(
		forwarder.Bundle(defaultforwarder.NewParams()),
		delegatedauthfx.Module(),
		logtrace.Module(),
		inventoryagentimpl.Module(),
		workloadmetafx.Module(workloadmeta.NewParams()),
		fx.Supply(metricsclient.NewStatsdClientWrapper(&ddgostatsd.NoOpClient{})),
		fx.Provide(func(client *metricsclient.StatsdClientWrapper) statsd.Component {
			return statsdotel.NewOTelStatsd(client)
		}),
		sysprobeconfig.NoneModule(),
		ipcfx.ModuleReadWrite(),
		collectorfx.Module(collectorimpl.NewParams(false)),
		collectorcontribFx.Module(),
		converterfx.Module(),
		fx.Provide(func(cp converter.Component) confmap.Converter {
			return cp
		}),
		fx.Provide(func() (coreconfig.Component, error) {
			c, err := agentConfig.NewConfigComponent(context.Background(), "", params.ConfPaths)
			if err != nil {
				return nil, err
			}
			c.Set("otelcollector.enabled", true, pkgconfigmodel.SourceFile)
			pkgconfigenv.DetectFeatures(c)
			return c, nil
		}),
		fxutil.ProvideOptional[coreconfig.Component](),
		fx.Provide(func() []string {
			return append(params.ConfPaths, params.Sets...)
		}),
		fx.Provide(func(h hostnameinterface.Component) serializerexporter.SourceProviderFunc {
			return h.Get
		}),
		hostnameinterface.MockModule(),
		secretsnoopfx.Module(),

		fx.Provide(func(_ coreconfig.Component) logdef.Params {
			return logdef.ForDaemon(params.LoggerName, "log_file", pkgconfigsetup.DefaultOTelAgentLogFile)
		}),
		fx.Provide(func() logconfig.IntakeOrigin {
			return logconfig.DDOTIntakeOrigin
		}),
		logsagentpipelineimpl.Module(),
		logscompressionfx.Module(),
		metricscompressionfx.Module(),
		// For FX to provide the compression.Compressor interface (used by serializer.NewSerializer)
		// implemented by the metricsCompression.Component
		fx.Provide(func(c metricscompression.Component) compression.Compressor {
			return c
		}),
		fx.Provide(serializer.NewSerializer),
		// For FX to provide the serializer.MetricSerializer from the serializer.Serializer
		fx.Provide(func(s *serializer.Serializer) serializer.MetricSerializer {
			return s
		}),
		fx.Supply("test-host"),
		fx.Provide(func(c defaultforwarder.Component) (defaultforwarder.Forwarder, error) {
			return defaultforwarder.Forwarder(c), nil
		}),
		orchestratorimpl.MockModule(),
		pidimpl.Module(),
		fx.Supply(pidimpl.NewParams(pidfilePath)),
		fx.Invoke(func(_ collectordef.Component, _ defaultforwarder.Forwarder, _ option.Option[logsagentpipeline.Component], _ pid.Component) {
		}),
		taggerfx.Module(),
		noopsimpl.Module(),
		fx.Provide(func(cfg traceconfig.Component) telemetry.TelemetryCollector {
			return telemetry.NewCollector(cfg.Object())
		}),
		gzipfx.Module(),

		// ctx is required to be supplied from here, as Windows needs to inject its own context
		// to allow the agent to work as a service.
		fx.Provide(func() context.Context { return ctx }), // fx.Supply(ctx) fails with a missing type error.
		fx.Supply(&traceagentcomp.Params{
			CPUProfile:  "",
			MemProfile:  "",
			PIDFilePath: "",
		}),
		payloadmodifierfx.NilModule(),
		tracecomp.Bundle(),
		agenttelemetryfx.Module(),
	)
}

func TestIntegration(t *testing.T) {
	var app *fx.App
	var err error

	// 1. Set up mock Datadog server
	// See also https://github.com/DataDog/datadog-agent/blob/49c16e0d4deab396626238fa1d572b684475a53f/cmd/trace-agent/test/backend.go
	apmstatsRec := &testutil.HTTPRequestRecorderWithChan{Pattern: testutil.APMStatsEndpoint, ReqChan: make(chan []byte)}
	tracesRec := &testutil.HTTPRequestRecorderWithChan{Pattern: testutil.TraceEndpoint, ReqChan: make(chan []byte)}
	server := testutil.DatadogServerMock(apmstatsRec.HandlerFunc, tracesRec.HandlerFunc)
	defer server.Close()
	t.Setenv("SERVER_URL", server.URL)

	// 2. Start in-process collector
	params := &subcommands.GlobalParams{
		ConfPaths:  []string{"integration_test_config.yaml"},
		ConfigName: "datadog-otel",
		LoggerName: "OTELCOL",
	}
	pidfilePath := "test_pid"
	go func() {
		if app, err = runTestOTelAgent(context.Background(), params, pidfilePath); err != nil {
			log.Fatal("failed to start otel agent ", err)
		}
	}()
	waitForReadiness()

	// 3. Validate that pid file was created
	_, err = os.Stat(pidfilePath)
	require.NoError(t, err)

	// 3. Generate and send traces
	sendTraces(t)

	// 4. Validate traces and APM stats from the mock server
	var spans []*pb.Span
	var stats []*pb.ClientGroupedStats

	// 5 sampled spans + APM stats on 10 spans are sent to datadog exporter
	for len(spans) < 5 || len(stats) < 10 {
		select {
		case tracesBytes := <-tracesRec.ReqChan:
			gz := getGzipReader(t, tracesBytes)
			slurp, err := io.ReadAll(gz)
			require.NoError(t, err)
			var traces pb.AgentPayload
			require.NoError(t, proto.Unmarshal(slurp, &traces))
			for _, tps := range traces.TracerPayloads {
				for _, chunks := range tps.Chunks {
					spans = append(spans, chunks.Spans...)
				}
			}

		case apmstatsBytes := <-apmstatsRec.ReqChan:
			gz := getGzipReader(t, apmstatsBytes)
			var spl pb.StatsPayload
			require.NoError(t, msgp.Decode(gz, &spl))
			for _, csps := range spl.Stats {
				for _, csbs := range csps.Stats {
					stats = append(stats, csbs.Stats...)
					for _, stat := range csbs.Stats {
						assert.True(t, strings.HasPrefix(stat.Resource, "TestSpan"))
						assert.Equal(t, uint64(1), stat.Hits)
						assert.Equal(t, uint64(1), stat.TopLevelHits)
						assert.Equal(t, "client", stat.SpanKind)
						assert.Equal(t, []string{"extra_peer_tag:tag_val", "peer.service:svc"}, stat.PeerTags)
					}
				}
			}
		}
	}

	// Verify we don't receive more than the expected numbers
	assert.Len(t, spans, 5)
	assert.Len(t, stats, 10)

	// Verify that DDOT stops gracefully
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	require.NoError(t, app.Stop(stopCtx))
}

func waitForReadiness() {
	for i := 0; ; i++ {
		resp, err := http.Get("http://localhost:13133") // default addr of the OTel collector health check extension
		defer func() {
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
		}()
		if err == nil && resp.StatusCode == 200 {
			return
		}
		log.Print("health check failed, retrying ", i, err, resp)
		t := time.Duration(math.Pow(2, float64(i)))
		time.Sleep(t * time.Second)
	}
}

func sendTraces(t *testing.T) {
	ctx := context.Background()

	// Set up OTel-Go SDK and exporter
	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
	require.NoError(t, err)
	bsp := sdktrace.NewBatchSpanProcessor(traceExporter)
	r1, _ := resource.New(ctx, resource.WithAttributes(attribute.String("k8s.node.name", "aaaa")))
	r2, _ := resource.New(ctx, resource.WithAttributes(attribute.String("k8s.node.name", "bbbb")))
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithResource(r1),
	)
	tracerProvider2 := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithResource(r2),
	)
	otel.SetTracerProvider(tracerProvider)
	defer func() {
		require.NoError(t, tracerProvider.Shutdown(ctx))
		require.NoError(t, tracerProvider2.Shutdown(ctx))
	}()

	tracer := otel.Tracer("test-tracer")
	for i := 0; i < 10; i++ {
		_, span := tracer.Start(ctx, fmt.Sprintf("TestSpan%d", i), apitrace.WithSpanKind(apitrace.SpanKindClient))

		if i == 3 {
			// Send some traces from a different resource
			// This verifies that stats from different hosts don't accidentally create extraneous empty stats buckets
			otel.SetTracerProvider(tracerProvider2)
			tracer = otel.Tracer("test-tracer2")
		}
		// Only sample 5 out of the 10 spans
		if i < 5 {
			span.SetAttributes(attribute.Bool("sampled", true))
		}
		span.SetAttributes(attribute.String("peer.service", "svc"))
		span.SetAttributes(attribute.String("extra_peer_tag", "tag_val"))
		span.End()
	}
	time.Sleep(1 * time.Second)
}

func getGzipReader(t *testing.T, reqBytes []byte) io.Reader {
	buf := bytes.NewBuffer(reqBytes)
	reader, err := gzip.NewReader(buf)
	require.NoError(t, err)
	return reader
}
