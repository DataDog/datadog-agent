// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"os"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/initcontainer"
	serverlessInitLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/metric"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/tag"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/otlp"
	"github.com/DataDog/datadog-agent/pkg/serverless/random"
	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	tracelog "github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	datadogConfigPath = "/datadog.yaml"
	logLevelEnvVar    = "DD_LOG_LEVEL"
	loggerName        = "SERVERLESS_INIT"
)

type cliParams struct {
	args []string
}

func main() {
	if len(os.Args) < 2 {
		panic("[datadog init process] invalid argument count, did you forget to set CMD ?")
	}

	cliParams := &cliParams{
		args: os.Args[1:],
	}

	err := fxutil.OneShot(run, fx.Supply(cliParams))

		if err != nil {
			log.Error(err)
			os.Exit(-1)
		}
	}
}

func run(cliParams *cliParams) {
	cloudService, logConfig, traceAgent, metricAgent, logsAgent := setup()
	initcontainer.Run(cloudService, logConfig, metricAgent, traceAgent, logsAgent, cliParams.args)
}

func setup() (cloudservice.CloudService, *serverlessInitLog.Config, *trace.ServerlessTraceAgent, *metrics.ServerlessMetricAgent, logsAgent.ServerlessLogsAgent) {
	setupLogger()

	tracelog.SetLogger(corelogger{})

	// load proxy settings
	config.LoadProxyFromEnv(config.Datadog())

	cloudService := cloudservice.GetCloudServiceType()

	// Ignore errors for now. Once we go GA, check for errors
	// and exit right away.
	_ = cloudService.Init()

	tags := tags.MergeWithOverwrite(tags.ArrayToMap(configUtils.GetConfiguredTags(config.Datadog(), false)), cloudService.GetTags())
	origin := cloudService.GetOrigin()
	prefix := cloudService.GetPrefix()

	logConfig := serverlessInitLog.CreateConfig(origin)
	logsAgent := serverlessInitLog.SetupLog(logConfig, tags)

	// Disable remote configuration for now as it just spams the debug logs
	// and provides no value.
	os.Setenv("DD_REMOTE_CONFIGURATION_ENABLED", "false")

	// The datadog-agent requires Load to be called or it could
	// panic down the line.
	_, err := config.LoadWithoutSecret()
	if err != nil {
		log.Debugf("Error loading config: %v\n", err)
	}

	traceAgent := &trace.ServerlessTraceAgent{}
	go setupTraceAgent(traceAgent, tags)

	metricAgent := setupMetricAgent(tags)
	metric.AddColdStartMetric(prefix, metricAgent.GetExtraTags(), time.Now(), metricAgent.Demux)

	setupOtlpAgent(metricAgent)

	go flushMetricsAgent(metricAgent)
	return cloudService, logConfig, traceAgent, metricAgent, logsAgent
}

func setupLogger() {
	if err := config.SetupLogger(
		loggerName,
		"error", // will be re-set later with the value from the env var
		"",      // logFile -> by setting this to an empty string, we don't write the logs to any file
		"",      // syslog URI
		false,   // syslog_rfc
		true,    // log_to_console
		false,   // log_format_json
	); err != nil {
		log.Errorf("Unable to setup logger: %s", err)
	}

	if logLevel := os.Getenv(logLevelEnvVar); len(logLevel) > 0 {
		if err := config.ChangeLogLevel(logLevel); err != nil {
			log.Errorf("Unable to change the log level: %s", err)
		}
	}
}

func setupTraceAgent(traceAgent *trace.ServerlessTraceAgent, tags map[string]string) {
	traceAgent.Start(config.Datadog().GetBool("apm_config.enabled"), &trace.LoadConfig{Path: datadogConfigPath}, nil, random.Random.Uint64())
	traceAgent.SetTags(tag.GetBaseTagsMapWithMetadata(tags))
	for range time.Tick(3 * time.Second) {
		traceAgent.Flush()
	}
}

func setupMetricAgent(tags map[string]string) *metrics.ServerlessMetricAgent {
	config.Datadog().Set("use_v2_api.series", false, model.SourceAgentRuntime)
	metricAgent := &metrics.ServerlessMetricAgent{
		SketchesBucketOffset: time.Second * 0,
	}
	// we don't want to add the container_id tag to metrics for cardinality reasons
	tags = tag.WithoutContainerID(tags)
	tagArray := tag.GetBaseTagsArrayWithMetadataTags(tags)
	metricAgent.Start(5*time.Second, &metrics.MetricConfig{}, &metrics.MetricDogStatsD{})
	metricAgent.SetExtraTags(tagArray)
	return metricAgent
}

func setupOtlpAgent(metricAgent *metrics.ServerlessMetricAgent) {
	if !otlp.IsEnabled() {
		log.Debugf("otlp endpoint disabled")
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
