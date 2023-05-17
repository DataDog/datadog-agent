// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/initcontainer"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/metric"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/tag"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/otlp"
	"github.com/DataDog/datadog-agent/pkg/serverless/random"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	logger "github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	datadogConfigPath = "/var/task/datadog.yaml"
)

func main() {
	if len(os.Args) < 2 {
		panic("[datadog init process] invalid argument count, did you forget to set CMD ?")
	} else {
		cloudService, logConfig, traceAgent, metricAgent := setup()
		initcontainer.Run(cloudService, logConfig, metricAgent, traceAgent, os.Args[1:])
	}
}

func setup() (cloudservice.CloudService, *log.Config, *trace.ServerlessTraceAgent, *metrics.ServerlessMetricAgent) {
	// load proxy settings
	setupProxy()

	cloudService := cloudservice.GetCloudServiceType()
	tags := tags.MergeWithOverwrite(tags.ArrayToMap(config.GetGlobalConfiguredTags(false)), cloudService.GetTags())
	origin := cloudService.GetOrigin()
	prefix := cloudService.GetPrefix()

	logConfig := log.CreateConfig(origin)
	log.SetupLog(logConfig, tags)

	// The datadog-agent requires Load to be called or it could
	// panic down the line.
	_, err := config.Load()
	if err != nil {
		logger.Debugf("Error loading config: %v\n", err)
	}

	traceAgent := &trace.ServerlessTraceAgent{}
	go setupTraceAgent(traceAgent, tags)

	metricAgent := setupMetricAgent(tags)
	metric.AddColdStartMetric(prefix, metricAgent.GetExtraTags(), time.Now(), metricAgent.Demux)

	setupOtlpAgent(metricAgent)

	go flushMetricsAgent(metricAgent)
	return cloudService, logConfig, traceAgent, metricAgent
}

func setupTraceAgent(traceAgent *trace.ServerlessTraceAgent, tags map[string]string) {
	traceAgent.Start(config.Datadog.GetBool("apm_config.enabled"), &trace.LoadConfig{Path: datadogConfigPath}, nil, random.Random.Uint64())
	traceAgent.SetTags(tag.GetBaseTagsMapWithMetadata(tags))
	for range time.Tick(3 * time.Second) {
		traceAgent.Flush()
	}
}

func setupMetricAgent(tags map[string]string) *metrics.ServerlessMetricAgent {
	config.Datadog.Set("use_v2_api.series", false)
	metricAgent := &metrics.ServerlessMetricAgent{}
	// we don't want to add the container_id tag to metrics for cardinality reasons
	delete(tags, "container_id")
	tagArray := tag.GetBaseTagsArrayWithMetadataTags(tags)
	metricAgent.Start(5*time.Second, &metrics.MetricConfig{}, &metrics.MetricDogStatsD{})
	metricAgent.SetExtraTags(tagArray)
	return metricAgent
}

func setupOtlpAgent(metricAgent *metrics.ServerlessMetricAgent) {
	if !otlp.IsEnabled() {
		logger.Debugf("otlp endpoint disabled")
		return
	}
	otlpAgent := otlp.NewServerlessOTLPAgent(metricAgent.Demux.Serializer())
	otlpAgent.Start()
}

func flushMetricsAgent(metricAgent *metrics.ServerlessMetricAgent) {
	for range time.Tick(3 * time.Second) {
		metricAgent.Flush()
	}
}

func setupProxy() {
	config.LoadProxyFromEnv(config.Datadog)
}
