// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:generate go run github.com/tinylib/msgp
//msgp:tag json

// Package tracermetadata parses the tracer-generated metadata
package tracermetadata

import (
	"iter"
	"strings"
)

// TracerMetadata as defined in
// https://github.com/DataDog/libdatadog/blob/0b59f64c4fc08105e5b73c5a0752ced3cf8f653e/datadog-library-config/src/tracer_metadata.rs#L7-L34
type TracerMetadata struct {
	SchemaVersion  uint8  `json:"schema_version"`
	RuntimeID      string `json:"runtime_id,omitempty"`
	TracerLanguage string `json:"tracer_language"`
	TracerVersion  string `json:"tracer_version"`
	Hostname       string `json:"hostname"`
	ServiceName    string `json:"service_name,omitempty"`
	ServiceEnv     string `json:"service_env,omitempty"`
	ServiceVersion string `json:"service_version,omitempty"`
	ProcessTags    string `json:"process_tags,omitempty"`
	ContainerID    string `json:"container_id,omitempty"`
	LogsCollected  bool   `json:"logs_collected,omitempty"`
}

// ShouldSkipServiceTagKV checks if a tracer service tag key-value pair should be
// skipped if it matches the UST tags.
func ShouldSkipServiceTagKV(tagKey, tagValue, ustService, ustEnv, ustVersion string) bool {
	if tagKey == "tracer_service_name" && tagValue == ustService {
		return true
	}
	if tagKey == "tracer_service_env" && tagValue == ustEnv {
		return true
	}
	if tagKey == "tracer_service_version" && tagValue == ustVersion {
		return true
	}
	return false
}

// ShouldSkipServiceTag checks if a tracer service tag should be skipped if it
// matches the UST tags.
func ShouldSkipServiceTag(tag string, ustService, ustEnv, ustVersion string) bool {
	if tagKey, tagValue, ok := strings.Cut(tag, ":"); ok {
		return ShouldSkipServiceTagKV(tagKey, tagValue, ustService, ustEnv, ustVersion)
	}
	return false
}

// Tags returns a sequence of tags from the tracer metadata
func (t TracerMetadata) Tags() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		if t.ServiceName != "" {
			if !yield("tracer_service_name", t.ServiceName) {
				return
			}
		}
		if t.ServiceEnv != "" {
			if !yield("tracer_service_env", t.ServiceEnv) {
				return
			}
		}
		if t.ServiceVersion != "" {
			if !yield("tracer_service_version", t.ServiceVersion) {
				return
			}
		}
		for tag := range strings.SplitSeq(t.ProcessTags, ",") {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}

			key, value, ok := strings.Cut(tag, ":")
			if !ok {
				continue
			}

			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if key == "" || value == "" {
				continue
			}

			if !yield(key, value) {
				return
			}
		}
	}
}

// GetTags returns a list of tags from the tracer metadata
func (t TracerMetadata) GetTags() []string {
	var tags []string
	for key, value := range t.Tags() {
		tags = append(tags, key+":"+value)
	}
	return tags
}
