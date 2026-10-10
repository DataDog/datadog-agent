// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/localdog/store"
)

// Payloads recorded from a real agent (copied from fakeintake's fixtures).
var (
	//go:embed testdata/log_bytes
	logFixture []byte
	//go:embed testdata/trace_bytes
	traceFixture []byte
	//go:embed testdata/metric_bytes
	metricFixture []byte
)

func newTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st := store.New(store.Options{MaxLogs: 1000, MaxSpans: 1000, MaxPointsPerSeries: 1000, Retention: 365 * 24 * time.Hour})
	return New(st, Options{}), st
}

func post(t *testing.T, s *Server, path, encoding string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func gz(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write(b)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func TestIntakeFixtures(t *testing.T) {
	s, st := newTestServer(t)

	assert.Equal(t, http.StatusAccepted, post(t, s, "/api/v2/logs", "gzip", logFixture).Code)
	assert.Equal(t, http.StatusAccepted, post(t, s, "/api/v0.2/traces", "gzip", traceFixture).Code)
	assert.Equal(t, http.StatusAccepted, post(t, s, "/api/v2/series", "deflate", metricFixture).Code)

	stats := st.Stats()
	assert.Positive(t, stats.Logs)
	assert.Positive(t, stats.Spans)
	assert.Positive(t, stats.MetricSeries)
	assert.Equal(t, 1, stats.Payloads["/api/v2/logs"])
}

func TestStructuredLogNormalization(t *testing.T) {
	s, st := newTestServer(t)
	line := `{"level":"error","msg":"boom","http.status_code":500,"dd":{"trace_id":"6aca7e1f0000000000c7d338bebd04d0","span_id":"42"}}`
	body, _ := json.Marshal([]map[string]any{{
		"message": line, "status": "info", "hostname": "h", "service": "web", "ddsource": "nodejs",
		"ddtags": "env:dev,team:x", "timestamp": 1700000000000,
	}})
	require.Equal(t, http.StatusAccepted, post(t, s, "/api/v2/logs", "gzip", gz(t, body)).Code)

	var got *store.Log
	st.Logs(func(l *store.Log) bool { got = l; return false })
	require.NotNil(t, got)
	assert.Equal(t, "boom", got.Message)
	assert.Equal(t, "error", got.Status)
	assert.Equal(t, []string{"env:dev", "team:x"}, got.Tags)
	// 128-bit hex trace IDs are reduced to the decimal low 64 bits used by spans
	assert.Equal(t, "56245761037108432", got.TraceID)
	assert.Equal(t, "42", got.SpanID)
	// dotted keys are nested
	httpAttrs, ok := got.Attributes["http"].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, 500, httpAttrs["status_code"])
	assert.Equal(t, []string{"500"}, got.Field("@http.status_code"))
}

func TestLogsListAggregateAndFacets(t *testing.T) {
	s, st := newTestServer(t)
	now := time.Now().UnixMilli()
	st.AddLogs([]*store.Log{
		{Timestamp: now - 3000, Message: "ok 1", Status: "info", Service: "web", Attributes: map[string]any{}},
		{Timestamp: now - 2000, Message: "failed to connect", Status: "error", Service: "web", Attributes: map[string]any{"http": map[string]any{"status_code": float64(503)}}},
		{Timestamp: now - 1000, Message: "ok 2", Status: "info", Service: "api", Attributes: map[string]any{}},
	})

	rec := post(t, s, "/api/v1/logs-analytics/list?type=logs", "", []byte(`{"list":{"limit":1,"search":{"query":"service:web"}}}`))
	var list struct {
		Status   string
		HitCount int
		Result   struct {
			Events []struct {
				Event map[string]any
			}
			Paging *struct{ After string }
		}
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	assert.Equal(t, "done", list.Status)
	assert.Equal(t, 2, list.HitCount)
	require.Len(t, list.Result.Events, 1)
	assert.Equal(t, "failed to connect", list.Result.Events[0].Event["message"], "newest first")
	require.NotNil(t, list.Result.Paging)

	rec = post(t, s, "/api/v1/logs-analytics/aggregate?type=logs", "", []byte(`{"aggregate":{
		"compute":[{"total":{"metric":"count","aggregation":"count","output":"c"}}],
		"groupBy":[{"field":{"id":"status","output":"status","limit":10}}]}}`))
	var agg struct {
		Result struct {
			Values []struct {
				By      map[string]any
				Metrics map[string]float64
			}
		}
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &agg))
	require.Len(t, agg.Result.Values, 2)
	assert.Equal(t, "info", agg.Result.Values[0].By["status"])
	assert.Equal(t, 2.0, agg.Result.Values[0].Metrics["c"])

	rec = post(t, s, "/api/v1/logs-analytics/facet_info?type=logs", "", []byte(`{"facet_info":{"path":"@http.status_code","limit":5}}`))
	assert.Contains(t, rec.Body.String(), `"field":"503"`)

	req := httptest.NewRequest(http.MethodGet, "/api/ui/event-platform/logs/facets", nil)
	frec := httptest.NewRecorder()
	s.ServeHTTP(frec, req)
	assert.Contains(t, frec.Body.String(), `"path":"http.status_code"`)
}

func TestTraceEndpoint(t *testing.T) {
	s, st := newTestServer(t)
	start := time.Now().UnixNano()
	st.AddSpans([]*store.Span{
		{TraceID: "7", SpanID: "1", ParentID: "0", Service: "web", Name: "http.request", Resource: "GET /", Start: start, Duration: 2e9, IsRoot: true, IsTopLevel: true},
		{TraceID: "7", SpanID: "2", ParentID: "1", Service: "db", Name: "query", Resource: "SELECT", Start: start + 1e8, Duration: 5e8},
		{TraceID: "7", SpanID: "3", ParentID: "99", Service: "worker", Name: "job", Start: start + 2e8, Duration: 1e8},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/ui/trace/7", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Trace struct {
			RootID string `json:"root_id"`
			Spans  map[string]struct {
				ChildrenIDs []string `json:"children_ids"`
				Duration    float64
				Metrics     map[string]float64
			}
		}
		Orphaned []struct {
			RootID string `json:"root_id"`
		}
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "1", resp.Trace.RootID)
	assert.Equal(t, []string{"2"}, resp.Trace.Spans["1"].ChildrenIDs)
	assert.InDelta(t, 2.0, resp.Trace.Spans["1"].Duration, 1e-9, "durations are in seconds")
	assert.Equal(t, 1.0, resp.Trace.Spans["1"].Metrics["_top_level"])
	require.Len(t, resp.Orphaned, 1)
	assert.Equal(t, "3", resp.Orphaned[0].RootID)
}

func TestTimeseriesAndScalar(t *testing.T) {
	s, st := newTestServer(t)
	now := time.Now().Unix()
	from := (now - 600) / 60 * 60
	for i := int64(0); i < 10; i++ {
		ts := from + i*60
		st.AddPoints("req", "rate", "h1", "", 10, []string{"route:/a"}, []store.Point{{Timestamp: ts, Value: 1}})
		st.AddPoints("req", "rate", "h1", "", 10, []string{"route:/b"}, []store.Point{{Timestamp: ts, Value: 3}})
	}
	body := fmt.Sprintf(`{"data":[{"type":"timeseries_request","attributes":{"from":%d,"to":%d,"interval":60000,
		"formulas":[{"formula":"query1 * 2"}],
		"queries":[{"data_source":"metrics","name":"query1","query":"sum:req{*} by {route}.as_count()"}]}}]}`, from*1000, (from+599)*1000)
	rec := post(t, s, "/api/ui/query/timeseries", "", []byte(body))
	var ts struct {
		Data []struct {
			Attributes struct {
				Series []struct {
					GroupTags []string `json:"group_tags"`
				}
				Times  []int64
				Values [][]*float64
			}
		}
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ts), rec.Body.String())
	attrs := ts.Data[0].Attributes
	require.Len(t, attrs.Series, 2)
	assert.Equal(t, []string{"route:/a"}, attrs.Series[0].GroupTags)
	assert.Len(t, attrs.Times, 10)
	// rate 1/s over a 10s interval = 10 per point, times 2
	assert.Equal(t, 20.0, *attrs.Values[0][0])
	assert.Equal(t, 60.0, *attrs.Values[1][0])

	body = strings.Replace(body, `"timeseries_request"`, `"scalar_request"`, 1)
	body = strings.Replace(body, `"name":"query1",`, `"name":"query1","aggregator":"sum",`, 1)
	rec = post(t, s, "/api/ui/query/scalar", "", []byte(body))
	assert.Contains(t, rec.Body.String(), `"values":[["/b"],["/a"]]`)
	assert.Contains(t, rec.Body.String(), `"values":[600,200]`)
}

func TestCORSPreflight(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/logs-analytics/list", nil)
	req.Header.Set("Origin", "https://localdog.example")
	req.Header.Set("Access-Control-Request-Headers", "content-type,x-csrf-token")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "https://localdog.example", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "content-type,x-csrf-token", rec.Header().Get("Access-Control-Allow-Headers"))
	assert.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Private-Network"))
}
