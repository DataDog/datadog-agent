// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package jobids serves the training job identifiers of host processes over the
// system-probe HTTP API. system-probe has the privileges and the host procfs
// access that the core agent may lack.
package jobids

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Handler serves the run and group IDs of a process, read from the environment
// variables configured in gpu.jobs. It never returns anything else from the
// process environment, which often contains secrets: the caller only provides a
// PID, the environment variables to read come from the configuration.
type Handler struct {
	jobs     gpuconfig.JobsConfig
	procRoot string
}

// NewHandler creates a handler that reads the environment variables configured in jobs
// from the procfs at procRoot.
func NewHandler(jobs gpuconfig.JobsConfig, procRoot string) *Handler {
	return &Handler{jobs: jobs, procRoot: procRoot}
}

// ServeHTTP handles a model.ProcessJobIDsRequest and answers with the model.ProcessJobIDs of the process.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req model.ProcessJobIDsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.PID <= 0 {
		http.Error(w, "invalid pid", http.StatusBadRequest)
		return
	}

	ids, err := h.readJobIDs(req.PID)
	if err != nil {
		log.Debugf("error reading job identifiers of process %d: %v", req.PID, err)
		http.Error(w, "could not read the job identifiers of the process", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(ids); err != nil {
		log.Debugf("error writing job identifiers response: %v", err)
	}
}

// readJobIDs reads the identifiers configured as environment variables. A process
// that does not exist anymore has no environment, which is not an error.
func (h *Handler) readJobIDs(pid int) (model.ProcessJobIDs, error) {
	run, err := h.readID(pid, h.jobs.Run)
	if errors.Is(err, fs.ErrNotExist) {
		return model.ProcessJobIDs{}, nil
	}
	if err != nil {
		return model.ProcessJobIDs{}, err
	}

	group, err := h.readID(pid, h.jobs.Group)
	if errors.Is(err, fs.ErrNotExist) {
		return model.ProcessJobIDs{}, nil
	}
	if err != nil {
		return model.ProcessJobIDs{}, err
	}

	return model.ProcessJobIDs{Run: run, Group: group}, nil
}

// readID returns the value of the environment variable id points to, or an empty
// string if id is not an environment variable identifier.
func (h *Handler) readID(pid int, id gpuconfig.IdentifierConfig) (string, error) {
	if !id.Configured() || id.Type != gpuconfig.IdentifierTypeEnv {
		return "", nil
	}

	value, err := kernel.GetProcessEnvVariable(pid, h.procRoot, id.Key)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", id.Key, err)
	}
	return value, nil
}
