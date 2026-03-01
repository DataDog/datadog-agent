// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpimpl

import (
	"fmt"
	"io"
	"net/http"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
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

func (ctx *requestCtx) errorf(format string, args ...any) {
	ctx.log.Errorf(ctx.prefix+format, args...)
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
	log      log.Component
	tagger   tagger.Component
	hostname string
	out      serializer
}

type seriesHandler struct {
	handlerBase
}

func (h *seriesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := requestCtx{
		prefix: fmt.Sprintf("dogstatsdhttp %q: ", r.RemoteAddr),
		log:    h.log,
		writer: w,
	}

	origin, err := originFromHeader(r.Header, h.tagger)
	if err != nil {
		ctx.respond(http.StatusBadRequest, "origin detection error: %v", err)
		return
	}

	body, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		ctx.respond(http.StatusBadRequest, "error reading body: %v", err)
		return
	}

	var payload pb.Payload
	err = payload.UnmarshalVT(body)
	if err != nil {
		ctx.respond(http.StatusBadRequest, "error parsing payload: %v", err)
		return
	}

	it, err := newSeriesIterator(&payload, origin, h.hostname)
	if err != nil {
		ctx.respond(http.StatusBadRequest, "error decoding payload dictionaries: %v", err)
		return
	}

	err = h.out.SendIterableSeries(it)
	if err != nil {
		// this error indicates a failure in the agent itself, so we don't
		// propagate it to the client
		ctx.errorf("failed to serialize payload: %v", err)
		ctx.respond(http.StatusInternalServerError, "internal error")
		return
	}

	if it.err != nil {
		ctx.respond(http.StatusBadRequest, "error decoding payload: %v", it.err)
		return
	}

	ctx.respond(http.StatusOK, "OK")
}
