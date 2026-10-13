// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package traininginfo resolves the training job identifiers of the processes using GPUs.
package traininginfo

import (
	"net/http"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	sputils "github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// builtinAllowedEnvVars lists the environment variables that may always be read from processes using GPUs.
// System-probe runs as root and can read the environment of every process on the host, while the jobs config comes
// from datadog.yaml, which the unprivileged dd-agent user can modify. Reads are restricted to known training job
// identifiers, plus those allowed in the root-owned system-probe.yaml, so the jobs config cannot be used to read
// arbitrary environment variables (such as secrets) that would then be sent to Datadog as tags.
var builtinAllowedEnvVars = []string{
	"_RAY_SUBMISSION_ID",
	"RAY_CLUSTER_NAME",
	"MLFLOW_RUN_ID",
	"MLFLOW_EXPERIMENT_ID",
}

// GPUProcess is a process using a GPU device.
type GPUProcess struct {
	// PID is the host PID of the process.
	PID uint32
	// DeviceUUID is the UUID of the GPU device used by the process.
	DeviceUUID string
}

// ListGPUProcessesFunc returns the processes using GPU devices. It may return partial results along with an error.
type ListGPUProcessesFunc func() ([]GPUProcess, error)

// Handler serves the training job identifiers of the processes using GPUs over the system-probe HTTP API.
type Handler struct {
	procRoot         string
	runEnvVar        string
	groupEnvVar      string
	listGPUProcesses ListGPUProcessesFunc
}

// NewHandler creates a handler reading the env identifiers of jobs from the environment of GPU processes in procRoot.
// extraAllowedEnvVars extends the built-in allowlist. Identifiers that are not of type env, or whose environment
// variable is not allowed, are ignored.
func NewHandler(jobs gpuconfig.JobsConfig, extraAllowedEnvVars []string, procRoot string, listGPUProcesses ListGPUProcessesFunc) *Handler {
	allowed := slices.Concat(builtinAllowedEnvVars, extraAllowedEnvVars)
	return &Handler{
		procRoot:         procRoot,
		runEnvVar:        allowedEnvVar("run", jobs.Run, allowed),
		groupEnvVar:      allowedEnvVar("group", jobs.Group, allowed),
		listGPUProcesses: listGPUProcesses,
	}
}

// allowedEnvVar returns the environment variable holding the identifier, or an empty string if the identifier is not
// read from an allowed environment variable.
func allowedEnvVar(name string, id gpuconfig.IdentifierConfig, allowed []string) string {
	if !id.IsEnv() {
		return ""
	}
	if !slices.Contains(allowed, id.Key) {
		log.Warnf("gpu.jobs.%s.key %q is not an allowed environment variable, it will be ignored. Add it to gpu_monitoring.training_info.allowed_env_vars in system-probe.yaml to allow it", name, id.Key)
		return ""
	}
	return id.Key
}

// Enabled returns true if any training job identifier is read from an allowed environment variable.
func (h *Handler) Enabled() bool {
	return h.runEnvVar != "" || h.groupEnvVar != ""
}

// Collect returns the training job identifiers of every GPU process that has at least one of them set.
func (h *Handler) Collect() []model.TrainingInfo {
	infos := []model.TrainingInfo{}
	if !h.Enabled() {
		return infos
	}

	processes, err := h.listGPUProcesses()
	if err != nil {
		log.Debugf("error listing GPU processes, training info may be incomplete: %v", err)
	}

	for _, process := range processes {
		info := model.TrainingInfo{
			PID:             process.PID,
			DeviceUUID:      process.DeviceUUID,
			TrainingRunID:   h.readEnv(process.PID, h.runEnvVar),
			TrainingGroupID: h.readEnv(process.PID, h.groupEnvVar),
		}
		if info.TrainingRunID != "" || info.TrainingGroupID != "" {
			infos = append(infos, info)
		}
	}

	return infos
}

// readEnv returns the value of envVar for the process, or an empty string if it cannot be read.
func (h *Handler) readEnv(pid uint32, envVar string) string {
	if envVar == "" {
		return ""
	}
	value, err := kernel.GetProcessEnvVariable(int(pid), h.procRoot, envVar)
	if err != nil {
		// The process may have exited since it was listed
		log.Debugf("error reading %s from process %d: %v", envVar, pid, err)
		return ""
	}
	return value
}

// HandleTrainingInfo returns the training job identifiers of the processes using GPUs.
func (h *Handler) HandleTrainingInfo(w http.ResponseWriter, req *http.Request) {
	sputils.WriteAsJSON(req, w, h.Collect(), sputils.CompactOutput)
}
