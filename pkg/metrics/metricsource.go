// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

// MetricSource represents how this metric made it into the Agent
type MetricSource int

// Enumeration of the currently supported MetricSources
const (
	MetricSourceUnknown MetricSource = iota

	MetricSourceDogstatsd

	// In the future, metrics from official JMX integrations will
	// be properly categorized, but as things are today, ALL metrics
	// from a JMX check will be marked as "custom", including official
	// integrations
	MetricSourceJmxCustom
)

// String returns a string representation of MetricSource
func (ms MetricSource) String() string {
	switch ms {
	case MetricSourceDogstatsd:
		return "dogstatsd"
	case MetricSourceJmxCustom:
		return "jmx-custom-check"
	default:
		return "<unknown>"

	}
}

func (ms MetricSource) OriginCategory() int32 {
	// These constants map to specific fields in the 'OriginCategory' enum
	switch ms {
	case MetricSourceUnknown:
		return 0
	case MetricSourceDogstatsd:
		return 10
	case MetricSourceJmxCustom:
		return 11
	default:
		return 0
	}
}

func (ms MetricSource) OriginService() int32 {
	// These constants map to specific fields in the 'OriginService' enum
	switch ms {
	case MetricSourceDogstatsd:
		return 0
	case MetricSourceJmxCustom:
		return 9
	case MetricSourceUnknown:
		return 0
	default:
		return 0
	}
}
