// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
	"math/rand"
	"net/http"
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
	// allow some time to stop to avoid 'can't listen: listen udp 127.0.0.1:8125: bind: address already in use'
	time.Sleep(100 * time.Millisecond)
}

type ValidMetricConfigMocked struct {
}

func (m *ValidMetricConfigMocked) GetMultipleEndpoints() (map[string][]string, error) {
	return map[string][]string{"http://localhost:8888": {"value"}}, nil
}

type InvalidMetricConfigMocked struct {
}

func (m *InvalidMetricConfigMocked) GetMultipleEndpoints() (map[string][]string, error) {
	return nil, fmt.Errorf("error")
}

func TestStartInvalidConfig(t *testing.T) {
	metricAgent := &ServerlessMetricAgent{}
	defer metricAgent.Stop()
	metricAgent.Start(1*time.Second, &InvalidMetricConfigMocked{}, &MetricDogStatsD{})
	assert.False(t, metricAgent.IsReady())
	// allow some time to stop to avoid 'can't listen: listen udp 127.0.0.1:8125: bind: address already in use'
	time.Sleep(100 * time.Millisecond)
}

type MetricDogStatsDMocked struct {
}

func (m *MetricDogStatsDMocked) NewServer(aggregator *aggregator.BufferedAggregator, extraTags []string) (*dogstatsd.Server, error) {
	return nil, fmt.Errorf("error")
}

func TestStartInvalidDogStatsD(t *testing.T) {
	metricAgent := &ServerlessMetricAgent{}
	defer metricAgent.Stop()
	metricAgent.Start(1*time.Second, &MetricConfig{}, &MetricDogStatsDMocked{})
	assert.False(t, metricAgent.IsReady())
	// allow some time to stop to avoid 'can't listen: listen udp 127.0.0.1:8125: bind: address already in use'
	time.Sleep(1 * time.Second)
}

func TestRaceFlushVersusAddSample(t *testing.T) {

	config.DetectFeatures()

	metricAgent := &ServerlessMetricAgent{}
	defer metricAgent.Stop()
	metricAgent.Start(10*time.Second, &ValidMetricConfigMocked{}, &MetricDogStatsD{})

	assert.NotNil(t, metricAgent.GetMetricChannel())

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(10 * time.Millisecond)
		})

		err := http.ListenAndServe("localhost:8888", nil)
		if err != nil {
			panic(err)
		}
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			n := rand.Intn(10)
			time.Sleep(time.Duration(n) * time.Microsecond)
			go SendTimeoutEnhancedMetric([]string{"tag0:value0", "tag1:value1"}, metricAgent.GetMetricChannel())
		}
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			n := rand.Intn(10)
			time.Sleep(time.Duration(n) * time.Microsecond)
			go metricAgent.Flush()
		}
	}()

	time.Sleep(2 * time.Second)
}

func TestBuildMetricBlocklist(t *testing.T) {
	userProvidedBlocklist := []string{
		"user.defined.a",
		"user.defined.b",
	}
	expected := []string{
		"user.defined.a",
		"user.defined.b",
		invocationsMetric,
	}
	result := buildMetricBlocklist(userProvidedBlocklist)
	assert.Equal(t, expected, result)
}
