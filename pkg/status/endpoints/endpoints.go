// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package endpoints fetch information needed to render the 'endpoints' section of the status page.
package endpoints

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

// PopulateStatus populates the status stats
func PopulateStatus(stats map[string]interface{}) {
	endpoints, err := utils.GetMultipleEndpoints(config.Datadog)
	if err != nil {
		stats["endpointsInfos"] = nil
		return
	}

	endpointsInfos := make(map[string]interface{})

	// obfuscate the api keys
	for endpoint, keys := range endpoints {
		for i, key := range keys {
			if len(key) > 5 {
				keys[i] = key[len(key)-5:]
			}
		}
		endpointsInfos[endpoint] = keys
	}
	stats["endpointsInfos"] = endpointsInfos
}
