// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PlatformObjectRecord contains additional information found in Platform log messages
type PlatformObjectRecord struct {
	RequestID string           // uuid; present in LogTypePlatform{Start,End,Report}
	Version   string           // present in LogTypePlatformStart only
	Metrics   ReportLogMetrics // present in LogTypePlatformReport only
}

// ReportLogMetrics contains metrics found in a LogTypePlatformReport log
type ReportLogMetrics struct {
	DurationMs       float64
	BilledDurationMs int
	MemorySizeMB     int
	MaxMemoryUsedMB  int
	InitDurationMs   float64
}

// ServerlessMetricAgent represents the DogStatsD server and the aggregator
type ServerlessMetricAgent struct {
	DogStatDServer *dogstatsd.Server
	Aggregator     *aggregator.BufferedAggregator
}

// MetricConfig abstacts the config package
type MetricConfig struct {
}

// MetricDogStatsD abstracts the DogStatsD package
type MetricDogStatsD struct {
}

// MultipleEndpointConfig abstracts the config package
type MultipleEndpointConfig interface {
	GetMultipleEndpoints() (map[string][]string, error)
}

// DogStatsDFactory allows create a new DogStatsD server
type DogStatsDFactory interface {
	NewServer(aggregator *aggregator.BufferedAggregator, extraTags []string) (*dogstatsd.Server, error)
}

// GetMultipleEndpoints returns the api keys per domain specified in the main agent config
func (m *MetricConfig) GetMultipleEndpoints() (map[string][]string, error) {
	return config.GetMultipleEndpoints()
}

// NewServer returns a running DogStatsD server
func (m *MetricDogStatsD) NewServer(aggregator *aggregator.BufferedAggregator, extraTags []string) (*dogstatsd.Server, error) {
	return dogstatsd.NewServer(aggregator, extraTags)
}

// Start starts the DogStatsD agent
func (c *ServerlessMetricAgent) Start(forwarderTimeout time.Duration, multipleEndpointConfig MultipleEndpointConfig, dogstatFactory DogStatsDFactory) {
	// prevents any UDP packets from being stuck in the buffer and not parsed during the current invocation
	// by setting this option to 1ms, all packets received will directly be sent to the parser
	config.Datadog.Set("dogstatsd_packet_buffer_flush_timeout", 1*time.Millisecond)
	aggregatorInstance := buildBufferedAggregator(multipleEndpointConfig, forwarderTimeout)

	if aggregatorInstance != nil {
		statsd, err := dogstatFactory.NewServer(aggregatorInstance, nil)
		if err != nil {
			// we're not reporting the error to AWS because we don't want the function
			// execution to be stopped. TODO(remy): discuss with AWS if there is way
			// of reporting non-critical init errors.
			// serverless.ReportInitError(serverlessID, serverless.FatalDogstatsdInit)
			log.Errorf("Unable to start the DogStatsD server: %s", err)
		} else {
			statsd.ServerlessMode = true // we're running in a serverless environment (will removed host field from samples)
			c.DogStatDServer = statsd
			c.Aggregator = aggregatorInstance
		}
	}
}

func buildBufferedAggregator(multipleEndpointConfig MultipleEndpointConfig, forwarderTimeout time.Duration) *aggregator.BufferedAggregator {
	log.Debugf("Using a SyncForwarder with a %v timeout", forwarderTimeout)
	keysPerDomain, err := multipleEndpointConfig.GetMultipleEndpoints()
	if err != nil {
		// we're not reporting the error to AWS because we don't want the function
		// execution to be stopped. TODO(remy): discuss with AWS if there is way
		// of reporting non-critical init errors.
		log.Errorf("Misconfiguration of agent endpoints: %s", err)
		return nil
	}
	f := forwarder.NewSyncForwarder(keysPerDomain, forwarderTimeout)
	f.Start() //nolint:errcheck
	serializer := serializer.NewSerializer(f, nil)
	return aggregator.InitAggregator(serializer, nil, "serverless")
}
