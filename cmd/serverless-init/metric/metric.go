// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package metric

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// AddColdStartMetric adds the coldstart metric to the demultiplexer
//
//nolint:revive // TODO(SERV) Fix revive linter
func AddColdStartMetric(metricPrefix string, tags []string, _ time.Time, demux aggregator.Demultiplexer) {
	add(fmt.Sprintf("%v.enhanced.cold_start", metricPrefix), tags, time.Now(), metricPrefixToSource(metricPrefix), demux)
}

// AddShutdownMetric adds the shutdown metric to the demultiplexer
//
//nolint:revive // TODO(SERV) Fix revive linter
func AddShutdownMetric(metricPrefix string, tags []string, _ time.Time, demux aggregator.Demultiplexer) {
	add(fmt.Sprintf("%v.enhanced.shutdown", metricPrefix), tags, time.Now(), metricPrefixToSource(metricPrefix), demux)
}

func add(name string, tags []string, timestamp time.Time, metricSource metrics.MetricSource, demux aggregator.Demultiplexer) {
	if demux == nil {
		log.Debugf("Cannot add metric %s, the metric agent is not running", name)
		return
	}
	metricTimestamp := float64(timestamp.UnixNano()) / float64(time.Second)
	demux.AggregateSample(metrics.MetricSample{
		Name:       name,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  metricTimestamp,
		Source:     metricSource,
	})
}

// metricPrefixToSource returns the metric source for the respective cloud service's enhanced metrics
func metricPrefixToSource(metricPrefix string) metrics.MetricSource {
	var metricSource metrics.MetricSource
	switch metricPrefix {
	case cloudservice.ContainerAppMetricPrefix:
		metricSource = metrics.MetricSourceAzureContainerAppEnhanced
	case cloudservice.AppServiceMetricPrefix:
		metricSource = metrics.MetricSourceAzureAppServiceEnhanced
	case cloudservice.CloudRunMetricPrefix:
		metricSource = metrics.MetricSourceGoogleCloudRunEnhanced
	}
	return metricSource
}
