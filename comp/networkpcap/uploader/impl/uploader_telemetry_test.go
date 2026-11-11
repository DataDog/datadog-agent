// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package uploaderimpl

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// metricsSnapshot is a snapshot of an uploader's telemetry. Tests compare
// snapshots taken before and after an upload.
type metricsSnapshot struct {
	attempts map[[2]string]float64
	results  map[[2]string]float64
	bytes    telemetry.HistogramValue
	duration telemetry.HistogramValue
}

func snapshotUploadMetrics(u *pcapUploader) metricsSnapshot {
	m := metricsSnapshot{
		attempts: map[[2]string]float64{},
		results:  map[[2]string]float64{},
		bytes:    u.metrics.bytes.Get(),
		duration: u.metrics.duration.Get(),
	}
	for _, class := range []string{"2xx", "4xx", "5xx", "transport", "other"} {
		for _, retry := range []string{"false", "true"} {
			m.attempts[[2]string{class, retry}] = u.metrics.attempts.WithValues(class, retry).Get()
		}
	}
	for _, endpoint := range []string{"reliable", "unreliable"} {
		for _, result := range []string{"success", "too_large", "failed"} {
			m.results[[2]string{endpoint, result}] = u.metrics.results.WithValues(endpoint, result).Get()
		}
	}
	return m
}

// uploadMetricsDelta is what changed between two snapshots; zero entries are
// dropped so tests can compare whole maps.
type uploadMetricsDelta struct {
	attempts      map[[2]string]float64
	results       map[[2]string]float64
	bytesCount    uint64
	bytesSum      float64
	durationCount uint64
}

func (m metricsSnapshot) since(before metricsSnapshot) uploadMetricsDelta {
	d := uploadMetricsDelta{
		attempts:      map[[2]string]float64{},
		results:       map[[2]string]float64{},
		bytesCount:    m.bytes.Count - before.bytes.Count,
		bytesSum:      m.bytes.Sum - before.bytes.Sum,
		durationCount: m.duration.Count - before.duration.Count,
	}
	for k, v := range m.attempts {
		if diff := v - before.attempts[k]; diff != 0 {
			d.attempts[k] = diff
		}
	}
	for k, v := range m.results {
		if diff := v - before.results[k]; diff != 0 {
			d.results[k] = diff
		}
	}
	return d
}

// newSizedIntake answers with statuses in order (the last one repeating) and
// records the size of the last compressed body it received.
func newSizedIntake(t *testing.T, lastBodySize *atomic.Int64, statuses ...int) *intake {
	t.Helper()
	in := &intake{}
	in.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(in.calls.Add(1)) - 1
		n, _ := io.Copy(io.Discard, r.Body)
		lastBodySize.Store(n)
		w.WriteHeader(statuses[min(i, len(statuses)-1)])
	}))
	t.Cleanup(in.srv.Close)
	return in
}

func TestUploadTelemetrySuccess(t *testing.T) {
	var bodySize atomic.Int64
	in := newSizedIntake(t, &bodySize, http.StatusAccepted)
	u := newTestUploader(t, newTestConfig(t, in))
	req, _ := pcapRequest(bytes.Repeat([]byte("pcap"), 64<<10))

	before := snapshotUploadMetrics(u)
	require.NoError(t, u.Upload(context.Background(), req))
	d := snapshotUploadMetrics(u).since(before)

	assert.Equal(t, map[[2]string]float64{{"2xx", "false"}: 1}, d.attempts)
	assert.Equal(t, map[[2]string]float64{{"reliable", "success"}: 1}, d.results)
	assert.EqualValues(t, 1, d.bytesCount)
	assert.Equal(t, float64(bodySize.Load()), d.bytesSum, "upload_bytes must be the compressed body size")
	assert.EqualValues(t, 1, d.durationCount)
}

func TestUploadTelemetryRetryThenSuccess(t *testing.T) {
	var bodySize atomic.Int64
	in := newSizedIntake(t, &bodySize, http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusAccepted)
	u := newTestUploader(t, newTestConfig(t, in))
	req, _ := pcapRequest([]byte("pcap"))

	before := snapshotUploadMetrics(u)
	require.NoError(t, u.Upload(context.Background(), req))
	d := snapshotUploadMetrics(u).since(before)

	assert.Equal(t, map[[2]string]float64{
		{"5xx", "false"}: 1,
		{"5xx", "true"}:  1,
		{"2xx", "true"}:  1,
	}, d.attempts)
	assert.Equal(t, map[[2]string]float64{{"reliable", "success"}: 1}, d.results)
	assert.EqualValues(t, 1, d.bytesCount)
	assert.Equal(t, float64(bodySize.Load()), d.bytesSum)
	assert.EqualValues(t, 1, d.durationCount)
}

func TestUploadTelemetryTooLarge(t *testing.T) {
	in := newIntake(t, http.StatusRequestEntityTooLarge)
	u := newTestUploader(t, newTestConfig(t, in))
	req, _ := pcapRequest([]byte("pcap"))

	before := snapshotUploadMetrics(u)
	require.Error(t, u.Upload(context.Background(), req))
	d := snapshotUploadMetrics(u).since(before)

	assert.Equal(t, map[[2]string]float64{{"4xx", "false"}: 1}, d.attempts)
	assert.Equal(t, map[[2]string]float64{{"reliable", "too_large"}: 1}, d.results)
	assert.Zero(t, d.bytesCount, "only successful uploads are sized")
	assert.EqualValues(t, 1, d.durationCount)
}

func TestUploadTelemetryTransportError(t *testing.T) {
	// A closed server leaves an address nothing listens on.
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	cfg := configmock.New(t)
	cfg.SetInTest("api_key", "main-api-key")
	cfg.SetInTest(testPrefix+"logs_dd_url", closed.URL)
	u := newTestUploader(t, cfg)
	req, _ := pcapRequest([]byte("pcap"))

	before := snapshotUploadMetrics(u)
	require.Error(t, u.Upload(context.Background(), req))
	d := snapshotUploadMetrics(u).since(before)

	assert.Equal(t, map[[2]string]float64{
		{"transport", "false"}: 1,
		{"transport", "true"}:  defaultMaxAttempts - 1,
	}, d.attempts)
	assert.Equal(t, map[[2]string]float64{{"reliable", "failed"}: 1}, d.results)
	assert.Zero(t, d.bytesCount)
	assert.EqualValues(t, 1, d.durationCount)
}

// Each endpoint is counted on its own, tagged by whether it is reliable.
func TestUploadTelemetryCountsEachEndpoint(t *testing.T) {
	main := newIntake(t)
	unreliable := newIntake(t, http.StatusBadRequest)
	cfg := newTestConfig(t, main)
	cfg.SetInTest(testPrefix+"additional_endpoints", []map[string]interface{}{
		{"api_key": "unreliable-api-key", "host": hostOf(t, unreliable), "port": portOf(t, unreliable), "use_ssl": false, "is_reliable": false},
	})
	u := newTestUploader(t, cfg)
	req, _ := pcapRequest([]byte("pcap"))

	before := snapshotUploadMetrics(u)
	require.NoError(t, u.Upload(context.Background(), req))
	d := snapshotUploadMetrics(u).since(before)

	assert.Equal(t, map[[2]string]float64{
		{"reliable", "success"}:  1,
		{"unreliable", "failed"}: 1,
	}, d.results)
	assert.EqualValues(t, 1, d.durationCount, "duration is per Upload call, not per endpoint")
}
