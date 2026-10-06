// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package logsagentexporter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/plog"

	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/util/otel"
)

// fakeSyncSender records the messages it is given and answers with the errors of errsFor.
type fakeSyncSender struct {
	errsFor func(msgs []*message.Message) []error
	sent    [][]*message.Message
}

func (s *fakeSyncSender) Send(_ context.Context, msgs []*message.Message) []error {
	s.sent = append(s.sent, msgs)
	if s.errsFor == nil {
		return make([]error, len(msgs))
	}
	return s.errsFor(msgs)
}

func newSyncExporter(t *testing.T, sender SyncSender) *Exporter {
	t.Helper()
	set := componenttest.NewNopTelemetrySettings()
	translator, err := attributes.NewTranslator(set)
	require.NoError(t, err)
	e, err := NewExporterWithSyncSender(set, &Config{OtelSource: otelSource, LogSourceName: LogSourceName},
		sources.NewLogSource(LogSourceName, &config.LogsConfig{}), sender, translator)
	require.NoError(t, err)
	return e
}

// logsWithBodies returns logs whose records have the given bodies, spread over two resources.
func logsWithBodies(bodies ...string) plog.Logs {
	ld := plog.NewLogs()
	for i, body := range bodies {
		if i%2 == 0 {
			ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty()
		}
		rl := ld.ResourceLogs().At(ld.ResourceLogs().Len() - 1)
		rl.Resource().Attributes().PutStr("service.name", fmt.Sprintf("service-%d", i/2))
		rl.ScopeLogs().At(0).LogRecords().AppendEmpty().Body().SetStr(body)
	}
	return ld
}

func messageBody(t *testing.T, msg *message.Message) string {
	t.Helper()
	var content struct {
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(msg.GetContent(), &content))
	return content.Message
}

func recordBodies(ld plog.Logs) []string {
	var bodies []string
	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		sls := ld.ResourceLogs().At(i).ScopeLogs()
		for j := 0; j < sls.Len(); j++ {
			lrs := sls.At(j).LogRecords()
			for k := 0; k < lrs.Len(); k++ {
				bodies = append(bodies, lrs.At(k).Body().AsString())
			}
		}
	}
	return bodies
}

func TestSyncSenderDelivers(t *testing.T) {
	sender := &fakeSyncSender{}
	e := newSyncExporter(t, sender)

	require.NoError(t, e.ConsumeLogs(context.Background(), logsWithBodies("a", "b", "c")))

	require.Len(t, sender.sent, 1)
	bodies := make([]string, len(sender.sent[0]))
	for i, msg := range sender.sent[0] {
		bodies[i] = messageBody(t, msg)
	}
	assert.Equal(t, []string{"a", "b", "c"}, bodies)
}

func TestSyncSenderPermanentErrors(t *testing.T) {
	apiKey := "abcdefabcdefabcdefabcdefabcdefab"
	rejected := fmt.Errorf("client error: 403 Forbidden for api_key=%s", apiKey)
	e := newSyncExporter(t, &fakeSyncSender{errsFor: func(msgs []*message.Message) []error {
		errs := make([]error, len(msgs))
		for i := range errs {
			errs[i] = rejected
		}
		return errs
	}})

	err := e.ConsumeLogs(context.Background(), logsWithBodies("a", "b"))

	require.Error(t, err)
	assert.True(t, consumererror.IsPermanent(err), "an error that is not retryable must be permanent: %v", err)
	assert.Contains(t, err.Error(), "failed to send 2 of 2 log records")
	assert.Contains(t, err.Error(), "403 Forbidden")
	assert.NotContains(t, err.Error(), apiKey, "the error must be scrubbed")
}

func TestSyncSenderRetriesOnlyFailedRecords(t *testing.T) {
	unavailable := client.NewRetryableError(errors.New("server error: 503 Service Unavailable"))
	e := newSyncExporter(t, &fakeSyncSender{errsFor: func(msgs []*message.Message) []error {
		errs := make([]error, len(msgs))
		for i, msg := range msgs {
			if body := messageBody(t, msg); body == "b" || body == "d" {
				errs[i] = unavailable
			}
		}
		return errs
	}})

	err := e.ConsumeLogs(context.Background(), logsWithBodies("a", "b", "c", "d"))

	require.Error(t, err)
	assert.False(t, consumererror.IsPermanent(err))
	var logsErr consumererror.Logs
	require.ErrorAs(t, err, &logsErr)
	assert.Equal(t, []string{"b", "d"}, recordBodies(logsErr.Data()))
	assert.Equal(t, 2, logsErr.Data().ResourceLogs().Len(), "records keep their resource")
}

