// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import "expvar"

type storeStats struct {
	External struct {
		Total int
		Valid int
	}
}

func getStoreStats() interface{} {
	stats := storeStats{}
	status := GetStatus()
	v, ok := status["External"]
	if !ok {
		return stats
	}
	externalStatus, ok := v.(map[string]interface{})
	if !ok {
		return stats
	}
	stats.External.Total, _ = externalStatus["Total"].(int)
	stats.External.Valid, _ = externalStatus["Valid"].(int)
	return stats
}

func init() {
	expvar.Publish("custommetrics", expvar.Func(getStoreStats))
}
