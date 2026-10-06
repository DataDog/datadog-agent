// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package datadogexporter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/pdata/plog"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logsagentpipeline "github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/def"
	logsagentpipelineimpl "github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/impl"
	datadogconfig "github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/datadogconfig"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/logsagentexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	compressionmock "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/otel"
)

const (
	sentLogRecordsMetric       = "otelcol_exporter_sent_log_records"
	sendFailedLogRecordsMetric = "otelcol_exporter_send_failed_log_records"
)

// setSyncSenderGate sets the UseSyncSender feature gate for the duration of the test.
func setSyncSenderGate(t *testing.T, enabled bool) {
	t.Helper()
	previous := logsagentexporter.IsSyncSenderEnabled()
	require.NoError(t, featuregate.GlobalRegistry().Set(logsagentexporter.SyncSenderGateID, enabled))
	t.Cleanup(func() {
		require.NoError(t, featuregate.GlobalRegistry().Set(logsagentexporter.SyncSenderGateID, previous))
	})
}

// fakeLogsIntake stands in for the Datadog logs intake. It answers every log payload with the status
// returned by statusFor and counts the log records it accepted.
type fakeLogsIntake struct {
	*httptest.Server
	statusFor func(payload int64) int
	payloads  atomic.Int64
	delivered atomic.Int64
}

func newFakeLogsIntake(t *testing.T, statusFor func(payload int64) int) *fakeLogsIntake {
	t.Helper()
	intake := &fakeLogsIntake{statusFor: statusFor}
	intake.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var records []json.RawMessage
		if json.Unmarshal(body, &records) != nil || len(records) == 0 {
			// The logs agent checks HTTP connectivity with an empty JSON object when it starts.
			w.WriteHeader(http.StatusOK)
			return
		}
		status := intake.statusFor(intake.payloads.Add(1))
		if status == http.StatusOK {
			intake.delivered.Add(int64(len(records)))
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(intake.Close)
	return intake
}

func alwaysStatus(status int) func(int64) int {
	return func(int64) int { return status }
}

type testHostname struct{}

func (testHostname) Get(context.Context) (string, error) { return "test-host", nil }
func (testHostname) GetWithProvider(context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{Hostname: "test-host", Provider: "test"}, nil
}
func (testHostname) GetSafe(context.Context) string { return "test-host" }

// testLogsAgentConfig returns the configuration of the OTel logs agent pipeline that DDOT uses,
// pointed at intakeURL, with overrides applied.
func testLogsAgentConfig(t *testing.T, intakeURL string, overrides map[string]interface{}) coreconfig.Component {
	t.Helper()
	u, err := url.Parse(intakeURL)
	require.NoError(t, err)
	settings := map[string]interface{}{
		"api_key":                       "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"logs_enabled":                  true,
		"logs_config.force_use_http":    true,
		"logs_config.logs_dd_url":       u.Host,
		"logs_config.logs_no_ssl":       true,
		"logs_config.batch_wait":        1,
		"logs_config.stop_grace_period": 1,
	}
	maps.Copy(settings, overrides)
	return coreconfig.NewMockWithOverrides(t, settings)
}

// startTestLogsAgent starts the OTel logs agent pipeline that DDOT uses, configured by cfg.
func startTestLogsAgent(t *testing.T, cfg coreconfig.Component) logsagentpipeline.LogsAgent {
	t.Helper()
	agent := logsagentpipelineimpl.NewLogsAgent(logsagentpipelineimpl.Dependencies{
		Log:          logmock.New(t),
		Config:       cfg,
		Hostname:     testHostname{},
		Compression:  compressionmock.NewMockCompressor(),
		IntakeOrigin: logsconfig.DDOTIntakeOrigin,
	})
	require.NotNil(t, agent)
	require.NoError(t, agent.Start(context.Background()))
	t.Cleanup(func() { _ = agent.Stop(context.Background()) })
	return agent
}

// newTestLogsExporter builds the DDOT logs exporter on top of agent. The sending queue is disabled so
// that ConsumeLogs returns the outcome of the export to the caller.
func newTestLogsExporter(t *testing.T, agent logsagentpipeline.Component, retry configretry.BackOffConfig) (exporter.Logs, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	set := exportertest.NewNopSettings(Type)
	set.TelemetrySettings.MeterProvider = sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	f := NewFactory(nil, nil, agent, sourceProvider, nil, otel.NewDisabledGatewayUsage(), serializerexporter.TelemetryStore{}, nil)
	cfg := f.CreateDefaultConfig().(*datadogconfig.Config)
	cfg.API.Key = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cfg.HostMetadata.Enabled = false
	cfg.QueueSettings = configoptional.None[exporterhelper.QueueBatchConfig]()
	cfg.BackOffConfig = retry

	exp, err := f.CreateLogs(context.Background(), set, cfg)
	require.NoError(t, err)
	require.NoError(t, exp.Start(context.Background(), componenttest.NewNopHost()))
	t.Cleanup(func() { require.NoError(t, exp.Shutdown(context.Background())) })
	return exp, reader
}

func noRetry() configretry.BackOffConfig {
	return configretry.BackOffConfig{Enabled: false}
}

func fastRetry() configretry.BackOffConfig {
	return configretry.BackOffConfig{
		Enabled:         true,
		InitialInterval: 10 * time.Millisecond,
		Multiplier:      1.5,
		MaxInterval:     50 * time.Millisecond,
		MaxElapsedTime:  5 * time.Second,
	}
}

// exporterCounter returns the value of an exporterhelper counter, summed over all its attributes.
func exporterCounter(t *testing.T, reader *sdkmetric.ManualReader, name string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "%s is not an int64 counter", name)
			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
		}
	}
	return total
}

