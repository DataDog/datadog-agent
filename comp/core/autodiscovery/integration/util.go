// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package integration defines types representing an integration configuration,
// which can be used by several components of the agent to configure checks or
// log collectors, for example.
package integration

import "strings"

// ConfigSourceToMetadataMap converts a config source string to a metadata map.
func ConfigSourceToMetadataMap(source string, instance map[string]interface{}) {
	if instance == nil {
		instance = make(map[string]interface{})
	}
	splitSource := strings.SplitN(source, ":", 2)
	instance["config.provider"] = splitSource[0]
	if len(splitSource) > 1 {
		instance["config.source"] = splitSource[1]
	} else {
		instance["config.source"] = "unknown"
	}
}
