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
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/stretchr/testify/assert"
)

func TestStartDoesNotBlock(t *testing.T) {
	metricAgent := &ServerlessMetricAgent{}
	metricAgent.Start(10*time.Second, &MetricConfig{}, &MetricDogStatsD{})
	assert.NotNil(t, metricAgent.Aggregator)
	assert.NotNil(t, metricAgent.DogStatDServer)
	assert.True(t, metricAgent.DogStatDServer.ServerlessMode)
}

type MetricConfigMocked struct {
}

func (m *MetricConfigMocked) GetMultipleEndpoints() (map[string][]string, error) {
	return nil, fmt.Errorf("error")
}

func TestStartInvalidConfig(t *testing.T) {
	metricAgent := &ServerlessMetricAgent{}
	go metricAgent.Start(1*time.Second, &MetricConfigMocked{}, &MetricDogStatsD{})
	assert.Nil(t, metricAgent.Aggregator)
	assert.Nil(t, metricAgent.DogStatDServer)
}

type MetricDogStatsDMocked struct {
}

func (m *MetricDogStatsDMocked) NewServer(aggregator *aggregator.BufferedAggregator, extraTags []string) (*dogstatsd.Server, error) {
	return nil, fmt.Errorf("error")
}

func TestStartInvalidDogStatsD(t *testing.T) {
	metricAgent := &ServerlessMetricAgent{}
	go metricAgent.Start(1*time.Second, &MetricConfig{}, &MetricDogStatsDMocked{})
	assert.Nil(t, metricAgent.Aggregator)
	assert.Nil(t, metricAgent.DogStatDServer)
}