func testLogs(n int) plog.Logs {
	ld := plog.NewLogs()
	records := ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords()
	for i := 0; i < n; i++ {
		lr := records.AppendEmpty()
		lr.Body().SetStr(fmt.Sprintf("log record %d", i))
		lr.SetSeverityText("Info")
	}
	return ld
}

// TestLogsExporter_AsyncPipeline_SwallowsIntakeErrors documents the behavior reported in
// open-telemetry/opentelemetry-collector-contrib#47386: without the sync sender, intake errors never
// reach exporterhelper, which counts the records as sent. If this test starts failing, the
// asynchronous path reports delivery errors and the UseSyncSender gate can be retired.
func TestLogsExporter_AsyncPipeline_SwallowsIntakeErrors(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusServiceUnavailable} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			setSyncSenderGate(t, false)
			intake := newFakeLogsIntake(t, alwaysStatus(status))
			exp, reader := newTestLogsExporter(t, startTestLogsAgent(t, testLogsAgentConfig(t, intake.URL, nil)), noRetry())

			require.NoError(t, exp.ConsumeLogs(context.Background(), testLogs(10)))
			require.Eventually(t, func() bool { return intake.payloads.Load() > 0 }, 10*time.Second, 10*time.Millisecond,
				"the logs agent never tried to deliver the payload")

			assert.Zero(t, intake.delivered.Load())
			assert.Equal(t, int64(10), exporterCounter(t, reader, sentLogRecordsMetric))
			assert.Zero(t, exporterCounter(t, reader, sendFailedLogRecordsMetric))
		})
	}
}

func TestLogsExporter_SyncSender_Delivers(t *testing.T) {
	setSyncSenderGate(t, true)
	intake := newFakeLogsIntake(t, alwaysStatus(http.StatusOK))
	exp, reader := newTestLogsExporter(t, startTestLogsAgent(t, testLogsAgentConfig(t, intake.URL, nil)), noRetry())

	require.NoError(t, exp.ConsumeLogs(context.Background(), testLogs(10)))

	assert.Equal(t, int64(10), intake.delivered.Load(), "records must reach the intake before ConsumeLogs returns")
	assert.Equal(t, int64(10), exporterCounter(t, reader, sentLogRecordsMetric))
	assert.Zero(t, exporterCounter(t, reader, sendFailedLogRecordsMetric))
}

