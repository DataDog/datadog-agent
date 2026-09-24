// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpimpl

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/dogstatsdhttp"
)

type requestCtx struct {
	prefix string
	log    log.Component
	writer http.ResponseWriter
}

func (ctx *requestCtx) debugf(format string, args ...any) {
	ctx.log.Debugf(ctx.prefix+format, args...)
}

func (ctx *requestCtx) respond(status int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	ctx.debugf("complete with status %d: %q", status, msg)
	ctx.writer.WriteHeader(status)
	_, err := ctx.writer.Write([]byte(msg))
	if err != nil {
		ctx.debugf("failed to write response: %v", err)
	}
}

type handlerBase struct {
	log        log.Component
	tagger     tagger.Component
	hostname   string
	filterList filterlist.Component
	out        serializer
	tlm        endpointTelemetry
	sem        semaphore

	// maxPayloadSize caps the request body we are willing to buffer. Zero or
	// less disables the cap.
	maxPayloadSize int64
}

func (h *handlerBase) handle(
	w http.ResponseWriter, r *http.Request,
	processPayload func(orig origin, payload *pb.Payload) (payloadStats, error),
) {
	start := time.Now()
	defer func() {
		h.tlm.requestDuration.Add(time.Since(start).Seconds())
	}()

	ctx := requestCtx{
		prefix: fmt.Sprintf("dogstatsdhttp %q: ", r.RemoteAddr),
		log:    h.log,
		writer: w,
	}

	// Claimed before anything else so that an overloaded server spends as little
	// as possible on a request it is going to refuse. The client is expected to
	// retry.
	if !h.sem.acquire() {
		h.tlm.requestOverloaded.Inc()
		ctx.respond(http.StatusServiceUnavailable, "too many requests")
		return
	}
	defer h.sem.release()

	origin, err := originFromHeader(r.Header, h.tagger)
	if err != nil {
		h.tlm.requestOriginError.Inc()
		ctx.respond(http.StatusBadRequest, "origin detection error: %v", err)
		return
	}

	reader := io.Reader(r.Body)
	if h.maxPayloadSize > 0 {
		reader = http.MaxBytesReader(w, r.Body, h.maxPayloadSize)
	}
	body, err := io.ReadAll(reader)
	r.Body.Close()
	// A failed read still consumed whatever it returned.
	h.tlm.requestBytes.Add(float64(len(body)))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			h.tlm.requestTooLarge.Inc()
			ctx.respond(http.StatusRequestEntityTooLarge, "payload exceeds the %d byte limit", h.maxPayloadSize)
			return
		}
		// The read deadline comes from the server's ReadTimeout, so the client
		// is slow rather than wrong.
		if errors.Is(err, os.ErrDeadlineExceeded) {
			h.tlm.requestTimeout.Inc()
			ctx.respond(http.StatusRequestTimeout, "timed out reading request body")
			return
		}
		h.tlm.requestReadError.Inc()
		ctx.respond(http.StatusBadRequest, "error reading body: %v", err)
		return
	}

	var payload pb.Payload
	if err = payload.UnmarshalVT(body); err != nil {
		h.tlm.requestParseError.Inc()
		ctx.respond(http.StatusBadRequest, "error parsing payload: %v", err)
		return
	}

	stats, err := processPayload(origin, &payload)
	stats.report(&h.tlm)
	if err != nil {
		h.tlm.requestProcessError.Inc()
		ctx.respond(http.StatusBadRequest, "error processing payload: %v", err)
		return
	}

	h.tlm.requestOK.Inc()
	ctx.respond(http.StatusOK, "OK")
}

type seriesHandler struct {
	handlerBase
}

func (h *seriesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handle(w, r, func(origin origin, payload *pb.Payload) (payloadStats, error) {
		it, err := newSeriesIterator(payload, origin, h.hostname, h.filterList.GetMetricFilterList())
		if err != nil {
			return payloadStats{}, err
		}
		err = h.out.SendIterableSeries(it)
		if err == nil {
			err = it.err
		}
		return it.stats, err
	})
}

type sketchesHandler struct {
	handlerBase
}

func (h *sketchesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handle(w, r, func(origin origin, payload *pb.Payload) (payloadStats, error) {
		it, err := newSketchIterator(payload, origin, h.hostname, h.filterList.GetMetricFilterList())
		if err != nil {
			return payloadStats{}, err
		}
		err = h.out.SendSketch(it)
		if err == nil {
			err = it.err
		}
		return it.stats, err
	})
}
