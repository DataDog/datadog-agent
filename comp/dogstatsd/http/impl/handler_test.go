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
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	taggerimpl "github.com/DataDog/datadog-agent/comp/core/tagger/impl"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetaimpl "github.com/DataDog/datadog-agent/comp/core/workloadmeta/impl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// newFakeTagger builds a mock tagger without fx. SetupFakeTagger cannot be used
// by a test running inside a synctest bubble: it starts an fx app, and
// workloadmeta's OnStart hook reaches the process-global context in
// pkg/util/common, whose Done channel is created lazily and would then belong to
// whichever bubble touched it first, which is fatal for every later user.
func newFakeTagger(t *testing.T) taggermock.Mock {
	cfg := config.NewMock(t)
	lg := logmock.New(t)

	// The lifecycle is deliberately never started, so that hook never runs.
	wmeta := workloadmetaimpl.NewWorkloadMetaMock(workloadmetaimpl.Dependencies{
		Lc:     compdef.NewTestLifecycle(t),
		Log:    lg,
		Config: cfg,
		Params: workloadmeta.NewParams(),
	})

	return taggerimpl.NewMock(taggerimpl.MockRequires{
		Config:       cfg,
		WorkloadMeta: wmeta,
		Log:          lg,
		Telemetry:    telemetrymock.New(t),
	}).Comp
}

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
			tagger:         newFakeTagger(t),
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

// requestCount returns the value of the requests counter for the given endpoint
// and status, or zero if the handler never touched it.
func requestCount(t *testing.T, tlm telemetry.Mock, endpoint, status string) float64 {
	t.Helper()

	found, err := tlm.GetCountMetric(telemetrySubsystem, "requests")
	require.NoError(t, err)

	for _, metric := range found {
		tags := metric.Tags()
		if tags["endpoint"] == endpoint && tags["status"] == status {
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
		require.Equal(t, 1.0, requestCount(t, f.tlm, "series", "ok"))
		require.Zero(t, requestCount(t, f.tlm, "series", "too_large"))
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

		require.Equal(t, 1.0, requestCount(t, f.tlm, "series", "too_large"))
		require.Zero(t, requestCount(t, f.tlm, "series", "ok"))
		require.Zero(t, requestCount(t, f.tlm, "series", "parse_error"))
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
		require.Equal(t, 1.0, requestCount(t, f.tlm, "series", "parse_error"))
		require.Zero(t, requestCount(t, f.tlm, "series", "too_large"))
	})
}

// blockingSerializer parks every series request until release is closed, so a
// test can hold a known number of requests in flight.
type blockingSerializer struct {
	release chan struct{}
}

func (s *blockingSerializer) SendIterableSeries(source metrics.SerieSource) error {
	<-s.release
	for source.MoveNext() {
	}
	return nil
}

func (s *blockingSerializer) SendSketch(metrics.SketchesSource) error {
	return nil
}

// concurrencyFixture holds a series and a sketches handler sharing one
// semaphore, which is what makes the limit process-wide rather than per-endpoint.
type concurrencyFixture struct {
	series   http.Handler
	sketches http.Handler
	out      *blockingSerializer
	tlm      telemetry.Mock
}

func newConcurrencyFixture(t *testing.T, maxConcurrent int) *concurrencyFixture {
	t.Helper()

	out := &blockingSerializer{release: make(chan struct{})}
	tlm := telemetrymock.New(t)
	store := newTelemetryStore(tlm)
	fakeTagger := newFakeTagger(t)
	sem := newSemaphore(maxConcurrent)

	base := func(endpoint string) handlerBase {
		return handlerBase{
			log:        logmock.New(t),
			tagger:     fakeTagger,
			hostname:   "default",
			filterList: filterlistimpl.NewNoopFilterList(),
			out:        out,
			tlm:        store.forEndpoint(endpoint),
			sem:        sem,
		}
	}

	return &concurrencyFixture{
		series:   &seriesHandler{base("series")},
		sketches: &sketchesHandler{base("sketches")},
		out:      out,
		tlm:      tlm,
	}
}

func (f *concurrencyFixture) post(h http.Handler, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header = http.Header{"X-Dsd-Ld": {"123456789"}}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// hold starts a request and returns once it has parked in the serializer, so its
// slot is taken by the time the caller looks at it.
func (f *concurrencyFixture) hold(h http.Handler, body []byte) <-chan *httptest.ResponseRecorder {
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- f.post(h, body) }()
	synctest.Wait()
	return done
}

// Each case runs in a synctest bubble. A request that is wrongly admitted parks
// in the serializer, leaving every goroutine in the bubble blocked, which
// synctest reports at once as a deadlock instead of letting the test hang.
func TestHandlerConcurrencyLimit(t *testing.T) {
	body, err := seriesTestPayload().MarshalVT()
	require.NoError(t, err)

	t.Run("a request beyond the limit is refused with 503", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			f := newConcurrencyFixture(t, 1)
			held := f.hold(f.series, body) // takes the only slot

			w := f.post(f.series, body)
			require.Equal(t, http.StatusServiceUnavailable, w.Code)
			require.Equal(t, 1.0, requestCount(t, f.tlm, "series", "overloaded"))

			close(f.out.release)
			require.Equal(t, http.StatusOK, (<-held).Code)
		})
	})

	t.Run("the limit spans endpoints", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			f := newConcurrencyFixture(t, 1)
			held := f.hold(f.series, body)

			// A different endpoint, but the same single slot is taken. The body
			// is refused before it is read, so it need not be a sketch payload.
			w := f.post(f.sketches, nil)
			require.Equal(t, http.StatusServiceUnavailable, w.Code)
			require.Equal(t, 1.0, requestCount(t, f.tlm, "sketches", "overloaded"))
			require.Zero(t, requestCount(t, f.tlm, "series", "overloaded"))

			close(f.out.release)
			require.Equal(t, http.StatusOK, (<-held).Code)
		})
	})

	t.Run("a slot is returned once the request completes", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			f := newConcurrencyFixture(t, 1)
			close(f.out.release) // nothing blocks

			for i := 0; i < 3; i++ {
				require.Equal(t, http.StatusOK, f.post(f.series, body).Code)
			}
			require.Equal(t, 3.0, requestCount(t, f.tlm, "series", "ok"))
			require.Zero(t, requestCount(t, f.tlm, "series", "overloaded"))
		})
	})

	t.Run("a limit of zero disables the cap", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			f := newConcurrencyFixture(t, 0)

			const requests = 4
			done := make(chan *httptest.ResponseRecorder, requests)
			for i := 0; i < requests; i++ {
				go func() { done <- f.post(f.series, body) }()
			}
			// All four park in the serializer at once. Under a limit of 1 only
			// the first would have got this far, and the rest would answer 503.
			synctest.Wait()

			close(f.out.release)
			for i := 0; i < requests; i++ {
				require.Equal(t, http.StatusOK, (<-done).Code)
			}
			require.Zero(t, requestCount(t, f.tlm, "series", "overloaded"))
		})
	})
}
