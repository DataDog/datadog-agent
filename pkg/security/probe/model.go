// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package probe holds probe related files
package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	// ServiceEnvVar environment variable used to report service
	ServiceEnvVar = "DD_SERVICE"
	// WorkloadServiceEnvVar environment variable used to report service when DD_SERVICE is not defined
	WorkloadServiceEnvVar = "DD_WORKLOAD_SERVICE"
)

var workloadLabelsAsEnvVars = map[string]string{
	WorkloadServiceEnvVar:    "service",
	"DD_WORKLOAD_IMAGE_NAME": "image_name",
	"DD_WORKLOAD_IMAGE_TAG":  "image_tag",
	"DD_WORKLOAD_VERSION":    "version",
	"DD_WORKLOAD_ENV":        "env",
}

// NewEvent returns a new event
func NewEvent(fh *FieldHandlers) *model.Event {
	event := model.NewDefaultEvent()
	event.FieldHandlers = fh
	return event
}
