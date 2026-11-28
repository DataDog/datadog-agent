// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metrics

import (
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
)

// consumeMetricMetadata passes the metadata of md to consumer if consumer is a
// [MetricMetadataConsumer] and md has a description.
// Datadog keys metadata by metric name, so md must be sent under its own name.
func consumeMetricMetadata(consumer Consumer, unitMapper *attributes.UnitMapper, md pmetric.Metric) {
	c, ok := consumer.(MetricMetadataConsumer)
	if !ok || md.Description() == "" {
		return
	}
	unit, _ := unitMapper.Map(md.Unit())
	c.ConsumeMetricMetadata(md.Name(), md.Description(), unit)
}
