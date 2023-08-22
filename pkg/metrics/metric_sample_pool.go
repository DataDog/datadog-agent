// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/metrics/model"
)

// MetricSampleBatch is a slice of MetricSample. It is used by the MetricSamplePool
// to avoid constant reallocation in high throughput pipelines.
//
// Can be used for both "on-time" and for "late" metrics.
type MetricSampleBatch = model.MetricSampleBatch

// MetricSamplePool is a pool of metrics sample
type MetricSamplePool = model.MetricSamplePool

// NewMetricSamplePool creates a new MetricSamplePool
var NewMetricSamplePool = func(batchSize int) MetricSamplePool {
	return *model.NewMetricSamplePool(batchSize, utils.IsTelemetryEnabled())
}
