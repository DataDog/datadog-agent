// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetrycapture

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type deadlineRecorder struct{ *httptest.ResponseRecorder }

func (*deadlineRecorder) SetWriteDeadline(time.Time) error { return nil }

func request(t *testing.T, handler http.Handler, method, path, body string, authenticated bool) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if authenticated {
		r.Header.Set("Authorization", "Bearer test-token")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(&deadlineRecorder{w}, r)
	return w
}

const validControlJSON = `{"protocol_version":1,"session_id":"test-session-00000001"}`

func TestHandlerReportsOwnedMetricSchedulesOnEveryResponse(t *testing.T) {
	m := testManager(t, Metrics, Software)
	provided := []MetricSchedule{{Family: "battery", Cadence: 5 * time.Minute}, {Family: "cpu", Cadence: 15 * time.Second}}
	calls := 0
	handler, err := m.Handler(testAuth, func() []MetricSchedule {
		calls++
		return provided
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := request(t, handler, http.MethodGet, "/capabilities", "", false); got.Code != http.StatusUnauthorized || calls != 0 {
		t.Fatal("unauthenticated request inspected collection schedules")
	}
	for _, operation := range []struct{ method, path, body string }{
		{http.MethodGet, "/capabilities", ""},
		{http.MethodPost, "/prepare", `{"protocol_version":1,"session_id":"test-session-00000001","streams":["metrics"]}`},
		{http.MethodPost, "/activate", validControlJSON},
		{http.MethodPost, "/heartbeat", validControlJSON},
		{http.MethodGet, "/status", ""},
		{http.MethodPost, "/records", validControlJSON},
		{http.MethodPost, "/stop", validControlJSON},
	} {
		w := request(t, handler, operation.method, operation.path, operation.body, true)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d", operation.path, w.Code)
		}
		var status Status
		if operation.path == "/records" {
			var batch struct {
				Status Status `json:"status"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &batch); err != nil {
				t.Fatal(err)
			}
			status = batch.Status
		} else if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(status.Capabilities[0].MetricSchedules, provided) || len(status.Capabilities[1].MetricSchedules) != 0 {
			t.Fatalf("%s did not report only the actual metric schedules", operation.path)
		}
	}
	if len(m.Status().Capabilities[0].MetricSchedules) != 0 {
		t.Fatal("read-only schedule projection mutated the manager")
	}
	projected := withMetricSchedules(m.Status(), func() []MetricSchedule { return provided })
	provided[0].Cadence = time.Hour
	if projected.Capabilities[0].MetricSchedules[0].Cadence != 5*time.Minute {
		t.Fatal("status retained provider-owned schedules")
	}
}

func TestMetricSchedulesAreBoundedAndRejectPrivateNames(t *testing.T) {
	status := Status{Capabilities: []Capability{{Stream: Metrics, Cadence: time.Second}}}
	provided := []MetricSchedule{{Family: "private-instance-secret", Cadence: time.Hour}, {Family: "cpu", Cadence: -1}}
	for _, family := range []string{"cpu", "memory", "disk", "uptime", "wlan", "battery", "network"} {
		provided = append(provided, MetricSchedule{Family: family, Cadence: time.Second}, MetricSchedule{Family: family, Cadence: time.Minute})
	}
	result := withMetricSchedules(status, func() []MetricSchedule { return provided })
	if len(result.Capabilities[0].MetricSchedules) != 7 || len(status.Capabilities[0].MetricSchedules) != 0 {
		t.Fatal("schedule projection is not bounded or mutated its input")
	}
	for _, schedule := range result.Capabilities[0].MetricSchedules {
		if MetricCheckFamily(schedule.Family) == "" || schedule.Cadence != time.Minute {
			t.Fatal("invalid schedule projection")
		}
	}
	for name := range metricNames {
		if MetricFamily(name) == "" {
			t.Fatalf("allowed metric has no native check family: %s", name)
		}
	}
	if MetricFamily("system.net.private_metric") != "" || MetricCheckFamily("battery-private-instance") != "" {
		t.Fatal("arbitrary names entered fixed capture family scope")
	}
}

func TestHandlerRequiresAuthenticationOnEveryRoute(t *testing.T) {
	m := testManager(t, Software)
	if _, err := m.Handler(nil); !errors.Is(err, ErrRequest) {
		t.Fatal("unauthenticated handler permitted")
	}
	handler, err := m.Handler(testAuth)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/capabilities", "/status", "/prepare", "/activate", "/heartbeat", "/records", "/stop"} {
		method := http.MethodPost
		if path == "/capabilities" || path == "/status" {
			method = http.MethodGet
		}
		if w := request(t, handler, method, path, validControlJSON, false); w.Code != http.StatusUnauthorized {
			t.Fatalf("unprotected %s: %d", path, w.Code)
		}
	}
	if m.Status().SessionID != "" {
		t.Fatal("unauthenticated request changed state")
	}
}

func TestControlValidationDoesNotEchoInput(t *testing.T) {
	m := testManager(t, Software)
	handler, err := m.Handler(testAuth)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"protocol_version":2,"session_id":"secret-native-host"}`,
		`{"protocol_version":1,"session_id":"secret-native-host","secret-native-host":true}`,
		`{"protocol_version":1,"session_id":"secret/native/host"}`,
		`{"secret-native-host":`,
		validControlJSON + ` {"secret-native-host":true}`,
		`{"session_id":"` + strings.Repeat("secret-native-host", 4096) + `"}`,
	} {
		w := request(t, handler, http.MethodPost, "/activate", body, true)
		if w.Code != http.StatusBadRequest || strings.Contains(w.Body.String(), "secret-native-host") {
			t.Fatalf("unsafe validation response: %d", w.Code)
		}
	}
}

func TestHTTPReadAckAndStop(t *testing.T) {
	m := testManager(t, Software)
	handler, err := m.Handler(testAuth)
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []struct{ path, body string }{
		{"/prepare", `{"protocol_version":1,"session_id":"test-session-00000001","streams":["software"]}`},
		{"/activate", validControlJSON},
		{"/heartbeat", validControlJSON},
	} {
		if w := request(t, handler, http.MethodPost, operation.path, operation.body, true); w.Code != http.StatusOK {
			t.Fatalf("%s: %d", operation.path, w.Code)
		}
	}
	if !observeSoftware(m) {
		t.Fatal("copy rejected")
	}
	w := request(t, handler, http.MethodGet, "/status", "", true)
	if strings.Contains(w.Body.String(), "complete software snapshot") || strings.Contains(w.Body.String(), "payload") {
		t.Fatal("status exposes telemetry")
	}
	w = request(t, handler, http.MethodPost, "/stop", validControlJSON, true)
	if w.Code != http.StatusOK || m.Status().State != Stopping {
		t.Fatal("premature stopped acknowledgement")
	}
	w = request(t, handler, http.MethodPost, "/records", validControlJSON, true)
	var batch struct {
		Status  Status   `json:"status"`
		Records []Record `json:"records"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &batch) != nil || len(batch.Records) != 1 {
		t.Fatalf("invalid record batch: %d", w.Code)
	}
	if !reflect.DeepEqual(batch.Records[0].Payload, softwarePayload()) {
		t.Fatal("record changed in IPC")
	}
	w = request(t, handler, http.MethodPost, "/records", `{"protocol_version":1,"session_id":"test-session-00000001","cursor":1}`, true)
	if w.Code != http.StatusOK || m.Status().State != Stopped {
		t.Fatal("final ack not recorded")
	}
	assertEmpty(t, m)
}

type brokenWriter struct{ header http.Header }

func (w *brokenWriter) Header() http.Header            { return w.header }
func (*brokenWriter) WriteHeader(int)                  {}
func (*brokenWriter) Write([]byte) (int, error)        { return 0, io.ErrClosedPipe }
func (*brokenWriter) SetWriteDeadline(time.Time) error { return nil }

func TestIPCFailureReleasesPinnedRecords(t *testing.T) {
	m := testManager(t, Software)
	arm(t, m, Software)
	if !observeSoftware(m) {
		t.Fatal("copy rejected")
	}
	handler, err := m.Handler(testAuth)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/records", strings.NewReader(validControlJSON))
	r.Header.Set("Authorization", "Bearer test-token")
	handler.ServeHTTP(&brokenWriter{header: make(http.Header)}, r)
	if m.Status().State != Failed {
		t.Fatal("IPC write failure not terminal")
	}
	assertEmpty(t, m)
}

func TestIPCRejectsUnboundedWriter(t *testing.T) {
	m := testManager(t, Software)
	arm(t, m, Software)
	if !observeSoftware(m) {
		t.Fatal("copy rejected")
	}
	handler, err := m.Handler(testAuth)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/records", strings.NewReader(validControlJSON))
	r.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder() // Does not support write deadlines.
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusConflict || m.Status().State != Failed {
		t.Fatal("unbounded reader accepted")
	}
	assertEmpty(t, m)
}

func TestBoundedWireEncodingMatchesJSON(t *testing.T) {
	oddString := "native\x00\x01\n\r\t\"\\雪\xff<&>\u2028"
	large := strings.Repeat(oddString, 2048)
	now := time.Date(2026, 9, 30, 12, 34, 56, 123456789, time.UTC)
	for _, payload := range []Payload{
		{Series: []Series{{Name: oddString, Tags: []string{large}, Points: []Point{{Timestamp: 123456789.123, Value: 1.23e-30}}, Source: 42, Type: -3, Interval: 15}}},
		{Metadata: &HostMetadata{AgentVersion: "test", HostTags: map[string][]string{oddString: {large}}, Windows: []string{"Windows", "11"}}},
		{Software: &Message{Body: []byte(large), Timestamp: -12345}},
		{Inventory: &Inventory{Hostname: oddString, UUID: large, Timestamp: now.UnixNano(), Agent: &AgentInventoryMetadata{AgentVersion: large, FeatureProcessEnabled: true, FeatureNetworksEnabled: false}}},
		{Inventory: &Inventory{Hostname: oddString, Timestamp: now.UnixNano(), Host: &HostInventoryMetadata{CPUModel: large, CPUFrequency: 1234.5, MemoryTotalKb: 16384000}}},
		{Inventory: &Inventory{Hostname: oddString, UUID: large, Timestamp: now.UnixNano(), SystemInfo: &HostSystemInfoMetadata{SerialNumber: large, Identifier: oddString, ModelName: "model"}}},
		{Chunks: []Chunk{{Body: []byte(large), Headers: map[string]string{"test": oddString}}}},
		{Routes: []Route{{PayloadID: 12, Ordinals: []uint64{1, 4, 7}, Endpoint: "/api/v2/series", Protocol: "v2", Destination: "primary", EnqueuedAt: now}}},
	} {
		batch := &Batch{Status: Status{ProtocolVersion: 1}, Records: []*Record{{ProtocolVersion: 1, SessionID: testControl.SessionID, CollectedAt: now, Payload: payload}}}
		var actual bytes.Buffer
		if err := writeBatch(&actual, batch); err != nil {
			t.Fatal(err)
		}
		expected, err := json.Marshal(struct {
			Status  Status    `json:"status"`
			Records []*Record `json:"records"`
		}{batch.Status, batch.Records})
		if err != nil {
			t.Fatal(err)
		}
		var actualValue, expectedValue any
		if json.Unmarshal(actual.Bytes(), &actualValue) != nil || json.Unmarshal(expected, &expectedValue) != nil || !reflect.DeepEqual(actualValue, expectedValue) {
			t.Fatal("stream encoding differs from JSON contract")
		}
	}
}

func TestWireRejectsNonfiniteMetricValues(t *testing.T) {
	for _, value := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
		batch := &Batch{Records: []*Record{{Payload: Payload{Series: []Series{{Points: []Point{{Value: value}}}}}}}}
		if err := writeBatch(io.Discard, batch); err == nil {
			t.Fatal("invalid JSON number accepted")
		}
	}
}
