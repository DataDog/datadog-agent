package main

import (
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/initcontainer"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/metadata"
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
		panic("invalid arguments")
	}
	containerId := metadata.GetContainerId(metadata.GetDefaultConfig())
	logConfig := log.CreateConfig(containerId)
	log.SetupLog(logConfig)
	log.Write(logConfig, []byte(fmt.Sprintf("[datadog init process] starting, K_SERVICE = %s, K_REVISION = %s", os.Getenv("K_SERVICE"), os.Getenv("K_REVISION"))))

	traceAgent := &trace.ServerlessTraceAgent{}
	go setupTraceAgent(traceAgent)

	metricAgent := setupMetricAgent()
	metric.ColdStart(tag.GetBaseTags(), time.Now(), metricAgent.Demux)
	go metricAgent.Flush()
	initcontainer.Run(logConfig, metricAgent, traceAgent, os.Args[1:])
}

func setupTraceAgent(traceAgent *trace.ServerlessTraceAgent) {
	traceAgent.Start(config.Datadog.GetBool("apm_config.enabled"), &trace.LoadConfig{Path: datadogConfigPath})
	for range time.Tick(3 * time.Second) {
		traceAgent.Get().FlushSync()
	}
}

func setupMetricAgent() *metrics.ServerlessMetricAgent {
	metricAgent := &metrics.ServerlessMetricAgent{}
	metricAgent.Start(5*time.Second, &metrics.MetricConfig{}, &metrics.MetricDogStatsD{})
	return metricAgent
}
