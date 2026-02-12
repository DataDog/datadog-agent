// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package collector provides the enhanced CPU metrics collector for serverless init (Linux only).
// This file provides a stub implementation for non-Linux platforms.
package collector

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
)

type Collector struct{}

func NewCollector(_ *serverlessMetrics.ServerlessMetricAgent, _ metrics.MetricSource, _ string) (*Collector, error) {
	return nil, errors.New("CPU collector is only supported on Linux")
}

func (c *Collector) Start() {}

func (c *Collector) Stop() {}
