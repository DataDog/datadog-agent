// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package model

// ProcessJobIDsRequest is the payload of the system-probe GPU monitoring module's
// /process-job-ids endpoint, shared between the core agent (client) and
// system-probe (server).
type ProcessJobIDsRequest struct {
	// PID is the host PID of the process to get the training job identifiers of.
	PID int `json:"pid"`
}

// ProcessJobIDs holds the training job identifiers of a process that system-probe read
// from its environment, according to its gpu.jobs configuration. It is the response of the
// /process-job-ids endpoint. Those are the only values that leave system-probe: which
// environment variables are read is decided by its configuration, never by the caller.
type ProcessJobIDs struct {
	// Run is the ID of the training run. Empty if not configured or not set in the process.
	Run string `json:"run,omitempty"`
	// Group is the ID of the group of training runs. Empty if not configured or not set in the process.
	Group string `json:"group,omitempty"`
}
