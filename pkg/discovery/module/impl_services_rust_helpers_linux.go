// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Pure-Go helpers for the Rust-backed service discovery implementation.
// Kept in a separate file so that the tracermetadata package is referenced
// outside of the CGo translation unit, avoiding toolchain-specific CGo
// type-resolution failures.

//go:build linux_bpf

package module

import tracermetadata "github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata/model"

// buildTracerMetadata constructs a tracermetadata.TracerMetadata from its
// individual fields. All fields are plain Go values so this function can live
// in a non-CGo file, avoiding potential issues with CGo type resolution.
func buildTracerMetadata(
	schemaVersion uint8,
	runtimeID, tracerLanguage, tracerVersion, hostname,
	serviceName, serviceEnv, serviceVersion, processTags, containerID string,
	logsCollected bool,
) tracermetadata.TracerMetadata {
	return tracermetadata.TracerMetadata{
		SchemaVersion:  schemaVersion,
		RuntimeID:      runtimeID,
		TracerLanguage: tracerLanguage,
		TracerVersion:  tracerVersion,
		Hostname:       hostname,
		ServiceName:    serviceName,
		ServiceEnv:     serviceEnv,
		ServiceVersion: serviceVersion,
		ProcessTags:    processTags,
		ContainerID:    containerID,
		LogsCollected:  logsCollected,
	}
}
