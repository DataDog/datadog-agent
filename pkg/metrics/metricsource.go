// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import "github.com/DataDog/datadog-agent/pkg/metrics/model"

// MetricSource represents how this metric made it into the Agent
type MetricSource = model.MetricSource

// Enumeration of the currently supported MetricSources
const (
	MetricSourceUnknown = model.MetricSourceUnknown

	MetricSourceDogstatsd = model.MetricSourceDogstatsd

	// In the future, metrics from official JMX integrations will
	// be properly categorized, but as things are today, ALL metrics
	// from a JMX check will be marked as "custom", including official
	// integrations
	MetricSourceJmxCustom = model.MetricSourceJmxCustom
)
