// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package logsagentexporter

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/otel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

func TestLogsExporter(t *testing.T) {
	lr := testutil.GenerateLogsOneLogRecord()
	ld := lr.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)

	type args struct {
		ld            plog.Logs
		otelSource    string
		logSourceName string
	}
	tests := []struct {
		name         string
		args         args
		want         testutil.JSONLogs
		expectedTags [][]string
	}{
		{
			name: "message",
			args: args{
				ld:            lr,
				otelSource:    otelSource,
				logSourceName: LogSourceName,
			},

			want: testutil.JSONLogs{
				{
					"message":              ld.Body().AsString(),
					"app":                  "server",
					"instance_num":         float64(1),
					"@timestamp":           testutil.TestLogTime.Format("2006-01-02T15:04:05.000Z07:00"),
					"status":               "Info",
					"dd.span_id":           strconv.FormatUint(spanIDToUint64(ld.SpanID()), 10),
					"dd.trace_id":          strconv.FormatUint(traceIDToUint64(ld.TraceID()), 10),
					"otel.severity_text":   "Info",
					"otel.severity_number": "9",
					"otel.span_id":         spanIDToHexOrEmptyString(ld.SpanID()),
					"otel.trace_id":        traceIDToHexOrEmptyString(ld.TraceID()),
					"otel.timestamp":       strconv.FormatInt(testutil.TestLogTime.UnixNano(), 10),
					"resource-attr":        "resource-attr-val-1",
				},
			},
			expectedTags: [][]string{{"otel_source:datadog_agent"}},
		},
		{
			name: "message-attribute",
			args: args{
				ld: func() plog.Logs {
					lrr := testutil.GenerateLogsOneLogRecord()
					ldd := lrr.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
					ldd.Attributes().PutStr("message", "hello")
					ldd.Attributes().PutStr("datadog.log.source", "custom_source")
					ldd.Attributes().PutStr("host.name", "test-host")
					return lrr
				}(),
				otelSource:    otelSource,
				logSourceName: LogSourceName,
			},

			want: testutil.JSONLogs{
				{
					"message":              "hello",
					"app":                  "server",
					"instance_num":         float64(1),
					"datadog.log.source":   "custom_source",
					"@timestamp":           testutil.TestLogTime.Format("2006-01-02T15:04:05.000Z07:00"),
					"status":               "Info",
					"dd.span_id":           strconv.FormatUint(spanIDToUint64(ld.SpanID()), 10),
					"dd.trace_id":          strconv.FormatUint(traceIDToUint64(ld.TraceID()), 10),
					"otel.severity_text":   "Info",
					"otel.severity_number": "9",
					"otel.span_id":         spanIDToHexOrEmptyString(ld.SpanID()),
					"otel.trace_id":        traceIDToHexOrEmptyString(ld.TraceID()),
					"otel.timestamp":       strconv.FormatInt(testutil.TestLogTime.UnixNano(), 10),
					"resource-attr":        "resource-attr-val-1",
					"host.name":            "test-host",
					"hostname":             "test-host",
				},
			},
			expectedTags: [][]string{{"otel_source:datadog_agent"}},
		},
		{
			name: "resource-attribute-source",
			args: args{
				ld: func() plog.Logs {
					l := testutil.GenerateLogsOneLogRecord()
					rl := l.ResourceLogs().At(0)
					resourceAttrs := rl.Resource().Attributes()
					resourceAttrs.PutStr("datadog.log.source", "custom_source_rattr")
					return l
				}(),
				otelSource:    otelSource,
				logSourceName: LogSourceName,
			},

			want: testutil.JSONLogs{
				{
					"message":              "This is a log message",
					"app":                  "server",
					"instance_num":         float64(1),
					"datadog.log.source":   "custom_source_rattr",
					"@timestamp":           testutil.TestLogTime.Format("2006-01-02T15:04:05.000Z07:00"),
					"status":               "Info",
					"dd.span_id":           strconv.FormatUint(spanIDToUint64(ld.SpanID()), 10),
					"dd.trace_id":          strconv.FormatUint(traceIDToUint64(ld.TraceID()), 10),
					"otel.severity_text":   "Info",
					"otel.severity_number": "9",
					"otel.span_id":         spanIDToHexOrEmptyString(ld.SpanID()),
					"otel.trace_id":        traceIDToHexOrEmptyString(ld.TraceID()),
					"otel.timestamp":       strconv.FormatInt(testutil.TestLogTime.UnixNano(), 10),
					"resource-attr":        "resource-attr-val-1",
				},
			},
			expectedTags: [][]string{{"otel_source:datadog_agent"}},
		},
		{
			name: "status",
			args: args{
				ld: func() plog.Logs {
					l := testutil.GenerateLogsOneLogRecord()
					rl := l.ResourceLogs().At(0)
					rl.ScopeLogs().At(0).LogRecords().At(0).SetSeverityText("Fatal")
					return l
				}(),
				otelSource:    otelSource,
				logSourceName: LogSourceName,
			},

			want: testutil.JSONLogs{
				{
					"message":              "This is a log message",
					"app":                  "server",
					"instance_num":         float64(1),
					"@timestamp":           testutil.TestLogTime.Format("2006-01-02T15:04:05.000Z07:00"),
					"status":               "Fatal",
					"dd.span_id":           strconv.FormatUint(spanIDToUint64(ld.SpanID()), 10),
					"dd.trace_id":          strconv.FormatUint(traceIDToUint64(ld.TraceID()), 10),
					"otel.severity_text":   "Fatal",
					"otel.severity_number": "9",
					"otel.span_id":         spanIDToHexOrEmptyString(ld.SpanID()),
					"otel.trace_id":        traceIDToHexOrEmptyString(ld.TraceID()),
					"otel.timestamp":       strconv.FormatInt(testutil.TestLogTime.UnixNano(), 10),
					"resource-attr":        "resource-attr-val-1",
				},
			},
			expectedTags: [][]string{{"otel_source:datadog_agent"}},
		},
		{
			name: "ddtags",
			args: args{
				ld: func() plog.Logs {
					lrr := testutil.GenerateLogsOneLogRecord()
					ldd := lrr.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
					ldd.Attributes().PutStr("ddtags", "tag1:true")
					return lrr
				}(),
				otelSource:    otelSource,
				logSourceName: LogSourceName,
			},

			want: testutil.JSONLogs{
				{
					"message":              ld.Body().AsString(),
					"app":                  "server",
					"instance_num":         float64(1),
					"@timestamp":           testutil.TestLogTime.Format("2006-01-02T15:04:05.000Z07:00"),
					"status":               "Info",
					"dd.span_id":           strconv.FormatUint(spanIDToUint64(ld.SpanID()), 10),
					"dd.trace_id":          strconv.FormatUint(traceIDToUint64(ld.TraceID()), 10),
					"otel.severity_text":   "Info",
					"otel.severity_number": "9",
					"otel.span_id":         spanIDToHexOrEmptyString(ld.SpanID()),
					"otel.trace_id":        traceIDToHexOrEmptyString(ld.TraceID()),
					"otel.timestamp":       strconv.FormatInt(testutil.TestLogTime.UnixNano(), 10),
					"resource-attr":        "resource-attr-val-1",
				},
			},
			expectedTags: [][]string{{"tag1:true", "otel_source:datadog_agent"}},
		},
		{
			name: "ddtags submits same tags",
			args: args{
				ld: func() plog.Logs {
					lrr := testutil.GenerateLogsTwoLogRecordsSameResource()
					ldd := lrr.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
					ldd.Attributes().PutStr("ddtags", "tag1:true")
					ldd2 := lrr.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(1)
					ldd2.Attributes().PutStr("ddtags", "tag1:true")
					return lrr
				}(),
				otelSource:    otelSource,
				logSourceName: LogSourceName,
			},

			want: testutil.JSONLogs{
				{
					"message":              ld.Body().AsString(),
					"app":                  "server",
					"instance_num":         float64(1),
					"@timestamp":           testutil.TestLogTime.Format("2006-01-02T15:04:05.000Z07:00"),
					"status":               "Info",
					"dd.span_id":           strconv.FormatUint(spanIDToUint64(ld.SpanID()), 10),
					"dd.trace_id":          strconv.FormatUint(traceIDToUint64(ld.TraceID()), 10),
					"otel.severity_text":   "Info",
					"otel.severity_number": "9",
					"otel.span_id":         spanIDToHexOrEmptyString(ld.SpanID()),
					"otel.trace_id":        traceIDToHexOrEmptyString(ld.TraceID()),
					"otel.timestamp":       strconv.FormatInt(testutil.TestLogTime.UnixNano(), 10),
					"resource-attr":        "resource-attr-val-1",
				},
				{
					"message":              "something happened",
					"env":                  "dev",
					"customer":             "acme",
					"@timestamp":           testutil.TestLogTime.Format("2006-01-02T15:04:05.000Z07:00"),
					"status":               "Info",
					"otel.severity_text":   "Info",
					"otel.severity_number": "9",
					"otel.timestamp":       strconv.FormatInt(testutil.TestLogTime.UnixNano(), 10),
					"resource-attr":        "resource-attr-val-1",
				},
			},
			expectedTags: [][]string{{"tag1:true", "otel_source:datadog_agent"}, {"tag1:true", "otel_source:datadog_agent"}},
		},
		{
			name: "ddtags submits different tags",
			args: args{
				ld: func() plog.Logs {
					lrr := testutil.GenerateLogsTwoLogRecordsSameResource()
					ldd := lrr.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
					ldd.Attributes().PutStr("ddtags", "tag1:true")
					ldd2 := lrr.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(1)
					ldd2.Attributes().PutStr("ddtags", "tag2:true")
					return lrr
				}(),
				otelSource:    "datadog_exporter",
				logSourceName: "",
			},

			want: testutil.JSONLogs{
				{
					"message":              ld.Body().AsString(),
					"app":                  "server",
					"instance_num":         float64(1),
					"@timestamp":           testutil.TestLogTime.Format("2006-01-02T15:04:05.000Z07:00"),
					"status":               "Info",
					"dd.span_id":           strconv.FormatUint(spanIDToUint64(ld.SpanID()), 10),
					"dd.trace_id":          strconv.FormatUint(traceIDToUint64(ld.TraceID()), 10),
					"otel.severity_text":   "Info",
					"otel.severity_number": "9",
					"otel.span_id":         spanIDToHexOrEmptyString(ld.SpanID()),
					"otel.trace_id":        traceIDToHexOrEmptyString(ld.TraceID()),
					"otel.timestamp":       strconv.FormatInt(testutil.TestLogTime.UnixNano(), 10),
					"resource-attr":        "resource-attr-val-1",
				},
				{
					"message":              "something happened",
					"env":                  "dev",
					"customer":             "acme",
					"@timestamp":           testutil.TestLogTime.Format("2006-01-02T15:04:05.000Z07:00"),
					"status":               "Info",
					"otel.severity_text":   "Info",
					"otel.severity_number": "9",
					"otel.timestamp":       strconv.FormatInt(testutil.TestLogTime.UnixNano(), 10),
					"resource-attr":        "resource-attr-val-1",
				},
			},
			expectedTags: [][]string{{"tag1:true", "otel_source:datadog_exporter"}, {"tag2:true", "otel_source:datadog_exporter"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testChannel := make(chan *message.Message, 10)

			params := exportertest.NewNopSettings(component.MustNewType(TypeStr))
			f := NewFactory(testChannel, otel.NewDisabledGatewayUsage())
			cfg := &Config{
				OtelSource:    tt.args.otelSource,
				LogSourceName: tt.args.logSourceName,
			}
			ctx := context.Background()
			exp, err := f.CreateLogs(ctx, params, cfg)

			require.NoError(t, err)
			require.NoError(t, exp.ConsumeLogs(ctx, tt.args.ld))

			ans := testutil.JSONLogs{}
			for i := 0; i < len(tt.want); i++ {
				output := <-testChannel
				outputJSON := make(map[string]interface{})
				json.Unmarshal(output.GetContent(), &outputJSON)
				if src, ok := outputJSON["datadog.log.source"]; ok {
					assert.Equal(t, src, output.Origin.Source())
				} else {
					assert.Equal(t, tt.args.logSourceName, output.Origin.Source())
				}
				assert.Equal(t, tt.expectedTags[i], output.Origin.Tags())
				ans = append(ans, outputJSON)
			}
			assert.Equal(t, tt.want, ans)
			close(testChannel)
		})
	}
}

