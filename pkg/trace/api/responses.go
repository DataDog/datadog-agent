// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	receiverErrorKey = "datadog.trace_agent.receiver.error"
)

// We encaspulate the answers in a container, this is to ease-up transition,
// should we add another fied.
type traceResponse struct {
	// All the sampling rates recommended, by service
	Rates map[string]float64 `json:"rate_by_service"`
}

// httpFormatError is used for payload format errors
func httpFormatError(w http.ResponseWriter, v Version, err error) {
	log.Errorf("Rejecting client request: %v", err)
	tags := []string{"error:format-error", "version:" + string(v)}
	metrics.Count(receiverErrorKey, 1, tags, 1)
	http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
}

// httpDecodingError is used for errors happening in decoding
func httpDecodingError(err error, tags []string, w http.ResponseWriter) {
	status := http.StatusBadRequest
	errtag := "decoding-error"
	msg := err.Error()

	switch err {
	case ErrLimitedReaderLimitReached:
		status = http.StatusRequestEntityTooLarge
		errtag = "payload-too-large"
		msg = errtag
	case io.EOF, io.ErrUnexpectedEOF:
		errtag = "unexpected-eof"
		msg = errtag
	}
	if err, ok := err.(net.Error); ok && err.Timeout() {
		status = http.StatusRequestTimeout
		errtag = "timeout"
		msg = errtag
	}

	tags = append(tags, fmt.Sprintf("error:%s", errtag))
	metrics.Count(receiverErrorKey, 1, tags, 1)
	http.Error(w, msg, status)
}

// httpOK is a dumb response for when things are a OK
func httpOK(w http.ResponseWriter) {
	io.WriteString(w, "OK\n")
}

// httpRateByService outputs, as a JSON, the recommended sampling rates for all services.
func httpRateByService(w http.ResponseWriter, dynConf *sampler.DynamicConfig) {
	w.Header().Set("Content-Type", "application/json")
	response := traceResponse{
		Rates: dynConf.RateByService.GetAll(), // this is thread-safe
	}
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(response); err != nil {
		tags := []string{"error:response-error"}
		metrics.Count(receiverErrorKey, 1, tags, 1)
		return
	}
}
