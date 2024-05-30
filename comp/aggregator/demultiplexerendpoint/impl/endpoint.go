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

	"github.com/DataDog/zstd"

	demultiplexerComp "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	apihelper "github.com/DataDog/datadog-agent/comp/api/api/helpers"
	"github.com/DataDog/datadog-agent/comp/api/api/utils"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
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
	Endpoint apihelper.AgentEndpointProvider
}

// NewComponent creates a new demultiplexerendpoint component
func NewComponent(reqs Requires) Provides {
	endpoint := demultiplexerEndpoint{
		demux:  reqs.Demultiplexer,
		config: reqs.Config,
		log:    reqs.Log,
	}

	return Provides{
		Endpoint: apihelper.NewAgentEndpointProvider(endpoint.dumpDogstatsdContexts, "/dogstatsd-contexts-dump", "POST"),
	}
}

func (demuxendpoint demultiplexerEndpoint) dumpDogstatsdContexts(w http.ResponseWriter, _ *http.Request) {
	path, err := demuxendpoint.writeDogstatsdContexts()
	if err != nil {
		utils.SetJSONError(w, demuxendpoint.log.Errorf("Failed to create dogstatsd contexts dump: %v", err), 500)
		return
	}

	resp, err := json.Marshal(path)
	if err != nil {
		utils.SetJSONError(w, demuxendpoint.log.Errorf("Failed to serialize response: %v", err), 500)
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
