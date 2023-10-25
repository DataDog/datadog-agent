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
	case metrics.MetricSourceJmxCustom,
		metrics.MetricSourceActivemq,
		metrics.MetricSourceCassandra,
		metrics.MetricSourceConfluentPlatform,
		metrics.MetricSourceHazelcast,
		metrics.MetricSourceHive,
		metrics.MetricSourceHivemq,
		metrics.MetricSourceHudi,
		metrics.MetricSourceIgnite,
		metrics.MetricSourceJbossWildfly,
		metrics.MetricSourceKafka,
		metrics.MetricSourcePresto,
		metrics.MetricSourceSolr,
		metrics.MetricSourceSonarqube,
		metrics.MetricSourceTomcat,
		metrics.MetricSourceWeblogic:
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
	case metrics.MetricSourceActivemq:
		return 12
	case metrics.MetricSourceCassandra:
		return 28
	case metrics.MetricSourceConfluentPlatform:
		return 40
	case metrics.MetricSourceHazelcast:
		return 70
	case metrics.MetricSourceHive:
		return 73
	case metrics.MetricSourceHivemq:
		return 74
	case metrics.MetricSourceHudi:
		return 76
	case metrics.MetricSourceIgnite:
		return 83
	case metrics.MetricSourceJbossWildfly:
		return 87
	case metrics.MetricSourceKafka:
		return 90
	case metrics.MetricSourcePresto:
		return 130
	case metrics.MetricSourceSolr:
		return 147
	case metrics.MetricSourceSonarqube:
		return 148
	case metrics.MetricSourceTomcat:
		return 163
	case metrics.MetricSourceWeblogic:
		return 172
	default:
		return 0
	}

}
