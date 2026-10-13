// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package model

// TrainingInfo holds the training job identifiers of a process using a GPU device.
type TrainingInfo struct {
	// PID is the host PID of the process using the GPU.
	PID uint32 `json:"pid"`
	// DeviceUUID is the UUID of the GPU device used by the process.
	DeviceUUID string `json:"device_uuid"`
	// TrainingRunID is the unique id of the training run, read from gpu.jobs.run.
	TrainingRunID string `json:"training_run_id,omitempty"`
	// TrainingGroupID is the id of the group of training runs, read from gpu.jobs.group.
	TrainingGroupID string `json:"training_group_id,omitempty"`
}
