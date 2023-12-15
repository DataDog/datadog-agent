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
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/internal/testutil"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
)

func TestLogsExporter(t *testing.T) {
	lr := testutil.GenerateLogsOneLogRecord()
	ld := lr.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)

	type args struct {
		ld plog.Logs
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
				ld: lr,
			},

			want: testutil.JSONLogs{
				{
					"message":              ld.Body().AsString(),
					"app":                  "server",
					"instance_num":         "1",
					"@timestamp":           testutil.TestLogTime.Format(time.RFC3339),
					"status":               "Info",
					"dd.span_id":           fmt.Sprintf("%d", spanIDToUint64(ld.SpanID())),
					"dd.trace_id":          fmt.Sprintf("%d", traceIDToUint64(ld.TraceID())),
					"otel.severity_text":   "Info",
					"otel.severity_number": "9",
					"otel.span_id":         spanIDToHexOrEmptyString(ld.SpanID()),
					"otel.trace_id":        traceIDToHexOrEmptyString(ld.TraceID()),
					"otel.timestamp":       fmt.Sprintf("%d", testutil.TestLogTime.UnixNano()),
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
					return lrr
				}(),
			},

			want: testutil.JSONLogs{
				{
					"message":              "hello",
					"app":                  "server",
					"instance_num":         "1",
					"@timestamp":           testutil.TestLogTime.Format(time.RFC3339),
					"status":               "Info",
					"dd.span_id":           fmt.Sprintf("%d", spanIDToUint64(ld.SpanID())),
					"dd.trace_id":          fmt.Sprintf("%d", traceIDToUint64(ld.TraceID())),
					"otel.severity_text":   "Info",
					"otel.severity_number": "9",
					"otel.span_id":         spanIDToHexOrEmptyString(ld.SpanID()),
					"otel.trace_id":        traceIDToHexOrEmptyString(ld.TraceID()),
					"otel.timestamp":       fmt.Sprintf("%d", testutil.TestLogTime.UnixNano()),
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
			},

			want: testutil.JSONLogs{
				{
					"message":              ld.Body().AsString(),
					"app":                  "server",
					"instance_num":         "1",
					"@timestamp":           testutil.TestLogTime.Format(time.RFC3339),
					"status":               "Info",
					"dd.span_id":           fmt.Sprintf("%d", spanIDToUint64(ld.SpanID())),
					"dd.trace_id":          fmt.Sprintf("%d", traceIDToUint64(ld.TraceID())),
					"otel.severity_text":   "Info",
					"otel.severity_number": "9",
					"otel.span_id":         spanIDToHexOrEmptyString(ld.SpanID()),
					"otel.trace_id":        traceIDToHexOrEmptyString(ld.TraceID()),
					"otel.timestamp":       fmt.Sprintf("%d", testutil.TestLogTime.UnixNano()),
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
			},

			want: testutil.JSONLogs{
				{
					"message":              ld.Body().AsString(),
					"app":                  "server",
					"instance_num":         "1",
					"@timestamp":           testutil.TestLogTime.Format(time.RFC3339),
					"status":               "Info",
					"dd.span_id":           fmt.Sprintf("%d", spanIDToUint64(ld.SpanID())),
					"dd.trace_id":          fmt.Sprintf("%d", traceIDToUint64(ld.TraceID())),
					"otel.severity_text":   "Info",
					"otel.severity_number": "9",
					"otel.span_id":         spanIDToHexOrEmptyString(ld.SpanID()),
					"otel.trace_id":        traceIDToHexOrEmptyString(ld.TraceID()),
					"otel.timestamp":       fmt.Sprintf("%d", testutil.TestLogTime.UnixNano()),
					"resource-attr":        "resource-attr-val-1",
				},
				{
					"message":              "something happened",
					"env":                  "dev",
					"customer":             "acme",
					"@timestamp":           testutil.TestLogTime.Format(time.RFC3339),
					"status":               "Info",
					"otel.severity_text":   "Info",
					"otel.severity_number": "9",
					"otel.timestamp":       fmt.Sprintf("%d", testutil.TestLogTime.UnixNano()),
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
			},

			want: testutil.JSONLogs{
				{
					"message":              ld.Body().AsString(),
					"app":                  "server",
					"instance_num":         "1",
					"@timestamp":           testutil.TestLogTime.Format(time.RFC3339),
					"status":               "Info",
					"dd.span_id":           fmt.Sprintf("%d", spanIDToUint64(ld.SpanID())),
					"dd.trace_id":          fmt.Sprintf("%d", traceIDToUint64(ld.TraceID())),
					"otel.severity_text":   "Info",
					"otel.severity_number": "9",
					"otel.span_id":         spanIDToHexOrEmptyString(ld.SpanID()),
					"otel.trace_id":        traceIDToHexOrEmptyString(ld.TraceID()),
					"otel.timestamp":       fmt.Sprintf("%d", testutil.TestLogTime.UnixNano()),
					"resource-attr":        "resource-attr-val-1",
				},
				{
					"message":              "something happened",
					"env":                  "dev",
					"customer":             "acme",
					"@timestamp":           testutil.TestLogTime.Format(time.RFC3339),
					"status":               "Info",
					"otel.severity_text":   "Info",
					"otel.severity_number": "9",
					"otel.timestamp":       fmt.Sprintf("%d", testutil.TestLogTime.UnixNano()),
					"resource-attr":        "resource-attr-val-1",
				},
			},
			expectedTags: [][]string{{"tag1:true", "otel_source:datadog_agent"}, {"tag2:true", "otel_source:datadog_agent"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &struct{}{}

			testChannel := make(chan *message.Message, 10)

			params := exportertest.NewNopCreateSettings()
			f := NewFactory(testChannel)
			ctx := context.Background()
			exp, err := f.CreateLogsExporter(ctx, params, cfg)

			require.NoError(t, err)
			require.NoError(t, exp.ConsumeLogs(ctx, tt.args.ld))

			ans := testutil.JSONLogs{}
			for i := 0; i < len(tt.want); i++ {
				output := <-testChannel
				outputJSON := make(map[string]interface{})
				json.Unmarshal(output.GetContent(), &outputJSON)
				assert.Equal(t, logSourceName, output.Origin.Source())
				assert.Equal(t, tt.expectedTags[i], output.Origin.Tags())
				ans = append(ans, outputJSON)
			}
			assert.Equal(t, tt.want, ans)
			close(testChannel)
		})
	}

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