func TestSyncSenderContextErrorsAreRetryable(t *testing.T) {
	e := newSyncExporter(t, &fakeSyncSender{errsFor: func([]*message.Message) []error {
		return []error{nil, fmt.Errorf("sending payload: %w", context.DeadlineExceeded)}
	}})

	err := e.ConsumeLogs(context.Background(), logsWithBodies("a", "b"))

	var logsErr consumererror.Logs
	require.ErrorAs(t, err, &logsErr)
	assert.Equal(t, []string{"b"}, recordBodies(logsErr.Data()))
}

func TestSyncSenderThroughExporterHelper(t *testing.T) {
	sender := &fakeSyncSender{errsFor: func(msgs []*message.Message) []error {
		errs := make([]error, len(msgs))
		for i := range errs {
			errs[i] = errors.New("client error: 400 Bad Request")
		}
		return errs
	}}
	f := NewFactoryWithSyncSender(sender, component.MustNewType(TypeStr), otel.NewDisabledGatewayUsage(), nil, nil)
	cfg := f.CreateDefaultConfig().(*Config)
	cfg.QueueSettings = configoptional.None[exporterhelper.QueueBatchConfig]()
	cfg.RetryConfig = configretry.BackOffConfig{Enabled: true, InitialInterval: time.Millisecond, MaxInterval: time.Millisecond, MaxElapsedTime: time.Second}
	exp, err := f.CreateLogs(context.Background(), exportertest.NewNopSettings(component.MustNewType(TypeStr)), cfg)
	require.NoError(t, err)
	require.NoError(t, exp.Start(context.Background(), componenttest.NewNopHost()))
	defer func() { require.NoError(t, exp.Shutdown(context.Background())) }()

	err = exp.ConsumeLogs(context.Background(), logsWithBodies("a"))

	require.Error(t, err)
	assert.True(t, consumererror.IsPermanent(err))
	assert.Len(t, sender.sent, 1, "exporterhelper must not retry a permanent error")
}

func TestSyncSenderQueueConfig(t *testing.T) {
	defaults := exporterhelper.NewDefaultQueueConfig()

	t.Run("disabled queue stays disabled", func(t *testing.T) {
		disabled := configoptional.None[exporterhelper.QueueBatchConfig]()
		assert.False(t, SyncSenderQueueConfig(disabled, 40, 1000).HasValue())
	})

	t.Run("defaults keep the concurrency and payload size of the pipeline", func(t *testing.T) {
		got := SyncSenderQueueConfig(configoptional.Some(defaults), 40, 1000)

		require.True(t, got.HasValue())
		assert.Equal(t, 40, got.Get().NumConsumers)
		assert.Equal(t, defaults.QueueSize, got.Get().QueueSize)
		require.True(t, got.Get().Batch.HasValue())
		assert.Equal(t, exporterhelper.BatchConfig{
			FlushTimeout: syncBatchFlushTimeout,
			Sizer:        exporterhelper.RequestSizerTypeItems,
			MinSize:      1000,
			MaxSize:      1000,
		}, *got.Get().Batch.Get())
	})

	t.Run("explicit settings are kept", func(t *testing.T) {
		custom := defaults
		custom.NumConsumers = 5
		batch := exporterhelper.BatchConfig{FlushTimeout: time.Second, Sizer: exporterhelper.RequestSizerTypeItems, MinSize: 10, MaxSize: 100}
		custom.Batch = configoptional.Some(batch)

		got := SyncSenderQueueConfig(configoptional.Some(custom), 40, 1000)

		assert.Equal(t, 5, got.Get().NumConsumers)
		assert.Equal(t, batch, *got.Get().Batch.Get())
	})
}
