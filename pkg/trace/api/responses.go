// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/trace/api/apiutil"
	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	receiverErrorKey = "datadog.trace_agent.receiver.error"
)

// httpFormatError is used for payload format errors
func httpFormatError(w http.ResponseWriter, v Version, err error, statsd statsd.ClientInterface) {
	log.Errorf("Rejecting client request: %v", err)
	tags := []string{"error:format-error", "version:" + string(v)}
	_ = statsd.Count(receiverErrorKey, 1, tags, 1)
	http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
}

// httpDecodingError is used for errors happening in decoding
func httpDecodingError(err error, tags []string, w http.ResponseWriter, statsd statsd.ClientInterface) {
	status := http.StatusBadRequest
	errtag := "decoding-error"
	msg := err.Error()

	switch err {
	case apiutil.ErrLimitedReaderLimitReached:
		status = http.StatusRequestEntityTooLarge
		errtag = "payload-too-large"
		msg = errtag
	case io.EOF, io.ErrUnexpectedEOF:
		errtag = "unexpected-eof"
		msg = errtag
	case context.DeadlineExceeded:
		status = http.StatusRequestTimeout
		errtag = "timeout"
		msg = errtag
	}
	if err, ok := err.(net.Error); ok && err.Timeout() {
		status = http.StatusRequestTimeout
		errtag = "timeout"
		msg = errtag
	}

	tags = append(tags, fmt.Sprintf("error:%s", errtag))
	_ = statsd.Count(receiverErrorKey, 1, tags, 1)
	http.Error(w, msg, status)
}

// httpOK is a dumb response for when things are a OK. It returns the number
// of bytes written along with a boolean specifying if the response was successful.
func httpOK(w http.ResponseWriter) (n uint64, ok bool) {
	nn, err := io.WriteString(w, "OK\n")
	return uint64(nn), err == nil
}

type writeCounter struct {
	w io.Writer
	n *atomic.Uint64
}

func newWriteCounter(w io.Writer) *writeCounter {
	return &writeCounter{
		w: w,
		n: atomic.NewUint64(0),
	}
}

func (wc *writeCounter) Write(p []byte) (n int, err error) {
	wc.n.Add(uint64(len(p)))
	return wc.w.Write(p)
}

func (wc *writeCounter) N() uint64 { return wc.n.Load() }

// httpRateByService outputs, as a JSON, the recommended sampling rates for all services.
// It returns the number of bytes written and a boolean specifying whether the write was
// successful.
func httpRateByService(ratesVersion string, w http.ResponseWriter, dynConf *sampler.DynamicConfig, statsd statsd.ClientInterface) (n uint64, ok bool) {
	wc := newWriteCounter(w)
	var err error
	defer func() {
		n, ok = wc.N(), err == nil
		if err != nil {
			tags := []string{"error:response-error"}
			_ = statsd.Count(receiverErrorKey, 1, tags, 1)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	body, currentVersion, berr := BuildRateByServiceJSON(ratesVersion, dynConf)
	if berr != nil {
		err = berr
		return
	}
	if ratesVersion != "" {
		w.Header().Set(header.RatesPayloadVersion, currentVersion)
	}
	_, err = wc.Write(body)
	return
}
