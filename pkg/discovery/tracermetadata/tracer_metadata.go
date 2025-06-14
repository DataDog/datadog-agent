// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:generate go run github.com/tinylib/msgp
//msgp:tag json

// Package tracermetadata parses the tracer-generated metadata
package tracermetadata

// TracerMetadata as defined in
// https://github.com/DataDog/libdatadog/blob/99056cf717cfe9/ddcommon/src/tracer_metadata.rs#L7-L29
type TracerMetadata struct {
	SchemaVersion  uint8  `json:"schema_version"`
	RuntimeID      string `json:"runtime_id,omitempty"`
	TracerLanguage string `json:"tracer_language"`
	TracerVersion  string `json:"tracer_version"`
	Hostname       string `json:"hostname"`
	ServiceName    string `json:"service_name,omitempty"`
	ServiceEnv     string `json:"service_env,omitempty"`
	ServiceVersion string `json:"service_version,omitempty"`
}
