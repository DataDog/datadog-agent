// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpimpl

import (
	"fmt"
	"io"
	"net/http"
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

	origin, err := originFromHeader(r.Header, h.tagger)
	if err != nil {
		h.tlm.requestOriginError.Inc()
		ctx.respond(http.StatusBadRequest, "origin detection error: %v", err)
		return
	}

	body, err := io.ReadAll(r.Body)
	r.Body.Close()
	// A failed read still consumed whatever it returned.
	h.tlm.requestBytes.Add(float64(len(body)))
	if err != nil {
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
