// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package common

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	apiKey string
)

// CachePrefix is the common root to use to prefix all the cache
// keys for any metadata value
const CachePrefix = "metadata"

// GetPayload fills and return the common metadata payload
func GetPayload(hostname string) *Payload {
	return &Payload{
		// olivier: I _think_ `APIKey` is only a legacy field, and
		// is not actually used by the backend
		AgentVersion:     version.AgentVersion,
		APIKey:           getAPIKey(),
		UUID:             getUUID(),
		InternalHostname: hostname,
	}
}

func getAPIKey() string {
	if apiKey == "" {
		apiKey = strings.Split(config.Datadog.GetString("api_key"), ",")[0]
	}

	return apiKey
}
