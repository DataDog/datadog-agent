// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/version"
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
		APIKey:           configUtils.SanitizeAPIKey(config.Datadog.GetString("api_key")),
		UUID:             getUUID(),
		InternalHostname: hostname,
	}
}
