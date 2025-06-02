// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package sender

import "strings"

// GetMetricKey returns a metric key for the given metric and keys
func GetMetricKey(metric string, keys ...string) string {
	return metric + ":" + strings.Join(keys, ",")
}

// SetNewSentTimestamp is a util to set new timestamps
func SetNewSentTimestamp(newTimestamps map[string]float64, key string, ts float64) {
	lastTs := newTimestamps[key]
	if lastTs > ts {
		return
	}
	newTimestamps[key] = ts
}
