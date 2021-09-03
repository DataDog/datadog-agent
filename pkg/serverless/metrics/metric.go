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
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ServerlessMetricAgent represents the DogStatsD server and the aggregator
type ServerlessMetricAgent struct {
	dogStatDServer *dogstatsd.Server
	aggregator     *aggregator.BufferedAggregator
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
			log.Errorf("Unable to start the DogStatsD server: %s", err)
		} else {
			statsd.ServerlessMode = true // we're running in a serverless environment (will removed host field from samples)
			c.dogStatDServer = statsd
			c.aggregator = aggregatorInstance
		}
	}
}

// IsReady indicates whether or not the DogStatsD server is ready
func (c *ServerlessMetricAgent) IsReady() bool {
	return c.dogStatDServer != nil
}

// Flush triggers a DogStatsD flush
func (c *ServerlessMetricAgent) Flush() {
	if c.IsReady() {
		c.dogStatDServer.Flush()
	}
}

// Stop stops the DogStatsD server
func (c *ServerlessMetricAgent) Stop() {
	if c.IsReady() {
		c.dogStatDServer.Stop()
	}
}

// SetExtraTags sets extra tags on the DogStatsD server
func (c *ServerlessMetricAgent) SetExtraTags(tagArray []string) {
	if c.IsReady() {
		c.dogStatDServer.SetExtraTags(tagArray)
	}
}

// GetMetricChannel returns a channel where metrics can be sent to
func (c *ServerlessMetricAgent) GetMetricChannel() chan []metrics.MetricSample {
	return c.aggregator.GetBufferedMetricsWithTsChannel()
}

func buildBufferedAggregator(multipleEndpointConfig MultipleEndpointConfig, forwarderTimeout time.Duration) *aggregator.BufferedAggregator {
	log.Debugf("Using a SyncForwarder with a %v timeout", forwarderTimeout)
	keysPerDomain, err := multipleEndpointConfig.GetMultipleEndpoints()
	if err != nil {
		log.Errorf("Misconfiguration of agent endpoints: %s", err)
		return nil
	}
	f := forwarder.NewSyncForwarder(keysPerDomain, forwarderTimeout)
	f.Start() //nolint:errcheck
	serializer := serializer.NewSerializer(f, nil)

	// flushInterval is set to 0 (disabled) because we want to control flushing from the daemon
	return aggregator.InitAggregatorWithFlushInterval(serializer, nil, "serverless", 0)
}
