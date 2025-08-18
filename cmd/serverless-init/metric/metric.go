// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package metric

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

func Add(name string, value float64, source metrics.MetricSource, agent serverlessMetrics.ServerlessMetricAgent) {
	if agent.Demux == nil {
		log.Debugf("Cannot add metric %s, the metric agent is not running", name)
		return
	}
	metricTimestamp := float64(time.Now().UnixNano()) / float64(time.Second)
	agent.Demux.AggregateSample(metrics.MetricSample{
		Name:       name,
		Value:      value,
		Mtype:      metrics.DistributionType,
		Tags:       utilstrings.ToUnique(agent.GetExtraTags()),
		SampleRate: 1,
		Timestamp:  metricTimestamp,
		Source:     source,
	})
}
