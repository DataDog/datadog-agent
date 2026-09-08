// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package demultiplexerendpointimpl component provides the /dogstatsd-contexts-dump API endpoint that can register via Fx value groups.
package demultiplexerendpointimpl

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"sync"

	"github.com/DataDog/zstd"

	demultiplexerComp "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	dogstatsdconfig "github.com/DataDog/datadog-agent/comp/dogstatsd/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator/contexttop"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

const (
	defaultNumMetrics             = 10
	defaultNumTags                = 5
	maxNumMetrics                 = 50
	maxNumTags                    = 20
	topSourceLive                 = "live"
	topSourceDump                 = "dump"
	dogstatsdContextsDumpFilename = "dogstatsd_contexts.json.zstd"
)

var errDogstatsdOnDataPlane = errors.New("DogStatsD traffic is being served by the Agent Data Plane; run DogStatsD diagnostic commands against the agent-data-plane process instead")

type contextDumper interface {
	DumpDogstatsdContexts(io.Writer) error
}

// Requires defines the dependencies for the demultiplexerendpoint component
type Requires struct {
	Log           log.Component
	Config        config.Component
	Demultiplexer demultiplexerComp.Component
}

type demultiplexerEndpoint struct {
	demux                contextDumper
	runPath              string
	dogstatsdOnDataPlane bool
	log                  log.Component
	dumpMu               sync.Mutex
}

// Provides defines the output of the demultiplexerendpoint component
type Provides struct {
	DumpEndpoint api.AgentEndpointProvider
	TopEndpoint  api.AgentEndpointProvider
}

// NewComponent creates a new demultiplexerendpoint component
func NewComponent(reqs Requires) Provides {
	endpoint := demultiplexerEndpoint{
		demux:                reqs.Demultiplexer,
		runPath:              reqs.Config.GetString("run_path"),
		dogstatsdOnDataPlane: dogstatsdconfig.NewConfig(reqs.Config).EnabledDataPlane(),
		log:                  reqs.Log,
	}

	return Provides{
		DumpEndpoint: api.NewAgentEndpointProvider(endpoint.dumpDogstatsdContexts, "/dogstatsd-contexts-dump", "POST"),
		TopEndpoint:  api.NewAgentEndpointProvider(endpoint.topDogstatsdContexts, "/dogstatsd-contexts-top", "POST"),
	}
}

type topRequest struct {
	NumMetrics int    `json:"num_metrics"`
	NumTags    int    `json:"num_tags"`
	Source     string `json:"source"`
}

type topResponse struct {
	contexttop.Result
	Source string `json:"source"`
}

func (demuxendpoint *demultiplexerEndpoint) topDogstatsdContexts(w http.ResponseWriter, r *http.Request) {
	request := topRequest{NumMetrics: defaultNumMetrics, NumTags: defaultNumTags, Source: topSourceLive}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		httputils.SetJSONError(w, fmt.Errorf("invalid DogStatsD top request: %w", err), http.StatusBadRequest)
		return
	}

	if err := validateTopRequest(request); err != nil {
		httputils.SetJSONError(w, err, http.StatusBadRequest)
		return
	}

	var result contexttop.Result
	var err error
	switch request.Source {
	case topSourceLive:
		result, err = demuxendpoint.getDogstatsdTop(request.NumMetrics, request.NumTags)
	case topSourceDump:
		result, err = contexttop.FromFile(
			path.Join(demuxendpoint.runPath, dogstatsdContextsDumpFilename),
			request.NumMetrics,
			request.NumTags,
		)
	}
	if err != nil {
		httputils.SetJSONError(w, demuxendpoint.log.Errorf("Failed to get dogstatsd contexts top: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(topResponse{Result: result, Source: request.Source}); err != nil {
		demuxendpoint.log.Errorf("Failed to serialize dogstatsd contexts top response: %v", err)
	}
}

func validateTopRequest(request topRequest) error {
	if request.Source != topSourceLive && request.Source != topSourceDump {
		return fmt.Errorf("source must be %q or %q", topSourceLive, topSourceDump)
	}
	if request.NumMetrics < 1 || request.NumMetrics > maxNumMetrics {
		return fmt.Errorf("num_metrics must be between 1 and %d", maxNumMetrics)
	}
	if request.NumTags < 1 || request.NumTags > maxNumTags {
		return fmt.Errorf("num_tags must be between 1 and %d", maxNumTags)
	}
	return nil
}

func (demuxendpoint *demultiplexerEndpoint) getDogstatsdTop(numMetrics, numTags int) (contexttop.Result, error) {
	f, err := os.CreateTemp(demuxendpoint.runPath, "dogstatsd_contexts_top_*.json.zstd")
	if err != nil {
		return contexttop.Result{}, err
	}
	filePath := f.Name()
	defer os.Remove(filePath)

	if err := demuxendpoint.writeDogstatsdContextsFile(f); err != nil {
		return contexttop.Result{}, err
	}
	return contexttop.FromFile(filePath, numMetrics, numTags)
}

func (demuxendpoint *demultiplexerEndpoint) dumpDogstatsdContexts(w http.ResponseWriter, _ *http.Request) {
	if demuxendpoint.dogstatsdOnDataPlane {
		httputils.SetJSONError(w, errDogstatsdOnDataPlane, http.StatusServiceUnavailable)
		return
	}

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

func (demuxendpoint *demultiplexerEndpoint) writeDogstatsdContexts() (string, error) {
	demuxendpoint.dumpMu.Lock()
	defer demuxendpoint.dumpMu.Unlock()

	path := path.Join(demuxendpoint.runPath, dogstatsdContextsDumpFilename)

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	if err := demuxendpoint.writeDogstatsdContextsFile(f); err != nil {
		return "", err
	}
	return path, nil
}

func (demuxendpoint demultiplexerEndpoint) writeDogstatsdContextsFile(f *os.File) error {
	c := zstd.NewWriter(f)
	w := bufio.NewWriter(c)

	for _, err := range []error{demuxendpoint.demux.DumpDogstatsdContexts(w), w.Flush(), c.Close(), f.Close()} {
		if err != nil {
			return err
		}
	}
	return nil
}
