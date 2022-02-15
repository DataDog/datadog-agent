// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package response

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// ConfigCheckResponse holds the config check response
type ConfigCheckResponse struct {
	Configs         []integration.Config            `json:"configs"`
	ResolveWarnings map[string][]string             `json:"resolve_warnings"`
	ConfigErrors    map[string]string               `json:"config_errors"`
	Unresolved      map[string][]integration.Config `json:"unresolved"`
}

// TaggerListResponse holds the tagger list response
type TaggerListResponse struct {
	Entities map[string]TaggerListEntity `json:"entities"`
}

// TaggerListEntity holds the tagging info about an entity
type TaggerListEntity struct {
	Tags map[string][]string `json:"tags"`
}
