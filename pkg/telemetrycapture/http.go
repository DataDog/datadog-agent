// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetrycapture

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"
)

const maxControlBytes = 4096
const controlTimeout = 5 * time.Second

// Handler returns the local session API, rooted at /. Daemons mount it below
// their capture prefix. Authentication is mandatory, including for capabilities
// and status; system-probe does not authenticate its router globally.
// A schedule provider may describe currently scheduled core checks without
// mutating the manager or running a check. Other producers omit it.
func (m *Manager) Handler(authenticate func(http.Handler) http.Handler, scheduleProvider ...func() []MetricSchedule) (http.Handler, error) {
	if authenticate == nil || len(scheduleProvider) > 1 {
		return nil, ErrRequest
	}
	var schedules func() []MetricSchedule
	if len(scheduleProvider) == 1 {
		schedules = scheduleProvider[0]
	}
	enrich := func(status Status) Status { return withMetricSchedules(status, schedules) }
	mux := http.NewServeMux()
	status := func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, enrich(m.Status())) }
	mux.HandleFunc("GET /capabilities", status)
	mux.HandleFunc("GET /status", status)
	mux.HandleFunc("POST /prepare", func(w http.ResponseWriter, r *http.Request) {
		var request PrepareRequest
		if !readControl(w, r, &request) {
			return
		}
		status, err := m.Prepare(request)
		writeControl(w, enrich(status), err)
	})
	for path, operation := range map[string]func(Control) (Status, error){
		"/activate":  m.Activate,
		"/heartbeat": m.Heartbeat,
	} {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			var request Control
			if !readControl(w, r, &request) {
				return
			}
			status, err := operation(request)
			writeControl(w, enrich(status), err)
		})
	}
	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, r *http.Request) {
		var request Control
		if !readControl(w, r, &request) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), controlTimeout)
		defer cancel()
		status, err := m.Stop(ctx, request)
		writeControl(w, enrich(status), err)
	})
	mux.HandleFunc("POST /records", func(w http.ResponseWriter, r *http.Request) { m.serveRecords(w, r, schedules) })
	return authenticate(mux), nil
}

// Normalize into at most seven fixed family names and own every exported slice.
// Multiple check instances use the longest schedule so capture can wait for all.
func withMetricSchedules(status Status, provider func() []MetricSchedule) Status {
	if provider == nil {
		return status
	}
	for i, capability := range status.Capabilities {
		if capability.Stream != Metrics {
			continue
		}
		byFamily := make(map[string]time.Duration, 7)
		for _, schedule := range provider() {
			family := MetricCheckFamily(schedule.Family)
			if family != "" && schedule.Cadence > byFamily[family] {
				byFamily[family] = schedule.Cadence
			}
		}
		owned := make([]MetricSchedule, 0, len(byFamily))
		for family, cadence := range byFamily {
			owned = append(owned, MetricSchedule{Family: family, Cadence: cadence})
		}
		slices.SortFunc(owned, func(a, b MetricSchedule) int { return strings.Compare(a.Family, b.Family) })
		status.Capabilities = slices.Clone(status.Capabilities)
		status.Capabilities[i].MetricSchedules = owned
		break
	}
	return status
}

func readControl(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxControlBytes)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil {
		writeError(w, ErrRequest)
		return false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		writeError(w, ErrRequest)
		return false
	}
	return true
}

func writeControl(w http.ResponseWriter, status Status, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, status)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	code := http.StatusConflict
	message := "capture operation failed"
	switch {
	case errors.Is(err, ErrProtocol):
		code, message = http.StatusBadRequest, ErrProtocol.Error()
	case errors.Is(err, ErrRequest), errors.Is(err, ErrCursor):
		code, message = http.StatusBadRequest, "invalid capture request"
	case errors.Is(err, ErrClosed):
		code, message = http.StatusServiceUnavailable, ErrClosed.Error()
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, message, code)
}

func (m *Manager) serveRecords(w http.ResponseWriter, r *http.Request, schedules func() []MetricSchedule) {
	var request ReadRequest
	if !readControl(w, r, &request) {
		return
	}
	// Fail closed if middleware hides write-deadline support. Otherwise a slow
	// reader could keep raw records pinned beyond session expiry or shutdown.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(controlTimeout)); err != nil {
		_ = m.Fail(request.Control)
		writeError(w, ErrFailed)
		return
	}
	batch, err := m.Read(request)
	if err != nil {
		writeError(w, err)
		return
	}
	defer batch.Release()
	batch.Status = withMetricSchedules(batch.Status, schedules)
	if !batch.reserveEncoding() {
		writeError(w, ErrFailed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := writeBatch(w, batch); err != nil {
		_ = m.Fail(request.Control)
	}
}

// Stream directly through bounded, explicitly owned scratch space. encoding/json
// pools its encode buffer, which would retain raw telemetry after release.
func writeBatch(w io.Writer, batch *Batch) error {
	stream := &jsonStream{writer: w}
	defer clear(stream.buffer[:])
	stream.text(`{"status":`)
	stream.value(reflect.ValueOf(batch.Status))
	stream.text(`,"records":[`)
	for i, record := range batch.Records {
		if i != 0 {
			stream.text(",")
		}
		stream.value(reflect.ValueOf(record))
	}
	stream.text("]}\n")
	stream.flush()
	return stream.err
}
