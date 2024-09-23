// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package series

import (
	"compress/gzip"
	"compress/zlib"
	"context"
	"io"
	"net/http"

	"github.com/DataDog/agent-payload/v5/gogen"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	"github.com/gorilla/mux"
)

const (
	encodingGzip    = "gzip"
	encodingDeflate = "deflate"
)

// InstallNodeMetricsEndpoints register handler for node metrics collection
func InstallNodeMetricsEndpoints(ctx context.Context, r *mux.Router, _ config.Component) {
	handler := NewSeriesHandler(ctx)
	r.HandleFunc("/series", api.WithTelemetryWrapper("node-metrics-handler", handler.Handle)).Methods("POST")
}

// Handler handles the series request and store the metrics to loadstore
type Handler struct {
	jobQueue *jobQueue
}

func NewSeriesHandler(ctx context.Context) *Handler {
	handler := Handler{
		jobQueue: newJobQueue(ctx),
	}
	return &handler
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	var err error
	var rc io.ReadCloser
	switch r.Header.Get("Content-Encoding") {
	case encodingGzip:
		rc, err = gzip.NewReader(r.Body)
	case encodingDeflate:
		rc, err = zlib.NewReader(r.Body)
	default:
		rc = r.Body
	}
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	payload, err := io.ReadAll(rc)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	metricPayload := &gogen.MetricPayload{}
	if err := metricPayload.Unmarshal(payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	h.jobQueue.addJob(metricPayload)
	w.WriteHeader(http.StatusOK)
	return
}
