// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	semconv16 "go.opentelemetry.io/otel/semconv/v1.6.1"
	"go.uber.org/zap/zaptest"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
)

type translatorTestCase struct {
	name string
	args struct {
		lr    plog.LogRecord
		res   pcommon.Resource
		scope pcommon.InstrumentationScope
	}
	want datadogV2.HTTPLogItem
}

type args struct {
	lr    plog.LogRecord
	res   pcommon.Resource
	scope pcommon.InstrumentationScope
}

func generateTranslatorTestCases(traceID [16]byte, spanID [8]byte, ddTr uint64, ddSp uint64) []translatorTestCase {
	return []translatorTestCase{
		{
			name: "basic",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().PutStr("app", "test")
					l.SetSeverityNumber(5)
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString(""),
				AdditionalProperties: map[string]interface{}{
					"app":              "test",
					"status":           "debug",
					otelSeverityNumber: "5",
				},
			},
		},
		{
			// log & resource with attribute
			name: "resource",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().PutStr("app", "test")
					l.SetSeverityNumber(5)
					return l
				}(),
				res: func() pcommon.Resource {
					r := pcommon.NewResource()
					r.Attributes().PutStr(string(semconv16.ServiceNameKey), "otlp_col")
					return r
				}(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("service:otlp_col,otel_source:test"),
				Message: *datadog.PtrString(""),
				Service: datadog.PtrString("otlp_col"),
				AdditionalProperties: map[string]interface{}{
					"app":              "test",
					"status":           "debug",
					otelSeverityNumber: "5",
					"service.name":     "otlp_col",
				},
			},
		},
		{
			// appends tags in attributes instead of replacing them
			name: "append tags",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().PutStr("app", "test")
					l.Attributes().PutStr("ddtags", "foo:bar")
					l.SetSeverityNumber(5)
					return l
				}(),
				res: func() pcommon.Resource {
					r := pcommon.NewResource()
					r.Attributes().PutStr(string(semconv16.ServiceNameKey), "otlp_col")
					return r
				}(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("service:otlp_col,foo:bar,otel_source:test"),
				Message: *datadog.PtrString(""),
				Service: datadog.PtrString("otlp_col"),
				AdditionalProperties: map[string]interface{}{
					"app":              "test",
					"status":           "debug",
					otelSeverityNumber: "5",
					"service.name":     "otlp_col",
				},
			},
		},
		{
			// service name from log
			name: "service",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().PutStr("app", "test")
					l.Attributes().PutStr(string(semconv16.ServiceNameKey), "otlp_col")
					l.SetSeverityNumber(5)
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString(""),
				Service: datadog.PtrString("otlp_col"),
				AdditionalProperties: map[string]interface{}{
					"app":              "test",
					"status":           "debug",
					otelSeverityNumber: "5",
					"service.name":     "otlp_col",
				},
			},
		},
		{
			name: "trace",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().PutStr("app", "test")
					l.SetSpanID(spanID)
					l.SetTraceID(traceID)
					l.Attributes().PutStr(string(semconv16.ServiceNameKey), "otlp_col")
					l.SetSeverityNumber(5)
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString(""),
				Service: datadog.PtrString("otlp_col"),
				AdditionalProperties: map[string]interface{}{
					"app":              "test",
					"status":           "debug",
					otelSeverityNumber: "5",
					otelSpanID:         fmt.Sprintf("%x", string(spanID[:])),
					otelTraceID:        fmt.Sprintf("%x", string(traceID[:])),
					ddSpanID:           strconv.FormatUint(ddSp, 10),
					ddTraceID:          strconv.FormatUint(ddTr, 10),
					"service.name":     "otlp_col",
				},
			},
		},
		{
			name: "trace from attributes",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().PutStr("app", "test")
					l.Attributes().PutStr("spanid", "2e26da881214cd7c")
					l.Attributes().PutStr("traceid", "437ab4d83468c540bb0f3398a39faa59")
					l.Attributes().PutStr(string(semconv16.ServiceNameKey), "otlp_col")
					l.SetSeverityNumber(5)
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString(""),
				Service: datadog.PtrString("otlp_col"),
				AdditionalProperties: map[string]interface{}{
					"app":              "test",
					"status":           "debug",
					otelSeverityNumber: "5",
					otelSpanID:         "2e26da881214cd7c",
					otelTraceID:        "437ab4d83468c540bb0f3398a39faa59",
					ddSpanID:           "3325585652813450620",
					ddTraceID:          "13479048940416379481",
					"service.name":     "otlp_col",
				},
			},
		},
		{
			name: "trace from attributes (underscore)",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().PutStr("app", "test")
					l.Attributes().PutStr("span_id", "2e26da881214cd7c")
					l.Attributes().PutStr("trace_id", "740112b325075be8c80a48de336ebc67")
					l.Attributes().PutStr(string(semconv16.ServiceNameKey), "otlp_col")
					l.SetSeverityNumber(5)
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString(""),
				Service: datadog.PtrString("otlp_col"),
				AdditionalProperties: map[string]interface{}{
					"app":              "test",
					"status":           "debug",
					otelSeverityNumber: "5",
					otelSpanID:         "2e26da881214cd7c",
					otelTraceID:        "740112b325075be8c80a48de336ebc67",
					ddSpanID:           "3325585652813450620",
					ddTraceID:          "14414413676535528551",
					"service.name":     "otlp_col",
				},
			},
		},
		{
			name: "trace from attributes decode error",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().PutStr("app", "test")
					l.Attributes().PutStr("spanid", "2e26da881214cd7c")
					l.Attributes().PutStr("traceid", "invalidtraceid")
					l.Attributes().PutStr(string(semconv16.ServiceNameKey), "otlp_col")
					l.SetSeverityNumber(5)
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString(""),
				Service: datadog.PtrString("otlp_col"),
				AdditionalProperties: map[string]interface{}{
					"app":              "test",
					"status":           "debug",
					otelSeverityNumber: "5",
					otelSpanID:         "2e26da881214cd7c",
					ddSpanID:           "3325585652813450620",
					"service.name":     "otlp_col",
				},
			},
		},
		{
			name: "trace from attributes size error",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().PutStr("app", "test")
					l.Attributes().PutStr("spanid", "2023675201651514964")
					l.Attributes().PutStr("traceid", "eb068afe5e53704f3b0dc3d3e1e397cb760549a7b58547db4f1dee845d9101f8db1ccf8fdd0976a9112f")
					l.Attributes().PutStr(string(semconv16.ServiceNameKey), "otlp_col")
					l.SetSeverityNumber(5)
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString(""),
				Service: datadog.PtrString("otlp_col"),
				AdditionalProperties: map[string]interface{}{
					"app":              "test",
					"status":           "debug",
					otelSeverityNumber: "5",
					"service.name":     "otlp_col",
				},
			},
		},
		{
			// here SeverityText should take precedence for log status
			name: "SeverityText",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().PutStr("app", "test")
					l.SetSpanID(spanID)
					l.SetTraceID(traceID)
					l.Attributes().PutStr(string(semconv16.ServiceNameKey), "otlp_col")
					l.SetSeverityText("alert")
					l.SetSeverityNumber(5)
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString(""),
				Service: datadog.PtrString("otlp_col"),
				AdditionalProperties: map[string]interface{}{
					"app":              "test",
					"status":           "alert",
					otelSeverityText:   "alert",
					otelSeverityNumber: "5",
					otelSpanID:         fmt.Sprintf("%x", string(spanID[:])),
					otelTraceID:        fmt.Sprintf("%x", string(traceID[:])),
					ddSpanID:           strconv.FormatUint(ddSp, 10),
					ddTraceID:          strconv.FormatUint(ddTr, 10),
					"service.name":     "otlp_col",
				},
			},
		},
		{
			name: "body",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().PutStr("app", "test")
					l.SetSpanID(spanID)
					l.SetTraceID(traceID)
					l.Attributes().PutStr(string(semconv16.ServiceNameKey), "otlp_col")
					l.SetSeverityNumber(13)
					l.Body().SetStr("This is log")
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString(""),
				Service: datadog.PtrString("otlp_col"),
				AdditionalProperties: map[string]interface{}{
					"message":          "This is log",
					"app":              "test",
					"status":           "warn",
					otelSeverityNumber: "13",
					otelSpanID:         fmt.Sprintf("%x", string(spanID[:])),
					otelTraceID:        fmt.Sprintf("%x", string(traceID[:])),
					ddSpanID:           strconv.FormatUint(ddSp, 10),
					ddTraceID:          strconv.FormatUint(ddTr, 10),
					"service.name":     "otlp_col",
				},
			},
		},
		{
			name: "log-level",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().PutStr("app", "test")
					l.SetSpanID(spanID)
					l.SetTraceID(traceID)
					l.Attributes().PutStr(string(semconv16.ServiceNameKey), "otlp_col")
					l.Attributes().PutStr("level", "error")
					l.Body().SetStr("This is log")
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString(""),
				Service: datadog.PtrString("otlp_col"),
				AdditionalProperties: map[string]interface{}{
					"message":      "This is log",
					"app":          "test",
					"status":       "error",
					otelSpanID:     fmt.Sprintf("%x", string(spanID[:])),
					otelTraceID:    fmt.Sprintf("%x", string(traceID[:])),
					ddSpanID:       strconv.FormatUint(ddSp, 10),
					ddTraceID:      strconv.FormatUint(ddTr, 10),
					"service.name": "otlp_col",
				},
			},
		},
		{
			name: "resource attributes in additional properties",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().PutStr("app", "test")
					l.SetSeverityNumber(5)
					return l
				}(),
				res: func() pcommon.Resource {
					r := pcommon.NewResource()
					r.Attributes().PutStr(string(semconv16.ServiceNameKey), "otlp_col")
					r.Attributes().PutStr("key", "val")
					return r
				}(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("service:otlp_col,otel_source:test"),
				Message: *datadog.PtrString(""),
				Service: datadog.PtrString("otlp_col"),
				AdditionalProperties: map[string]interface{}{
					"app":              "test",
					"status":           "debug",
					otelSeverityNumber: "5",
					"key":              "val",
					"service.name":     "otlp_col",
				},
			},
		},
		{
			name: "DD hostname and service are not overridden by resource attributes",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().PutStr("app", "test")
					l.SetSeverityNumber(5)
					return l
				}(),
				res: func() pcommon.Resource {
					r := pcommon.NewResource()
					r.Attributes().PutStr("hostname", "example_host")
					r.Attributes().PutStr("service", "otlp_col")
					return r
				}(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("service:otlp_col,otel_source:test"),
				Message: *datadog.PtrString(""),
				AdditionalProperties: map[string]interface{}{
					"app":              "test",
					"status":           "debug",
					otelSeverityNumber: "5",
					"otel.service":     "otlp_col",
					"otel.hostname":    "example_host",
				},
			},
		},
		{
			name: "Nestings",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().FromRaw(
						map[string]any{
							"root": map[string]any{
								"nest1": map[string]any{
									"nest2": "val",
								},
								"nest12": map[string]any{
									"nest22": map[string]any{
										"nest3": "val2",
									},
								},
								"nest13": "val3",
							},
						},
					)
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString(""),
				AdditionalProperties: map[string]interface{}{
					"root.nest1.nest2":         "val",
					"root.nest12.nest22.nest3": "val2",
					"root.nest13":              "val3",
					"status":                   "",
				},
			},
		},
		{
			name: "Nil Map",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().FromRaw(nil)
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString(""),
				AdditionalProperties: map[string]interface{}{
					"status": "",
				},
			},
		},
		{
			name: "Too many nestings",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().FromRaw(
						map[string]any{
							"nest1": map[string]any{
								"nest2": map[string]any{
									"nest3": map[string]any{
										"nest4": map[string]any{
											"nest5": map[string]any{
												"nest6": map[string]any{
													"nest7": map[string]any{
														"nest8": map[string]any{
															"nest9": map[string]any{
																"nest10": map[string]any{
																	"nest11": map[string]any{
																		"nest12": "ok",
																	},
																},
															},
														},
													},
												},
												"nest14": map[string]any{
													"nest15": "ok2",
												},
											},
										},
									},
								},
							},
						},
					)
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString(""),
				AdditionalProperties: map[string]interface{}{
					"nest1.nest2.nest3.nest4.nest5.nest6.nest7.nest8.nest9.nest10": "{\"nest11\":{\"nest12\":\"ok\"}}",
					"nest1.nest2.nest3.nest4.nest5.nest14.nest15":                  "ok2",
					"status": "",
				},
			},
		},
		{
			name: "Timestamps are formatted properly",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.SetTimestamp(pcommon.Timestamp(uint64(1700499303397000000)))
					l.SetSeverityNumber(5)
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString(""),
				AdditionalProperties: map[string]interface{}{
					"status":           "debug",
					otelSeverityNumber: "5",
					ddTimestamp:        "2023-11-20T16:55:03.397Z",
					otelTimestamp:      "1700499303397000000",
				},
			},
		},
		{
			name: "scope attributes",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Body().SetStr("hello world")
					l.SetSeverityNumber(5)
					return l
				}(),
				res: pcommon.NewResource(),
				scope: func() pcommon.InstrumentationScope {
					s := pcommon.NewInstrumentationScope()
					sa := s.Attributes()
					sa.PutStr("otelcol.component.id", "otlp")
					sa.PutStr("otelcol.component.kind", "Receiver")
					return s
				}(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString("hello world"),
				AdditionalProperties: map[string]interface{}{
					"status":                 "debug",
					otelSeverityNumber:       "5",
					"otelcol.component.id":   "otlp",
					"otelcol.component.kind": "Receiver",
				},
			},
		},
		{
			name: "array attribute with strings",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Body().SetStr("test array attribute")
					l.SetSeverityNumber(5)
					arr := l.Attributes().PutEmptySlice("test_array")
					arr.AppendEmpty().SetStr("value1")
					arr.AppendEmpty().SetStr("value2")
					arr.AppendEmpty().SetStr("value3")
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString("test array attribute"),
				AdditionalProperties: map[string]interface{}{
					"status":           "debug",
					otelSeverityNumber: "5",
					"test_array":       []interface{}{"value1", "value2", "value3"},
				},
			},
		},
		{
			name: "array attribute with name-value maps",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Body().SetStr("test array with maps")
					l.SetSeverityNumber(5)
					l.Attributes().FromRaw(map[string]any{
						"array_with_maps": []any{
							map[string]any{"item_name": "item1", "value": int64(100)},
							map[string]any{"item_name": "item2", "value": int64(200)},
						},
					})
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString("test array with maps"),
				AdditionalProperties: map[string]interface{}{
					"status":           "debug",
					otelSeverityNumber: "5",
					"array_with_maps": []interface{}{
						map[string]interface{}{"item_name": "item1", "value": int64(100)},
						map[string]interface{}{"item_name": "item2", "value": int64(200)},
					},
				},
			},
		},
		{
			name: "multiple nesting",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Body().SetStr("test multiple nesting")
					l.SetSeverityNumber(5)
					l.Attributes().FromRaw(map[string]any{
						"multiple_nesting": []any{
							map[string]any{
								"month":      "January",
								"categories": []any{"meetings", "personal", "holidays"},
								"events": []any{
									map[string]any{
										"title":     "Team Meeting",
										"attendees": []any{"a", "b", "c"},
									},
									map[string]any{
										"title":     "Project Review",
										"attendees": []any{"d", "e"},
									},
								},
							},
							map[string]any{
								"month":      "February",
								"categories": []any{"conferences", "workshops"},
								"events": []any{
									map[string]any{
										"title":     "Tech Conference",
										"attendees": []any{"f", "g", "h"},
									},
								},
							},
						},
					})
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString("test multiple nesting"),
				AdditionalProperties: map[string]interface{}{
					"status":           "debug",
					otelSeverityNumber: "5",
					"multiple_nesting": []interface{}{
						map[string]interface{}{
							"month":      "January",
							"categories": []interface{}{"meetings", "personal", "holidays"},
							"events": []interface{}{
								map[string]interface{}{
									"title":     "Team Meeting",
									"attendees": []interface{}{"a", "b", "c"},
								},
								map[string]interface{}{
									"title":     "Project Review",
									"attendees": []interface{}{"d", "e"},
								},
							},
						},
						map[string]interface{}{
							"month":      "February",
							"categories": []interface{}{"conferences", "workshops"},
							"events": []interface{}{
								map[string]interface{}{
									"title":     "Tech Conference",
									"attendees": []interface{}{"f", "g", "h"},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "slice of slices of maps",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Body().SetStr("test slice of slices of maps")
					l.SetSeverityNumber(5)
					l.Attributes().FromRaw(map[string]any{
						"matrix": []any{
							[]any{
								map[string]any{"row": int64(0), "col": int64(0), "value": "a"},
								map[string]any{"row": int64(0), "col": int64(1), "value": "b"},
							},
							[]any{
								map[string]any{"row": int64(1), "col": int64(0), "value": "c"},
								map[string]any{"row": int64(1), "col": int64(1), "value": "d"},
							},
							[]any{
								map[string]any{"row": int64(2), "col": int64(0), "value": "e"},
								map[string]any{"row": int64(2), "col": int64(1), "value": "f"},
							},
						},
					})
					return l
				}(),
				res:   pcommon.NewResource(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString("test slice of slices of maps"),
				AdditionalProperties: map[string]interface{}{
					"status":           "debug",
					otelSeverityNumber: "5",
					"matrix": []interface{}{
						[]interface{}{
							map[string]interface{}{"row": int64(0), "col": int64(0), "value": "a"},
							map[string]interface{}{"row": int64(0), "col": int64(1), "value": "b"},
						},
						[]interface{}{
							map[string]interface{}{"row": int64(1), "col": int64(0), "value": "c"},
							map[string]interface{}{"row": int64(1), "col": int64(1), "value": "d"},
						},
						[]interface{}{
							map[string]interface{}{"row": int64(2), "col": int64(0), "value": "e"},
							map[string]interface{}{"row": int64(2), "col": int64(1), "value": "f"},
						},
					},
				},
			},
		},
	}
}
func TestTranslator(t *testing.T) {
	traceID := [16]byte{0x08, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x0, 0x0, 0x0, 0x0, 0x0a}
	var spanID [8]byte
	copy(spanID[:], traceID[8:])
	ddTr := traceIDToUint64(traceID)
	ddSp := spanIDToUint64(spanID)

	tests := generateTranslatorTestCases(traceID, spanID, ddTr, ddSp)

	set := componenttest.NewNopTelemetrySettings()
	set.Logger = zaptest.NewLogger(t)
	attributesTranslator, err := attributes.NewTranslator(set)
	require.NoError(t, err)
	translator, err := NewTranslator(set, attributesTranslator, "test")
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := plog.NewLogs()
			rl := logs.ResourceLogs().AppendEmpty()
			tt.args.res.MoveTo(rl.Resource())
			sl := rl.ScopeLogs().AppendEmpty()
			tt.args.scope.MoveTo(sl.Scope())
			tt.args.lr.CopyTo(sl.LogRecords().AppendEmpty())

			payloads := translator.MapLogs(context.Background(), logs, nil)
			require.Len(t, payloads, 1)
			got := payloads[0]

			gs, err := got.MarshalJSON()
			require.NoError(t, err)

			ws, err := tt.want.MarshalJSON()
			require.NoError(t, err)

			if !assert.JSONEq(t, string(ws), string(gs)) {
				t.Errorf("Transform() = %v, want %v", string(gs), string(ws))
			}
		})
	}
}

