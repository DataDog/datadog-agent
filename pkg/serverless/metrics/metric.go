// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metrics provides the serverless metric agent for collecting and forwarding metrics.
package metrics

import (
	"time"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ServerlessMetricAgent represents the DogStatsD server and the aggregator
type ServerlessMetricAgent struct {
	dogStatsDServer       dogstatsdServer.ServerlessDogstatsd
	enhancedMetricTags    []string // does not include high cardinality tags
	enhancedMetricTagsAll []string // includes high cardinality tags
	tags                  []string
	Tagger                tagger.Component
	Demux                 aggregator.Demultiplexer

	SketchesBucketOffset time.Duration
}

// MetricConfig abstacts the config package
type MetricConfig struct {
}

// MetricDogStatsD abstracts the DogStatsD package
type MetricDogStatsD struct {
}

// MultipleEndpointConfig abstracts the config package
type MultipleEndpointConfig interface {
	GetMultipleEndpoints() (utils.EndpointDescriptorSet, error)
}

// DogStatsDFactory allows create a new DogStatsD server
type DogStatsDFactory interface {
	NewServer(demux aggregator.Demultiplexer, extraTags []string) (dogstatsdServer.ServerlessDogstatsd, error)
}

// GetMultipleEndpoints returns the api keys per domain specified in the main agent config
func (m *MetricConfig) GetMultipleEndpoints() (utils.EndpointDescriptorSet, error) {
	return utils.GetMultipleEndpoints(pkgconfigsetup.Datadog())
}

// NewServer returns a running DogStatsD server
func (m *MetricDogStatsD) NewServer(demux aggregator.Demultiplexer, extraTags []string) (dogstatsdServer.ServerlessDogstatsd, error) {
	return dogstatsdServer.NewServerlessServer(demux, extraTags)
}

// Start starts the DogStatsD agent
func (c *ServerlessMetricAgent) Start(forwarderTimeout time.Duration, multipleEndpointConfig MultipleEndpointConfig, dogstatFactory DogStatsDFactory, shouldForceFlushAllOnForceFlushToSerializer bool, extraTags []string, enhancedMetricTags []string, enhancedMetricTagsAll []string) {
	// prevents any UDP packets from being stuck in the buffer and not parsed
	// by setting this option to 1ms, all packets received will directly be sent to the parser
	pkgconfigsetup.Datadog().Set("dogstatsd_packet_buffer_flush_timeout", 1*time.Millisecond, model.SourceAgentRuntime)

	demux, err := buildDemultiplexer(multipleEndpointConfig, forwarderTimeout, c.Tagger, shouldForceFlushAllOnForceFlushToSerializer)
	if err != nil {
		log.Errorf("Unable to start the Demultiplexer: %s", err)
	}

	if demux != nil {
		statsd, err := dogstatFactory.NewServer(demux, extraTags)
		if err != nil {
			log.Errorf("Unable to start the DogStatsD server: %s", err)
		} else {
			c.dogStatsDServer = statsd
			c.Demux = demux
			c.tags = extraTags
			c.enhancedMetricTags = enhancedMetricTags
			c.enhancedMetricTagsAll = enhancedMetricTagsAll
		}
	}
}

// IsReady indicates whether or not the DogStatsD server is ready
func (c *ServerlessMetricAgent) IsReady() bool {
	return c.dogStatsDServer != nil
}

// Flush triggers a DogStatsD flush
func (c *ServerlessMetricAgent) Flush() {
	if c.IsReady() {
		c.dogStatsDServer.ServerlessFlush(c.SketchesBucketOffset)
	}
}

// Stop stops the DogStatsD server
func (c *ServerlessMetricAgent) Stop() {
	if c.IsReady() {
		c.dogStatsDServer.Stop()
	}
}

// AddLegacyEnhancedMetric reports a metric value to the intake with all tags.
func (c *ServerlessMetricAgent) AddLegacyEnhancedMetric(name string, value float64, metricSource metrics.MetricSource, extraTags ...string) {
	c.sendMetricSample(name, value, metricSource, 0, c.tags, extraTags...)
}

// AddEnhancedMetric reports a metric value to the intake with the given timestamp and tags selected for enhanced metrics.
func (c *ServerlessMetricAgent) AddEnhancedMetric(name string, value float64, metricSource metrics.MetricSource, timestamp float64, extraTags ...string) {
	c.sendMetricSample(name, value, metricSource, timestamp, c.enhancedMetricTags, extraTags...)
}

// AddHighCardinalityEnhancedMetric reports a metric value to the intake with the given timestamp and tags selected for enhanced metrics, including high cardinality tags.
func (c *ServerlessMetricAgent) AddHighCardinalityEnhancedMetric(name string, value float64, metricSource metrics.MetricSource, timestamp float64, extraTags ...string) {
	c.sendMetricSample(name, value, metricSource, timestamp, c.enhancedMetricTagsAll, extraTags...)
}

// Add records a distribution metric sample using the agent's extra tags plus any
// optional tags supplied as `key:value` strings through extraTags.
func (c *ServerlessMetricAgent) sendMetricSample(name string, value float64, metricSource metrics.MetricSource, timestamp float64, tags []string, extraTags ...string) {
	if c.Demux == nil {
		log.Debugf("Cannot add metric %s, the metric agent is not running", name)
		return
	}

	if timestamp == 0 {
		timestamp = float64(time.Now().UnixNano()) / float64(time.Second)
	}

	if len(extraTags) > 0 {
		tags = append(append([]string{}, tags...), extraTags...)
	}
	c.Demux.AggregateSample(metrics.MetricSample{
		Name:       name,
		Value:      value,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  timestamp,
		Source:     metricSource,
	})
}

func buildDemultiplexer(multipleEndpointConfig MultipleEndpointConfig, forwarderTimeout time.Duration, tagger tagger.Component, shouldForceFlushAllOnForceFlushToSerializer bool) (aggregator.Demultiplexer, error) {
	log.Debugf("Using a SyncForwarder with a %v timeout", forwarderTimeout)
	keysPerDomain, err := multipleEndpointConfig.GetMultipleEndpoints()
	if err != nil {
		log.Errorf("Misconfiguration of agent endpoints: %s", err)
		return nil, err
	}
	return aggregator.InitAndStartServerlessDemultiplexer(keysPerDomain, forwarderTimeout, tagger, shouldForceFlushAllOnForceFlushToSerializer)
}
