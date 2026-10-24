// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package demultiplexerendpointimpl component provides the /dogstatsd-contexts-dump API endpoints that can register via Fx value groups.
package demultiplexerendpointimpl

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/sync/singleflight"

	demultiplexerComp "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	dogstatsdconfig "github.com/DataDog/datadog-agent/comp/dogstatsd/config"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/zstd"
)

const (
	dogstatsdContextsDumpFilename = "dogstatsd_contexts.json.zstd"
)

var (
	errDogstatsdOnDataPlane        = errors.New("DogStatsD traffic is being served by the Agent Data Plane; run DogStatsD diagnostic commands against the agent-data-plane process instead")
	errDogstatsdDumpNotFound       = errors.New("DogStatsD contexts dump has not been created")
	errDogstatsdDumpNotRegularFile = errors.New("DogStatsD contexts dump is not a regular file")
)

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
	dumpGroup            singleflight.Group
}

// Provides defines the output of the demultiplexerendpoint component
type Provides struct {
	DumpEndpoint     api.AgentEndpointProvider
	DumpInfoEndpoint api.AgentEndpointProvider
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
		DumpEndpoint:     api.NewAgentEndpointProvider(endpoint.dumpDogstatsdContexts, "/dogstatsd-contexts-dump", "POST"),
		DumpInfoEndpoint: api.NewAgentEndpointProvider(endpoint.getDogstatsdContextsDump, "/dogstatsd-contexts-dump", "GET"),
	}
}

type dogstatsdContextsDumpInfo struct {
	Path               string `json:"path"`
	Size               int64  `json:"size"`
	ModifiedAtUnixNano int64  `json:"modified_at_unix_nano"`
}

func (demuxendpoint *demultiplexerEndpoint) getDogstatsdContextsDump(w http.ResponseWriter, _ *http.Request) {
	if demuxendpoint.dogstatsdOnDataPlane {
		httputils.SetJSONError(w, errDogstatsdOnDataPlane, http.StatusNotFound)
		return
	}

	path := filepath.Join(demuxendpoint.runPath, dogstatsdContextsDumpFilename)
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		httputils.SetJSONError(w, errDogstatsdDumpNotFound, http.StatusNotFound)
		return
	}
	if err != nil {
		httputils.SetJSONError(w, demuxendpoint.log.Errorf("Failed to inspect dogstatsd contexts dump: %v", err), http.StatusInternalServerError)
		return
	}
	if !info.Mode().IsRegular() {
		httputils.SetJSONError(w, errDogstatsdDumpNotRegularFile, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(dogstatsdContextsDumpInfo{
		Path:               path,
		Size:               info.Size(),
		ModifiedAtUnixNano: info.ModTime().UnixNano(),
	}); err != nil {
		demuxendpoint.log.Errorf("Failed to serialize dogstatsd contexts dump information: %v", err)
	}
}

func (demuxendpoint *demultiplexerEndpoint) dumpDogstatsdContexts(w http.ResponseWriter, _ *http.Request) {
	if demuxendpoint.dogstatsdOnDataPlane {
		httputils.SetJSONError(w, errDogstatsdOnDataPlane, http.StatusNotFound)
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
	finalPath := filepath.Join(demuxendpoint.runPath, dogstatsdContextsDumpFilename)

	result, err, _ := demuxendpoint.dumpGroup.Do(finalPath, func() (any, error) {
		return demuxendpoint.writeDogstatsdContextsFile(finalPath)
	})
	if err != nil {
		return "", err
	}

	return result.(string), nil
}

func (demuxendpoint *demultiplexerEndpoint) writeDogstatsdContextsFile(finalPath string) (string, error) {
	f, err := os.CreateTemp(demuxendpoint.runPath, ".dogstatsd_contexts-*.tmp")
	if err != nil {
		return "", err
	}
	tempPath := f.Name()
	defer os.Remove(tempPath)

	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(finalPath); statErr == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		_ = f.Close()
		return "", statErr
	}
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return "", err
	}

	if err := demuxendpoint.writeDogstatsdContextsToFile(f); err != nil {
		return "", err
	}

	if err := os.Rename(tempPath, finalPath); err != nil {
		return "", err
	}

	return finalPath, nil
}

func (demuxendpoint *demultiplexerEndpoint) writeDogstatsdContextsToFile(f *os.File) error {
	c, err := zstd.NewWriter(f)
	if err != nil {
		_ = f.Close()
		return err
	}
	w := bufio.NewWriter(c)

	for _, err := range []error{demuxendpoint.demux.DumpDogstatsdContexts(w), w.Flush(), c.Close(), f.Close()} {
		if err != nil {
			return err
		}
	}
	return nil
}
