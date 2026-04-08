// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logs

import (
	"testing"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	semconv16 "go.opentelemetry.io/otel/semconv/v1.6.1"
)

// buildLogs is a helper that assembles a plog.Logs from a single resource, scope, and log record.
func buildLogs(res pcommon.Resource, lr plog.LogRecord) plog.Logs {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	res.CopyTo(rl.Resource())
	sl := rl.ScopeLogs().AppendEmpty()
	lr.CopyTo(sl.LogRecords().AppendEmpty())
	return ld
}

func TestTranslate_Basic(t *testing.T) {
	lr := plog.NewLogRecord()
	lr.Body().SetStr("hello world")
	lr.SetSeverityNumber(9) // info

	payloads, err := Translate(buildLogs(pcommon.NewResource(), lr))
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	got := payloads[0]
	assert.Equal(t, "hello world", got.GetMessage())
	assert.Nil(t, got.Hostname)
	assert.Nil(t, got.Service)
	assert.Equal(t, "info", got.AdditionalProperties[ddStatus])
}

func TestTranslate_HostFromResource(t *testing.T) {
	res := pcommon.NewResource()
	res.Attributes().PutStr(string(semconv16.HostNameKey), "my-host")
	res.Attributes().PutStr(string(semconv16.ServiceNameKey), "my-service")

	lr := plog.NewLogRecord()
	lr.Body().SetStr("msg")

	payloads, err := Translate(buildLogs(res, lr))
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	got := payloads[0]
	assert.Equal(t, datadog.PtrString("my-host"), got.Hostname)
	assert.Equal(t, datadog.PtrString("my-service"), got.Service)
}

func TestTranslate_HostFromLogAttrs(t *testing.T) {
	// HACK: host and service absent from resource but present in log record attributes.
	lr := plog.NewLogRecord()
	lr.Attributes().PutStr(string(semconv16.HostNameKey), "record-host")
	lr.Attributes().PutStr(string(semconv16.ServiceNameKey), "record-service")
	lr.Body().SetStr("msg")

	payloads, err := Translate(buildLogs(pcommon.NewResource(), lr))
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	got := payloads[0]
	assert.Equal(t, datadog.PtrString("record-host"), got.Hostname)
	assert.Equal(t, datadog.PtrString("record-service"), got.Service)
}

func TestTranslate_ResourceHostTakesPrecedenceOverLogAttrs(t *testing.T) {
	res := pcommon.NewResource()
	res.Attributes().PutStr(string(semconv16.HostNameKey), "resource-host")
	res.Attributes().PutStr(string(semconv16.ServiceNameKey), "resource-service")

	lr := plog.NewLogRecord()
	lr.Attributes().PutStr(string(semconv16.HostNameKey), "record-host")
	lr.Attributes().PutStr(string(semconv16.ServiceNameKey), "record-service")

	payloads, err := Translate(buildLogs(res, lr))
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	got := payloads[0]
	assert.Equal(t, datadog.PtrString("resource-host"), got.Hostname)
	assert.Equal(t, datadog.PtrString("resource-service"), got.Service)
}

func TestTranslate_SeverityMapping(t *testing.T) {
	cases := []struct {
		num    plog.SeverityNumber
		text   string
		wantDD string
	}{
		{3, "", logLevelTrace},
		{5, "", logLevelDebug},
		{9, "", logLevelInfo},
		{13, "", logLevelWarn},
		{17, "", logLevelError},
		{21, "", logLevelFatal},
		// SeverityText takes precedence when both are set
		{5, "critical", "critical"},
	}

	for _, tc := range cases {
		lr := plog.NewLogRecord()
		lr.SetSeverityNumber(tc.num)
		if tc.text != "" {
			lr.SetSeverityText(tc.text)
		}

		payloads, err := Translate(buildLogs(pcommon.NewResource(), lr))
		require.NoError(t, err)
		require.Len(t, payloads, 1)
		assert.Equal(t, tc.wantDD, payloads[0].AdditionalProperties[ddStatus], "severity %v text %q", tc.num, tc.text)
	}
}

func TestTranslate_NestedMap(t *testing.T) {
	lr := plog.NewLogRecord()
	lr.Body().SetStr("nested")
	lr.Attributes().FromRaw(map[string]any{
		"root": map[string]any{
			"child": map[string]any{
				"leaf": "value",
			},
		},
	})

	payloads, err := Translate(buildLogs(pcommon.NewResource(), lr))
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	assert.Equal(t, "value", payloads[0].AdditionalProperties["root.child.leaf"])
}

func TestTranslate_NestedList(t *testing.T) {
	lr := plog.NewLogRecord()
	lr.Body().SetStr("list")
	lr.Attributes().FromRaw(map[string]any{
		"items": []any{"a", "b", "c"},
	})

	payloads, err := Translate(buildLogs(pcommon.NewResource(), lr))
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	assert.Equal(t, []interface{}{"a", "b", "c"}, payloads[0].AdditionalProperties["items"])
}

