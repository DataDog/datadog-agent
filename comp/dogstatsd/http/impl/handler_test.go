// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpimpl

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// drainingSerializer consumes the iterators the way the real serializer does, so
// that a payload which reaches the serializer is fully decoded.
type drainingSerializer struct {
	series []*metrics.Serie
}

func (s *drainingSerializer) SendIterableSeries(source metrics.SerieSource) error {
	for source.MoveNext() {
		s.series = append(s.series, source.Current())
	}
	return nil
}

func (s *drainingSerializer) SendSketch(metrics.SketchesSource) error {
	return nil
}

type handlerFixture struct {
	handler http.Handler
	out     *drainingSerializer
	tlm     telemetry.Mock
}

func newHandlerFixture(t *testing.T, maxPayloadSize int64) *handlerFixture {
	t.Helper()

	out := &drainingSerializer{}
	tlm := telemetrymock.New(t)

	return &handlerFixture{
		handler: &seriesHandler{handlerBase{
			log:            logmock.New(t),
			tagger:         tagger.SetupFakeTagger(t),
			hostname:       "default",
			filterList:     filterlistimpl.NewNoopFilterList(),
			out:            out,
			tlm:            newTelemetryStore(tlm).forEndpoint("series"),
			maxPayloadSize: maxPayloadSize,
		}},
		out: out,
		tlm: tlm,
	}
}

func (f *handlerFixture) post(t *testing.T, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/series", body)
	req.Header = http.Header{"X-Dsd-Ld": {"123456789"}}

	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	return w
}

// requestCount returns the value of the requests counter for the given status,
// or zero if the handler never touched it.
func requestCount(t *testing.T, tlm telemetry.Mock, status string) float64 {
	t.Helper()

	found, err := tlm.GetCountMetric(telemetrySubsystem, "requests")
	require.NoError(t, err)

	for _, metric := range found {
		tags := metric.Tags()
		if tags["endpoint"] == "series" && tags["status"] == status {
			return metric.Value()
		}
	}
	return 0
}

func TestHandlerPayloadSizeLimit(t *testing.T) {
	t.Run("payload under the limit is accepted", func(t *testing.T) {
		f := newHandlerFixture(t, 256<<10)
		body, err := seriesTestPayload().MarshalVT()
		require.NoError(t, err)

		require.Equal(t, http.StatusOK, f.post(t, bytes.NewReader(body)).Code)
		require.Len(t, f.out.series, 3)
		require.Equal(t, 1.0, requestCount(t, f.tlm, "ok"))
		require.Zero(t, requestCount(t, f.tlm, "too_large"))
	})

	t.Run("payload over the limit is rejected with 413", func(t *testing.T) {
		f := newHandlerFixture(t, 1024)
		// The body never has to be a valid payload: the read fails before it is
		// ever unmarshalled.
		body := bytes.Repeat([]byte("x"), 2048)

		w := f.post(t, bytes.NewReader(body))
		require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
		require.Contains(t, w.Body.String(), "payload exceeds the 1024 byte limit")
		require.Empty(t, f.out.series)

		require.Equal(t, 1.0, requestCount(t, f.tlm, "too_large"))
		require.Zero(t, requestCount(t, f.tlm, "ok"))
		require.Zero(t, requestCount(t, f.tlm, "parse_error"))
	})

	t.Run("a limit of zero disables the cap", func(t *testing.T) {
		f := newHandlerFixture(t, 0)
		// A run of continuation bytes never terminates a varint, so this is
		// unparseable rather than a payload full of skipped unknown fields.
		body := bytes.Repeat([]byte{0xFF}, 512<<10)

		// It fails to parse, but it must get past the read to do so, which is
		// what proves the cap is off.
		w := f.post(t, bytes.NewReader(body))
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Equal(t, 1.0, requestCount(t, f.tlm, "parse_error"))
		require.Zero(t, requestCount(t, f.tlm, "too_large"))
	})
}
