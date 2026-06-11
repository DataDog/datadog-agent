// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package demultiplexerendpointimpl component provides the /dogstatsd-contexts-dump API endpoint that can register via Fx value groups.
package demultiplexerendpointimpl

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/DataDog/zstd"

	demultiplexerComp "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/lookbacksender"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// Requires defines the dependencies for the demultiplexerendpoint component
type Requires struct {
	Log           log.Component
	Config        config.Component
	Demultiplexer demultiplexerComp.Component
}

type demultiplexerEndpoint struct {
	demux  demultiplexerComp.Component
	config config.Component
	log    log.Component
}

// Provides defines the output of the demultiplexerendpoint component
type Provides struct {
	Endpoint             api.AgentEndpointProvider
	LookbackDumpEndpoint api.AgentEndpointProvider
	LookbackSeedEndpoint api.AgentEndpointProvider
}

// NewComponent creates a new demultiplexerendpoint component
func NewComponent(reqs Requires) Provides {
	endpoint := demultiplexerEndpoint{
		demux:  reqs.Demultiplexer,
		config: reqs.Config,
		log:    reqs.Log,
	}

	return Provides{
		Endpoint:             api.NewAgentEndpointProvider(endpoint.dumpDogstatsdContexts, "/dogstatsd-contexts-dump", "POST"),
		LookbackDumpEndpoint: api.NewAgentEndpointProvider(endpoint.dumpLookback, "/metric-lookback-dump", "POST"),
		LookbackSeedEndpoint: api.NewAgentEndpointProvider(endpoint.seedLookback, "/metric-lookback-seed", "POST"),
	}
}

type lookbackSenderManagerProvider interface {
	LookbackSenderManager() *lookbacksender.SenderManager
}

// lookbackDumpResponse is the JSON body returned by the /metric-lookback-dump endpoint.
type lookbackDumpResponse struct {
	SeriesDumped int `json:"series_dumped"`
}

// dumpLookback flushes the retained metric lookback samples through the
// serializer and reports how many series were sent.
func (demuxendpoint demultiplexerEndpoint) dumpLookback(w http.ResponseWriter, _ *http.Request) {
	count, err := demuxendpoint.demux.DumpLookback()
	if err != nil {
		httputils.SetJSONError(w, demuxendpoint.log.Errorf("Failed to dump metric lookback buffer: %v", err), 500)
		return
	}

	resp, err := json.Marshal(lookbackDumpResponse{SeriesDumped: count})
	if err != nil {
		httputils.SetJSONError(w, demuxendpoint.log.Errorf("Failed to serialize response: %v", err), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}

// lookbackSeedRequest is the JSON body accepted by the demo-only
// /metric-lookback-seed endpoint.
type lookbackSeedRequest struct {
	CheckID  string   `json:"check_id"`
	Metric   string   `json:"metric"`
	Value    float64  `json:"value"`
	Hostname string   `json:"hostname,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Type     string   `json:"type,omitempty"`
}

// lookbackSeedResponse is the JSON body returned by /metric-lookback-seed.
type lookbackSeedResponse struct {
	CheckID         string `json:"check_id"`
	Metric          string `json:"metric"`
	Type            string `json:"type"`
	SamplesBuffered int    `json:"samples_buffered"`
}

// seedLookback writes a single scalar sample through the metric lookback shadow
// sender. It is intended only for live demos before the scheduler exists and is
// gated by metric_lookback.debug_seed.enabled.
func (demuxendpoint demultiplexerEndpoint) seedLookback(w http.ResponseWriter, r *http.Request) {
	if demuxendpoint.config == nil || !demuxendpoint.config.GetBool("metric_lookback.debug_seed.enabled") {
		httputils.SetJSONError(w, demuxendpoint.log.Errorf("metric lookback debug seed endpoint is disabled"), http.StatusForbidden)
		return
	}

	var req lookbackSeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.SetJSONError(w, demuxendpoint.log.Errorf("Invalid metric lookback seed request: %v", err), http.StatusBadRequest)
		return
	}
	if req.CheckID == "" {
		req.CheckID = "demo-shadow"
	}
	if req.Metric == "" {
		httputils.SetJSONError(w, demuxendpoint.log.Errorf("metric lookback seed request missing metric"), http.StatusBadRequest)
		return
	}
	if req.Type == "" {
		req.Type = "gauge"
	}
	req.Type = strings.ToLower(req.Type)

	provider, ok := demuxendpoint.demux.(lookbackSenderManagerProvider)
	if !ok {
		httputils.SetJSONError(w, demuxendpoint.log.Errorf("demultiplexer does not expose metric lookback sender manager"), http.StatusInternalServerError)
		return
	}
	manager := provider.LookbackSenderManager()
	if manager == nil {
		httputils.SetJSONError(w, demuxendpoint.log.Errorf("metric lookback is disabled"), http.StatusConflict)
		return
	}

	sender, err := manager.GetSender(checkid.ID(req.CheckID))
	if err != nil {
		httputils.SetJSONError(w, demuxendpoint.log.Errorf("Failed to get metric lookback sender: %v", err), http.StatusInternalServerError)
		return
	}

	switch req.Type {
	case "gauge":
		sender.Gauge(req.Metric, req.Value, req.Hostname, req.Tags)
	case "count":
		sender.Count(req.Metric, req.Value, req.Hostname, req.Tags)
	case "rate":
		sender.Rate(req.Metric, req.Value, req.Hostname, req.Tags)
	default:
		httputils.SetJSONError(w, demuxendpoint.log.Errorf("unsupported metric lookback seed type %q", req.Type), http.StatusBadRequest)
		return
	}
	sender.Commit()

	resp, err := json.Marshal(lookbackSeedResponse{
		CheckID:         req.CheckID,
		Metric:          req.Metric,
		Type:            req.Type,
		SamplesBuffered: 1,
	})
	if err != nil {
		httputils.SetJSONError(w, demuxendpoint.log.Errorf("Failed to serialize response: %v", err), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}

func (demuxendpoint demultiplexerEndpoint) dumpDogstatsdContexts(w http.ResponseWriter, _ *http.Request) {
	path, err := demuxendpoint.writeDogstatsdContexts()
	if err != nil {
		httputils.SetJSONError(w, demuxendpoint.log.Errorf("Failed to create dogstatsd contexts dump: %v", err), 500)
		return
	}

	resp, err := json.Marshal(path)
	if err != nil {
		httputils.SetJSONError(w, demuxendpoint.log.Errorf("Failed to serialize response: %v", err), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}

func (demuxendpoint demultiplexerEndpoint) writeDogstatsdContexts() (string, error) {
	path := path.Join(demuxendpoint.config.GetString("run_path"), "dogstatsd_contexts.json.zstd")

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}

	c := zstd.NewWriter(f)

	w := bufio.NewWriter(c)

	for _, err := range []error{demuxendpoint.demux.DumpDogstatsdContexts(w), w.Flush(), c.Close(), f.Close()} {
		if err != nil {
			return "", err
		}
	}

	return path, nil
}
