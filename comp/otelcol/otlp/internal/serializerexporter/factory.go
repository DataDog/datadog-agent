// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package serializerexporter

import (
	"context"

	"go.opentelemetry.io/collector/component"
	exp "go.opentelemetry.io/collector/exporter"

	"github.com/DataDog/datadog-agent/pkg/serializer"
)

const (
	// TypeStr defines the serializer exporter type string.
	TypeStr   = "serializer"
	stability = component.StabilityLevelStable
)

type factory struct {
	s serializer.MetricSerializer
}

// NewFactory creates a new serializer exporter factory.
func NewFactory(s serializer.MetricSerializer) exp.Factory {
	panic("not called")
}

func (f *factory) createMetricExporter(ctx context.Context, params exp.CreateSettings, c component.Config) (exp.Metrics, error) {
	panic("not called")
}
