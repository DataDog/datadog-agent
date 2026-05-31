// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package ksmaggregation implements the cluster-agent side of KSM node-aggregate
// collection. Node agents POST their per-node KSM partial to this endpoint; the
// cluster-agent stores them and combines them into the authoritative .total series.
package ksmaggregation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/ksmaggregation"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const handlerName = "ksm-aggregation-handler"

type handler struct {
	store      *ksmaggregation.PartialStore
	enabled    bool
	partialTTL time.Duration
}

// InstallKSMAggregationEndpoints registers the /ksmaggregation endpoint on the
// cluster-agent API router. The handler is wrapped with the leader-proxy so that
// a partial received by a follower is forwarded to the leader, whose process-local
// store is the one the cluster_aggregates_only check reads from.
func InstallKSMAggregationEndpoints(_ context.Context, r *http.ServeMux, cfg config.Component) {
	h := &handler{
		store:      ksmaggregation.GetStore(),
		enabled:    cfg.GetBool("kubernetes_state_core.cluster_aggregates.enabled"),
		partialTTL: time.Duration(cfg.GetInt("kubernetes_state_core.cluster_aggregates.partial_ttl_seconds")) * time.Second,
	}
	leaderHandler := api.WithLeaderProxyHandler(handlerName, h.preHandler, h.leaderHandler)
	r.HandleFunc("POST /ksmaggregation", api.WithTelemetryWrapper(handlerName, leaderHandler))
}

// preHandler runs on both leader and followers; it validates the request before
// it is either forwarded to the leader or handled locally.
func (h *handler) preHandler(w http.ResponseWriter, r *http.Request) bool {
	if !h.enabled {
		http.Error(w, "KSM cluster aggregates feature is disabled", http.StatusServiceUnavailable)
		return false
	}
	if r.Body == nil {
		http.Error(w, "request body is empty", http.StatusBadRequest)
		return false
	}
	return true
}

// leaderHandler runs only on the leader: it stores the node partial and returns the verdict.
func (h *handler) leaderHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read body: %v", err), http.StatusBadRequest)
		return
	}

	var req clusteragent.KSMNodePartialRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("failed to unmarshal body: %v", err), http.StatusBadRequest)
		return
	}
	if req.NodeName == "" {
		http.Error(w, "node_name is required", http.StatusBadRequest)
		return
	}

	partial := ksmaggregation.NodePartial{Metrics: make(map[string][]ksmaggregation.AggValue, len(req.Metrics))}
	for metricName, vals := range req.Metrics {
		aggVals := make([]ksmaggregation.AggValue, len(vals))
		for i, v := range vals {
			aggVals[i] = ksmaggregation.AggValue{Labels: v.Labels, Value: v.Value}
		}
		partial.Metrics[metricName] = aggVals
	}
	h.store.Upsert(req.NodeName, partial)

	reporting := h.store.ReportingNodes(h.partialTTL)
	// Only tell the node to suppress its local .total while the cluster_aggregates_only
	// check is an active emitter. The active window is set by the emitter itself (a small
	// multiple of its own interval, via MarkEmitterRun) — short and self-tracking, so a
	// stalled emitter lets nodes resume local emission within ~one interval rather than
	// going silent. Separate from the (longer) partial freshness TTL used for the combine.
	emitterActive := ksmaggregation.EmitterActive()
	log.Debugf("KSM aggregation: stored partial for %s; %d nodes reporting; emitter active=%t", req.NodeName, reporting, emitterActive)

	reply := clusteragent.KSMNodePartialReply{
		Accepted:      true,
		SuppressLocal: emitterActive,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(reply); err != nil {
		log.Warnf("KSM aggregation: failed to encode reply: %v", err)
	}
}
