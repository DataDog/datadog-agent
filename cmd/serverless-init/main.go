// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package main

import (
	"os"
	"time"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/initcontainer"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/metric"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/tag"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
)

const (
	datadogConfigPath = "/var/task/datadog.yaml"
)

func main() {
	if len(os.Args) < 2 {
		panic("[datadog init process] invalid argument count, did you forget to set CMD ?")
	}

	cloudService := cloudservice.GetCloudServiceType()
	tags := cloudService.GetTags()
	origin := cloudService.GetOrigin()
	prefix := cloudService.GetPrefix()

	logConfig := log.CreateConfig(origin)
	log.SetupLog(logConfig, tags)

	traceAgent := &trace.ServerlessTraceAgent{}
	go setupTraceAgent(traceAgent, tags)

	metricAgent := setupMetricAgent(tags)
	metric.AddColdStartMetric(prefix, metricAgent.GetExtraTags(), time.Now(), metricAgent.Demux)

	go metricAgent.Flush()
	initcontainer.Run(cloudService, logConfig, metricAgent, traceAgent, os.Args[1:])
}

func setupTraceAgent(traceAgent *trace.ServerlessTraceAgent, tags map[string]string) {
	traceAgent.Start(config.Datadog.GetBool("apm_config.enabled"), &trace.LoadConfig{Path: datadogConfigPath}, nil)
	traceAgent.SetTags(tag.GetBaseTagsMapWithMetadata(tags))
	for range time.Tick(3 * time.Second) {
		traceAgent.Flush()
	}
}

func setupMetricAgent(tags map[string]string) *metrics.ServerlessMetricAgent {
	metricAgent := &metrics.ServerlessMetricAgent{}
	// we don't want to add the container_id tag to metrics for cardinality reasons
	delete(tags, "container_id")
	tagArray := tag.GetBaseTagsArrayWithMetadataTags(tags)
	metricAgent.Start(5*time.Second, &metrics.MetricConfig{}, &metrics.MetricDogStatsD{})
	metricAgent.SetExtraTags(tagArray)
	return metricAgent
}
