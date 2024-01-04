// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package aggregator fetch information needed to render the 'aggregator' section of the status page.
package aggregator

import (
	"encoding/json"
	"expvar"

	checkstats "github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PopulateStatus populates the status stats
func PopulateStatus(stats map[string]interface{}) {
	aggregatorStatsJSON := []byte(expvar.Get("aggregator").String())
	aggregatorStats := make(map[string]interface{})
	json.Unmarshal(aggregatorStatsJSON, &aggregatorStats) //nolint:errcheck
	stats["aggregatorStats"] = aggregatorStats
	s, err := checkstats.TranslateEventPlatformEventTypes(stats["aggregatorStats"])
	if err != nil {
		log.Debugf("failed to translate event platform event types in aggregatorStats: %s", err.Error())
	} else {
		stats["aggregatorStats"] = s
	}
}