func TestLogsExporter_SyncSender_PermanentError(t *testing.T) {
	setSyncSenderGate(t, true)
	intake := newFakeLogsIntake(t, alwaysStatus(http.StatusForbidden))
	// Retries stay enabled: a permanent error must be dropped without retrying.
	exp, reader := newTestLogsExporter(t, startTestLogsAgent(t, testLogsAgentConfig(t, intake.URL, nil)), fastRetry())

	err := exp.ConsumeLogs(context.Background(), testLogs(10))

	require.Error(t, err)
	assert.True(t, consumererror.IsPermanent(err), "403 must be reported as a permanent error: %v", err)
	assert.Contains(t, err.Error(), "403")
	assert.Equal(t, int64(1), intake.payloads.Load(), "a permanent error must not be retried")
	assert.Equal(t, int64(10), exporterCounter(t, reader, sendFailedLogRecordsMetric))
	assert.Zero(t, exporterCounter(t, reader, sentLogRecordsMetric))
}

func TestLogsExporter_SyncSender_RetryableError(t *testing.T) {
	setSyncSenderGate(t, true)
	intake := newFakeLogsIntake(t, alwaysStatus(http.StatusServiceUnavailable))
	exp, reader := newTestLogsExporter(t, startTestLogsAgent(t, testLogsAgentConfig(t, intake.URL, nil)), noRetry())

	err := exp.ConsumeLogs(context.Background(), testLogs(10))

	require.Error(t, err)
	assert.False(t, consumererror.IsPermanent(err), "503 must stay retryable: %v", err)
	assert.Contains(t, err.Error(), "503")
	assert.Equal(t, int64(10), exporterCounter(t, reader, sendFailedLogRecordsMetric))
	assert.Zero(t, exporterCounter(t, reader, sentLogRecordsMetric))
}

func TestLogsExporter_SyncSender_RetriesTransientErrors(t *testing.T) {
	setSyncSenderGate(t, true)
	intake := newFakeLogsIntake(t, func(payload int64) int {
		if payload <= 2 {
			return http.StatusServiceUnavailable
		}
		return http.StatusOK
	})
	exp, reader := newTestLogsExporter(t, startTestLogsAgent(t, testLogsAgentConfig(t, intake.URL, nil)), fastRetry())

	require.NoError(t, exp.ConsumeLogs(context.Background(), testLogs(10)))

	assert.Equal(t, int64(3), intake.payloads.Load())
	assert.Equal(t, int64(10), intake.delivered.Load())
	assert.Equal(t, int64(10), exporterCounter(t, reader, sentLogRecordsMetric))
	assert.Zero(t, exporterCounter(t, reader, sendFailedLogRecordsMetric))
}

func TestLogsExporter_SyncSender_WarnsThatMultiRegionFailoverIsNotSupported(t *testing.T) {
	setSyncSenderGate(t, true)
	intake := newFakeLogsIntake(t, alwaysStatus(http.StatusOK))
	for _, tt := range []struct {
		mrfEnabled bool
		warnings   int
	}{
		{mrfEnabled: false, warnings: 0},
		{mrfEnabled: true, warnings: 1},
	} {
		t.Run(fmt.Sprintf("multi_region_failover.enabled=%t", tt.mrfEnabled), func(t *testing.T) {
			core, logs := observer.New(zap.WarnLevel)
			set := exportertest.NewNopSettings(Type)
			set.Logger = zap.New(core)
			coreCfg := testLogsAgentConfig(t, intake.URL, map[string]interface{}{
				"multi_region_failover.enabled": tt.mrfEnabled,
				"multi_region_failover.api_key": "cccccccccccccccccccccccccccccccc",
				"multi_region_failover.dd_url":  intake.URL,
			})
			f := NewFactory(nil, nil, startTestLogsAgent(t, coreCfg), sourceProvider, nil, otel.NewDisabledGatewayUsage(), serializerexporter.TelemetryStore{}, coreCfg)
			cfg := f.CreateDefaultConfig().(*datadogconfig.Config)
			cfg.API.Key = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			cfg.HostMetadata.Enabled = false

			exp, err := f.CreateLogs(context.Background(), set, cfg)
			require.NoError(t, err)
			require.NoError(t, exp.Start(context.Background(), componenttest.NewNopHost()))
			require.NoError(t, exp.Shutdown(context.Background()))

			assert.Equal(t, tt.warnings, logs.FilterMessageSnippet("multi_region_failover").Len())
		})
	}
}
