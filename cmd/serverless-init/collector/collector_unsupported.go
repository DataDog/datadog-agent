// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package collector provides enhanced metrics collection (Linux only)
// This file provides a stub implementation for non-Linux platforms
package collector

import (
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type EnhancedMetricSender interface{}

type Collector struct{}

func NewCollector(_ EnhancedMetricSender, _ metrics.MetricSource, _ string, _ string, _ time.Duration) (*Collector, error) {
	return nil, errors.New("Collector is only supported on Linux")
}

func (c *Collector) Start() {}

func (c *Collector) Stop() {}
