// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && otlp

package datadogexporter

import (
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/plog"

	datadogconfig "github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/datadogconfig"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/logsagentexporter"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/otel"
)

func TestSharedDIInsideLogsQueue(t *testing.T) {
	var attempts atomic.Int32
	intake := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if string(body) != `[{"service":"checkout","message":"capture"}]` {
			t.Errorf("unexpected native upload: %s", body)
		}
		if r.Header.Get("DD-EVP-ORIGIN") != "agent-debugger" {
			t.Error("incorrect debugger origin")
		}
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer intake.Close()
	cfg := CreateDefaultConfig().(*datadogconfig.Config)
	cfg.API.Key = "collector-key"
	cfg.DynamicInstrumentation.Enabled = true
	cfg.DynamicInstrumentation.Endpoint = intake.URL + "/api/v2/debugger"
	ca := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: intake.Certificate().Raw}), 0o600))
	cfg.ClientConfig.TLS.CAFile = ca
	channel := make(chan *message.Message, 4)
	factory := logsagentexporter.NewFactoryWithType(channel, component.MustNewType("datadog"), otel.NewDisabledGatewayUsage(), nil, nil, logsagentexporter.WithConsumerDecorator(diDecorator(cfg)))
	logsCfg := factory.CreateDefaultConfig().(*logsagentexporter.Config)
	logsCfg.QueueSettings = configoptional.None[exporterhelper.QueueBatchConfig]()
	logsCfg.RetryConfig.InitialInterval = time.Millisecond
	logsCfg.RetryConfig.MaxInterval = time.Millisecond
	logsCfg.RetryConfig.MaxElapsedTime = time.Second
	exp, err := factory.CreateLogs(t.Context(), exportertest.NewNopSettings(component.MustNewType("datadog")), logsCfg)
	require.NoError(t, err)
	require.NoError(t, exp.Start(t.Context(), componenttest.NewNopHost()))
	data := plog.NewLogs()
	resource := data.ResourceLogs().AppendEmpty()
	resource.Resource().Attributes().PutStr("service.name", "checkout")
	records := resource.ScopeLogs().AppendEmpty().LogRecords()
	records.AppendEmpty().Body().SetStr("ordinary log")
	record := records.AppendEmpty()
	record.SetEventName("dd.debugger.snapshot")
	record.Attributes().PutInt("datadog.di.schema_version", 1)
	record.Attributes().PutStr("datadog.di.payload", `{"service":"checkout","message":"capture"}`)
	require.NoError(t, exp.ConsumeLogs(t.Context(), data))
	require.Equal(t, int32(2), attempts.Load())
	require.Len(t, channel, 1, "successful ordinary logs must not be retried with DI")
	require.NoError(t, exp.Shutdown(t.Context()))
}
