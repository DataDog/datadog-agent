// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func MetricSourceToOriginCategory(ms metrics.MetricSource) int32 {
	// These constants map to specific fields in the 'OriginCategory' enum in origin.proto
	switch ms {
	case metrics.MetricSourceUnknown:
		return 0
	case metrics.MetricSourceDogstatsd:
		return 10
	case metrics.MetricSourceJmxCustom:
		return 11
	default:
		return 0
	}
}

func MetricSourceToOriginService(ms metrics.MetricSource) int32 {
	// These constants map to specific fields in the 'OriginService' enum in origin.proto
	switch ms {
	case metrics.MetricSourceDogstatsd:
		return 0
	case metrics.MetricSourceJmxCustom:
		return 9
	case metrics.MetricSourceUnknown:
		return 0
	default:
		return 0
	}

}
