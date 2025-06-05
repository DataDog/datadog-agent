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
func AddColdStartMetric(metricPrefix string, origin string, tags []string, _ time.Time, demux aggregator.Demultiplexer) {
	add(fmt.Sprintf("%v.enhanced.cold_start", metricPrefix), origin, tags, time.Now(), demux)
}

// AddShutdownMetric adds the shutdown metric to the demultiplexer
//
//nolint:revive // TODO(SERV) Fix revive linter
func AddShutdownMetric(metricPrefix string, origin string, tags []string, _ time.Time, demux aggregator.Demultiplexer) {
	add(fmt.Sprintf("%v.enhanced.shutdown", metricPrefix), origin, tags, time.Now(), demux)
}

func add(name string, origin string, tags []string, timestamp time.Time, demux aggregator.Demultiplexer) {
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
		Source:     originToMetricSource(origin),
	})
}

func originToMetricSource(origin string) metrics.MetricSource {
	switch origin {
	case cloudservice.CloudRunOrigin:
		return metrics.MetricSourceGoogleCloudRunEnhanced
	case cloudservice.AppServiceOrigin:
		return metrics.MetricSourceAzureAppServiceEnhanced
	case cloudservice.ContainerAppOrigin:
		return metrics.MetricSourceAzureContainerAppEnhanced
	default:
		return metrics.MetricSourceServerless
	}

}
