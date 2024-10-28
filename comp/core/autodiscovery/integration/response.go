// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package integration

// ConfigCheckResponse holds the config check response
type ConfigCheckResponse struct {
	Configs         []Config            `json:"configs"`
	ResolveWarnings map[string][]string `json:"resolve_warnings"`
	ConfigErrors    map[string]string   `json:"config_errors"`
	Unresolved      map[string][]Config `json:"unresolved"`
}
