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
