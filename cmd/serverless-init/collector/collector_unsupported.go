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

type Collector struct{}

func NewCollector(metricAgent *serverlessMetrics.ServerlessMetricAgent, metricSource metrics.MetricSource, metricPrefix string) (*Collector, error) {
	return nil, fmt.Errorf("CPU collector is only supported on Linux")
}

func (c *Collector) Start(ctx context.Context) {}

func (c *Collector) Stop() {}
