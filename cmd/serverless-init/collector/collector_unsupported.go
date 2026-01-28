// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package collector

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
)

// CPUCollector is a stub for unsupported platforms
type CPUCollector struct{}

// NewCPUCollector returns an error on unsupported platforms
func NewCPUCollector(metricAgent *serverlessMetrics.ServerlessMetricAgent, metricSource metrics.MetricSource) (*CPUCollector, error) {
	return nil, fmt.Errorf("CPU collector is only supported on Linux")
}

// Start is a no-op on unsupported platforms
func (c *CPUCollector) Start(ctx context.Context) {}

// Stop is a no-op on unsupported platforms
func (c *CPUCollector) Stop() {}
