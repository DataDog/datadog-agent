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
	"strings"

	"github.com/DataDog/agent-payload/v5/gogen"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gorilla/mux"
)

const (
	maxHTTPResponseReadBytes = 64 * 1024
	encodingGzip             = "gzip"
	encodingDeflate          = "deflate"
	nodeMetricHandlerName    = "node-metrics-handler"
)

// InstallNodeMetricsEndpoints installs node metrics collection endpoints
func InstallNodeMetricsEndpoints(ctx context.Context, r *mux.Router, cfg config.Component) {
	service := newHandler(cfg)
	handler := api.WithLeaderProxyHandler(
		nodeMetricHandlerName,
		service.prehandler,
		service.leaderHandler,
	)
	r.HandleFunc("/series", api.WithTelemetryWrapper(nodeMetricHandlerName, handler)).Methods("POST")
}

type Handler struct {
	cfg config.Component
}

func newHandler(cfg config.Component) Handler {
	return Handler{cfg: cfg}
}

func (h *Handler) prehandler(w http.ResponseWriter, r *http.Request) bool {
	return true
}

func (h *Handler) leaderHandler(w http.ResponseWriter, r *http.Request) {
	protoInspector := func(payload []byte) error {
		metricPayload := &gogen.MetricPayload{}

		if err := metricPayload.Unmarshal(payload); err != nil {
			return err
		}
		h.printSeries(metricPayload)

		return nil
	}
	err, rc := getReaderFromRequest(r)
	if err != nil {
		return
	}

	all, err := io.ReadAll(rc)
	if err != nil {
		return
	}

	err = protoInspector(all)
	w.WriteHeader(http.StatusOK)
	return
}

func (h *Handler) printSeries(payload *gogen.MetricPayload) {
	for _, series := range payload.Series {
		log.Infof("Metric=%q\t Tags=%q", series.Metric, strings.Join(series.Tags, ", "))
	}
}

func getReaderFromRequest(r *http.Request) (error, io.ReadCloser) {
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
	return err, rc
}