func TestLogsExporterContextCancelled(t *testing.T) {
	// Unbuffered channel: sends always block when there is no reader.
	testChannel := make(chan *message.Message)

	params := exportertest.NewNopSettings(component.MustNewType(TypeStr))
	f := NewFactory(testChannel, otel.NewDisabledGatewayUsage())
	cfg := &Config{
		OtelSource:    otelSource,
		LogSourceName: LogSourceName,
	}
	ctx := context.Background()
	exp, err := f.CreateLogs(ctx, params, cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(ctx)
	cancel()

	lr := testutil.GenerateLogsOneLogRecord()
	err = exp.ConsumeLogs(ctx, lr)
	// The scrubber replaces the error with errors.New(string) which breaks
	// the error chain, so we check the string instead of using errors.Is.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
	assert.Contains(t, err.Error(), "log records remaining")
}

func TestLogsExporterContextCancelledPartialDelivery(t *testing.T) {
	// Buffer of 1: first log is accepted, second blocks and sees cancelled context.
	testChannel := make(chan *message.Message, 1)

	params := exportertest.NewNopSettings(component.MustNewType(TypeStr))
	f := NewFactory(testChannel, otel.NewDisabledGatewayUsage())
	cfg := &Config{
		OtelSource:    otelSource,
		LogSourceName: LogSourceName,
	}
	ctx := context.Background()
	exp, err := f.CreateLogs(ctx, params, cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(ctx)
	cancel()

	lr := testutil.GenerateLogsTwoLogRecordsSameResource()
	err = exp.ConsumeLogs(ctx, lr)

	// With a cancelled context and buffer of 1, the select is non-deterministic:
	// either both sends hit ctx.Done(), or the first succeeds and the second fails.
	// In both cases we must get an error.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
	assert.Contains(t, err.Error(), "log records remaining")
}

// traceIDToUint64 converts 128bit traceId to 64 bit uint64
func traceIDToUint64(b [16]byte) uint64 {
	return binary.BigEndian.Uint64(b[len(b)-8:])
}

// spanIDToUint64 converts byte array to uint64
func spanIDToUint64(b [8]byte) uint64 {
	return binary.BigEndian.Uint64(b[:])
}

// spanIDToHexOrEmptyString returns a hex string from SpanID.
// An empty string is returned, if SpanID is empty.
func spanIDToHexOrEmptyString(id pcommon.SpanID) string {
	if id.IsEmpty() {
		return ""
	}
	return hex.EncodeToString(id[:])
}

// traceIDToHexOrEmptyString returns a hex string from TraceID.
// An empty string is returned, if TraceID is empty.
func traceIDToHexOrEmptyString(id pcommon.TraceID) string {
	if id.IsEmpty() {
		return ""
	}
	return hex.EncodeToString(id[:])
}

func TestSplitLogsByScope(t *testing.T) {
	t.Run("single-scope ResourceLogs", func(t *testing.T) {
		ld := plog.NewLogs()
		for _, scope := range []string{"filelog", string(K8sObjectsReceiver), "filelog", string(K8sObjectsReceiver)} {
			ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().Scope().SetName(scope)
		}

		k8s, regular := splitLogsByScope(ld)
		assert.Equal(t, 2, k8s.ResourceLogs().Len())
		assert.Equal(t, 2, regular.ResourceLogs().Len())
		assert.Equal(t, string(K8sObjectsReceiver), k8s.ResourceLogs().At(0).ScopeLogs().At(0).Scope().Name())
		assert.Equal(t, "filelog", regular.ResourceLogs().At(0).ScopeLogs().At(0).Scope().Name())
	})

	t.Run("mixed-scope ResourceLogs", func(t *testing.T) {
		ld := plog.NewLogs()
		rl := ld.ResourceLogs().AppendEmpty()
		rl.SetSchemaUrl("https://example.com/schema")
		rl.Resource().Attributes().PutStr("k8s.cluster.name", "test-cluster")
		rl.ScopeLogs().AppendEmpty().Scope().SetName("filelog")
		rl.ScopeLogs().AppendEmpty().Scope().SetName(string(K8sObjectsReceiver))
		rl.ScopeLogs().AppendEmpty().Scope().SetName("filelog")

		k8s, regular := splitLogsByScope(ld)

		assert.Equal(t, 1, k8s.ResourceLogs().Len())
		assert.Equal(t, 1, k8s.ResourceLogs().At(0).ScopeLogs().Len())
		assert.Equal(t, string(K8sObjectsReceiver), k8s.ResourceLogs().At(0).ScopeLogs().At(0).Scope().Name())
		assert.Equal(t, "https://example.com/schema", k8s.ResourceLogs().At(0).SchemaUrl())
		clusterName, ok := k8s.ResourceLogs().At(0).Resource().Attributes().Get("k8s.cluster.name")
		assert.True(t, ok)
		assert.Equal(t, "test-cluster", clusterName.AsString())

		assert.Equal(t, 1, regular.ResourceLogs().Len())
		assert.Equal(t, 2, regular.ResourceLogs().At(0).ScopeLogs().Len())
		assert.Equal(t, "filelog", regular.ResourceLogs().At(0).ScopeLogs().At(0).Scope().Name())
		assert.Equal(t, "filelog", regular.ResourceLogs().At(0).ScopeLogs().At(1).Scope().Name())
		assert.Equal(t, "https://example.com/schema", regular.ResourceLogs().At(0).SchemaUrl())
	})

	t.Run("only k8sobjects scope", func(t *testing.T) {
		ld := plog.NewLogs()
		ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().Scope().SetName(string(K8sObjectsReceiver))

		k8s, regular := splitLogsByScope(ld)
		assert.Equal(t, 1, k8s.ResourceLogs().Len())
		assert.Equal(t, 0, regular.ResourceLogs().Len())
	})

	t.Run("all-regular fast path returns ld unchanged", func(t *testing.T) {
		ld := plog.NewLogs()
		ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().Scope().SetName("filelog")
		ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().Scope().SetName("otherreceiver")

		k8s, regular := splitLogsByScope(ld)
		assert.Equal(t, 0, k8s.ResourceLogs().Len())
		assert.Equal(t, 2, regular.ResourceLogs().Len())
		// fast path returns the same backing plog.Logs (no copy)
		assert.Equal(t, ld, regular)
	})

	t.Run("scopeless ResourceLogs routed to regular side", func(t *testing.T) {
		ld := plog.NewLogs()
		scopeless := ld.ResourceLogs().AppendEmpty()
		scopeless.SetSchemaUrl("https://example.com/schema")
		scopeless.Resource().Attributes().PutStr("host.name", "host-a")
		ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().Scope().SetName(string(K8sObjectsReceiver))

		k8s, regular := splitLogsByScope(ld)
		assert.Equal(t, 1, k8s.ResourceLogs().Len())
		assert.Equal(t, 1, regular.ResourceLogs().Len())
		assert.Equal(t, 0, regular.ResourceLogs().At(0).ScopeLogs().Len())
		assert.Equal(t, "https://example.com/schema", regular.ResourceLogs().At(0).SchemaUrl())
		hostName, ok := regular.ResourceLogs().At(0).Resource().Attributes().Get("host.name")
		assert.True(t, ok)
		assert.Equal(t, "host-a", hostName.AsString())
	})

	t.Run("scopeless ResourceLogs only takes fast path as regular", func(t *testing.T) {
		ld := plog.NewLogs()
		ld.ResourceLogs().AppendEmpty().Resource().Attributes().PutStr("host.name", "host-a")

		k8s, regular := splitLogsByScope(ld)
		assert.Equal(t, 0, k8s.ResourceLogs().Len())
		assert.Equal(t, 1, regular.ResourceLogs().Len())
		assert.Equal(t, ld, regular)
	})
}