type mockHTTPClient struct{}

func (m *mockHTTPClient) Do(_ *http.Request) (*http.Response, error) {
	body := io.NopCloser(bytes.NewReader([]byte(`{"ok": true}`)))
	return &http.Response{
		StatusCode: http.StatusAccepted,
		Body:       body,
	}, nil
}

func TestTranslatorWithRUMRouting(t *testing.T) {
	traceID := [16]byte{0x08, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x0, 0x0, 0x0, 0x0, 0x0a}
	var spanID [8]byte
	copy(spanID[:], traceID[8:])
	ddTr := traceIDToUint64(traceID)
	ddSp := spanIDToUint64(spanID)

	tests := generateTranslatorTestCases(traceID, spanID, ddTr, ddSp)
	rumTests := []translatorTestCase{
		{
			name: "basic-rum",
			args: args{
				lr: func() plog.LogRecord {
					l := plog.NewLogRecord()
					l.Attributes().PutStr("session.id", "123")
					l.Attributes().PutStr("span_id", "2e26da881214cd7c")
					l.Attributes().PutStr("trace_id", "740112b325075be8c80a48de336ebc67")
					return l
				}(),
				res: func() pcommon.Resource {
					r := pcommon.NewResource()
					r.Attributes().PutStr("request_ddforward", "/v1/rum/events")
					return r
				}(),
				scope: pcommon.NewInstrumentationScope(),
			},
			want: datadogV2.HTTPLogItem{
				Ddtags:  datadog.PtrString("otel_source:test"),
				Message: *datadog.PtrString(""),
			},
		},
	}
	tests = append(tests, rumTests...)

	set := componenttest.NewNopTelemetrySettings()
	set.Logger = zaptest.NewLogger(t)
	attributesTranslator, err := attributes.NewTranslator(set)
	require.NoError(t, err)
	translator, err := NewTranslatorWithHTTPClient(set, attributesTranslator, "test", &mockHTTPClient{})
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := plog.NewLogs()
			rl := logs.ResourceLogs().AppendEmpty()
			tt.args.res.MoveTo(rl.Resource())
			sl := rl.ScopeLogs().AppendEmpty()
			tt.args.scope.MoveTo(sl.Scope())
			tt.args.lr.CopyTo(sl.LogRecords().AppendEmpty())

			payloads, err := translator.MapLogsAndRouteRUMEvents(context.Background(), logs, nil, true, "https://test-intake-datadoghq.com")
			require.NoError(t, err)

			attributes := sl.LogRecords().At(0).Attributes()
			if _, ok := attributes.Get("session.id"); ok {
				require.Len(t, payloads, 0)
				return
			}

			require.Len(t, payloads, 1)
			got := payloads[0]

			gs, err := got.MarshalJSON()
			require.NoError(t, err)

			ws, err := tt.want.MarshalJSON()
			require.NoError(t, err)

			if !assert.JSONEq(t, string(ws), string(gs)) {
				t.Errorf("Transform() = %v, want %v", string(gs), string(ws))
			}

			payloadsNoRouting, err := translator.MapLogsAndRouteRUMEvents(context.Background(), logs, nil, false, "https://test-intake-datadoghq.com")
			require.NoError(t, err)
			require.Len(t, payloads, 1)
			got = payloadsNoRouting[0]

			gs, err = got.MarshalJSON()
			require.NoError(t, err)

			ws, err = tt.want.MarshalJSON()
			require.NoError(t, err)

			if !assert.JSONEq(t, string(ws), string(gs)) {
				t.Errorf("Transform() = %v, want %v", string(gs), string(ws))
			}
		})
	}
}