func TestTranslate_ListOfMaps(t *testing.T) {
	lr := plog.NewLogRecord()
	lr.Body().SetStr("list of maps")
	lr.Attributes().FromRaw(map[string]any{
		"events": []any{
			map[string]any{"name": "click", "count": int64(3)},
			map[string]any{"name": "view", "count": int64(7)},
		},
	})

	payloads, err := Translate(buildLogs(pcommon.NewResource(), lr))
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	want := []interface{}{
		map[string]interface{}{"name": "click", "count": int64(3)},
		map[string]interface{}{"name": "view", "count": int64(7)},
	}
	assert.Equal(t, want, payloads[0].AdditionalProperties["events"])
}

func TestTranslate_NestedListsOfLists(t *testing.T) {
	lr := plog.NewLogRecord()
	lr.Attributes().FromRaw(map[string]any{
		"matrix": []any{
			[]any{"a", "b"},
			[]any{"c", "d"},
		},
	})

	payloads, err := Translate(buildLogs(pcommon.NewResource(), lr))
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	want := []interface{}{
		[]interface{}{"a", "b"},
		[]interface{}{"c", "d"},
	}
	assert.Equal(t, want, payloads[0].AdditionalProperties["matrix"])
}

func TestTranslate_MultipleResourcesAndRecords(t *testing.T) {
	ld := plog.NewLogs()

	// First resource: two log records
	rl1 := ld.ResourceLogs().AppendEmpty()
	rl1.Resource().Attributes().PutStr(string(semconv16.HostNameKey), "host-1")
	rl1.Resource().Attributes().PutStr(string(semconv16.ServiceNameKey), "svc-1")
	sl1 := rl1.ScopeLogs().AppendEmpty()
	lr1a := sl1.LogRecords().AppendEmpty()
	lr1a.Body().SetStr("msg-1a")
	lr1b := sl1.LogRecords().AppendEmpty()
	lr1b.Body().SetStr("msg-1b")

	// Second resource: one log record
	rl2 := ld.ResourceLogs().AppendEmpty()
	rl2.Resource().Attributes().PutStr(string(semconv16.HostNameKey), "host-2")
	sl2 := rl2.ScopeLogs().AppendEmpty()
	lr2 := sl2.LogRecords().AppendEmpty()
	lr2.Body().SetStr("msg-2")

	payloads, err := Translate(ld)
	require.NoError(t, err)
	require.Len(t, payloads, 3)

	assert.Equal(t, "msg-1a", payloads[0].GetMessage())
	assert.Equal(t, datadog.PtrString("host-1"), payloads[0].Hostname)
	assert.Equal(t, datadog.PtrString("svc-1"), payloads[0].Service)

	assert.Equal(t, "msg-1b", payloads[1].GetMessage())
	assert.Equal(t, datadog.PtrString("host-1"), payloads[1].Hostname)

	assert.Equal(t, "msg-2", payloads[2].GetMessage())
	assert.Equal(t, datadog.PtrString("host-2"), payloads[2].Hostname)
	assert.Nil(t, payloads[2].Service)
}

func TestTranslate_EmptyLogs(t *testing.T) {
	payloads, err := Translate(plog.NewLogs())
	require.NoError(t, err)
	assert.Empty(t, payloads)
}

func TestTranslate_TagsFromResourceAttributes(t *testing.T) {
	res := pcommon.NewResource()
	res.Attributes().PutStr(string(semconv16.ServiceNameKey), "my-svc")
	res.Attributes().PutStr("env", "prod")

	lr := plog.NewLogRecord()
	lr.Body().SetStr("tagged")

	payloads, err := Translate(buildLogs(res, lr))
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	// Tags are derived from resource attributes via TagsFromAttributes.
	got := payloads[0]
	require.NotNil(t, got.Ddtags)
	ddtags := got.GetDdtags()
	assert.Contains(t, ddtags, "service:my-svc")
}

func TestTranslate_TraceAndSpanIDs(t *testing.T) {
	traceID := pcommon.TraceID([16]byte{0x08, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x0, 0x0, 0x0, 0x0, 0x0a})
	spanID := pcommon.SpanID([8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08})

	lr := plog.NewLogRecord()
	lr.SetTraceID(traceID)
	lr.SetSpanID(spanID)

	payloads, err := Translate(buildLogs(pcommon.NewResource(), lr))
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	got := payloads[0].AdditionalProperties
	assert.NotEmpty(t, got[ddTraceID])
	assert.NotEmpty(t, got[ddSpanID])
	assert.NotEmpty(t, got[otelTraceID])
	assert.NotEmpty(t, got[otelSpanID])
}

func TestTranslate_ReturnsHTTPLogItems(t *testing.T) {
	// Verify the return type matches datadogV2.HTTPLogItem (compile-time check via usage).
	var _ []datadogV2.HTTPLogItem
	res := pcommon.NewResource()
	res.Attributes().PutStr(string(semconv16.ServiceNameKey), "svc")
	lr := plog.NewLogRecord()
	lr.Body().SetStr("check type")

	payloads, err := Translate(buildLogs(res, lr))
	require.NoError(t, err)
	require.Len(t, payloads, 1)
	assert.IsType(t, datadogV2.HTTPLogItem{}, payloads[0])
}
