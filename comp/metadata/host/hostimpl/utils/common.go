// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// CommonPayload handles the JSON unmarshalling of the metadata payload
type CommonPayload struct {
	APIKey           string `json:"apiKey"`
	AgentVersion     string `json:"agentVersion"`
	UUID             string `json:"uuid"`
	InternalHostname string `json:"internalHostname"`
}

// GetCommonPayload fills and return the common metadata payload
func GetCommonPayload(hostname string, conf config.Reader) *CommonPayload {
	return &CommonPayload{
		// olivier: I _think_ `APIKey` is only a legacy field, and
		// is not actually used by the backend
		AgentVersion:     version.AgentVersion,
		APIKey:           configUtils.SanitizeAPIKey(conf.GetString("api_key")),
		UUID:             uuid.GetUUID(),
		InternalHostname: hostname,
	}
}
