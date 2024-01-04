// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package forwarder fetch information needed to render the 'forwarder' section of the status page.
package forwarder

import (
	"encoding/json"
	"expvar"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// PopulateStatus populates the status stats
func PopulateStatus(stats map[string]interface{}) {
	forwarderStatsJSON := []byte(expvar.Get("forwarder").String())
	forwarderStats := make(map[string]interface{})
	json.Unmarshal(forwarderStatsJSON, &forwarderStats) //nolint:errcheck
	forwarderStorageMaxSizeInBytes := config.Datadog.GetInt("forwarder_storage_max_size_in_bytes")
	if forwarderStorageMaxSizeInBytes > 0 {
		forwarderStats["forwarder_storage_max_size_in_bytes"] = strconv.Itoa(forwarderStorageMaxSizeInBytes)
	}
	stats["forwarderStats"] = forwarderStats
}
