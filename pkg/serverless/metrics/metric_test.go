// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/stretchr/testify/assert"
)

func TestStartDoesNotBlock(t *testing.T) {
	config.DetectFeatures()
	metricAgent := &ServerlessMetricAgent{}
	defer metricAgent.Stop()
	metricAgent.Start(10*time.Second, &MetricConfig{}, &MetricDogStatsD{})
	assert.NotNil(t, metricAgent.GetMetricChannel())
	assert.True(t, metricAgent.IsReady())
}

type MetricConfigMocked struct {
}

func (m *MetricConfigMocked) GetMultipleEndpoints() (map[string][]string, error) {
	return nil, fmt.Errorf("error")
}

func TestStartInvalidConfig(t *testing.T) {
	metricAgent := &ServerlessMetricAgent{}
	defer metricAgent.Stop()
	go metricAgent.Start(1*time.Second, &MetricConfigMocked{}, &MetricDogStatsD{})
	assert.False(t, metricAgent.IsReady())
}

type MetricDogStatsDMocked struct {
}

func (m *MetricDogStatsDMocked) NewServer(demux aggregator.Demultiplexer, extraTags []string) (*dogstatsd.Server, error) {
	return nil, fmt.Errorf("error")
}

func TestStartInvalidDogStatsD(t *testing.T) {
	metricAgent := &ServerlessMetricAgent{}
	defer metricAgent.Stop()
	go metricAgent.Start(1*time.Second, &MetricConfig{}, &MetricDogStatsDMocked{})
	assert.False(t, metricAgent.IsReady())
}