func TestDeriveStatus(t *testing.T) {
	type args struct {
		severity plog.SeverityNumber
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "trace3",
			args: args{
				severity: 3,
			},
			want: logLevelTrace,
		},
		{
			name: "trace4",
			args: args{
				severity: 4,
			},
			want: logLevelTrace,
		},
		{
			name: "debug5",
			args: args{
				severity: 5,
			},
			want: logLevelDebug,
		},
		{
			name: "debug7",
			args: args{
				severity: 7,
			},
			want: logLevelDebug,
		},
		{
			name: "debug8",
			args: args{
				severity: 8,
			},
			want: logLevelDebug,
		},
		{
			name: "info9",
			args: args{
				severity: 9,
			},
			want: logLevelInfo,
		},
		{
			name: "info12",
			args: args{
				severity: 12,
			},
			want: logLevelInfo,
		},
		{
			name: "warn13",
			args: args{
				severity: 13,
			},
			want: logLevelWarn,
		},
		{
			name: "warn16",
			args: args{
				severity: 16,
			},
			want: logLevelWarn,
		},
		{
			name: "error17",
			args: args{
				severity: 17,
			},
			want: logLevelError,
		},
		{
			name: "error20",
			args: args{
				severity: 20,
			},
			want: logLevelError,
		},
		{
			name: "fatal21",
			args: args{
				severity: 21,
			},
			want: logLevelFatal,
		},
		{
			name: "fatal24",
			args: args{
				severity: 24,
			},
			want: logLevelFatal,
		},
		{
			name: "undefined",
			args: args{
				severity: 50,
			},
			want: logLevelError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, statusFromSeverityNumber(tt.args.severity), "derviveDdStatusFromSeverityNumber(%v)", tt.args.severity)
		})
	}
}
