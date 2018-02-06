// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package response

import "github.com/DataDog/datadog-agent/pkg/collector/check"

// ConfigCheckResponse holds the config check response
type ConfigCheckResponse struct {
	Configs         map[string][]check.Config `json:"configs"`
	ResolveWarnings map[string][]string       `json:"resolve_warnings"`
	ConfigErrors    map[string]string         `json:"config_errors"`
	Unresolved      map[string]check.Config   `json:"unresolved"`
}
