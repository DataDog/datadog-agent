// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"context"
	"os"
	"sync"
	"time"

	serverlessInitLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/healthprobe"
	"github.com/DataDog/datadog-agent/comp/core/healthprobe/healthprobeimpl"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/serverless"

	"go.uber.org/atomic"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/metric"
	serverlessInitTag "github.com/DataDog/datadog-agent/cmd/serverless-init/tag"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/otlp"
	"github.com/DataDog/datadog-agent/pkg/serverless/random"
	serverlessTag "github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	tracelog "github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const datadogConfigPath = "datadog.yaml"

var modeConf mode.ModeConf

func main() {

	modeConf = mode.DetectMode()
	setEnvWithoutOverride(modeConf.EnvDefaults)

	err := fxutil.OneShot(
		run,
		autodiscoveryimpl.Module(),
		workloadmeta.Module(),
		fx.Provide(func(config coreconfig.Component) healthprobe.Options {
			return healthprobe.Options{
				Port:           config.GetInt("health_port"),
				LogsGoroutines: config.GetBool("log_all_goroutines_when_unhealthy"),
			}
		}),
		taggerimpl.Module(),
		healthprobeimpl.Module(),
		fx.Supply(workloadmeta.NewParams()),
		fx.Supply(tagger.NewTaggerParams()),
		fx.Supply(core.BundleParams{
			ConfigParams: coreconfig.NewParams("", coreconfig.WithConfigMissingOK(true)),
			SecretParams: secrets.NewEnabledParams(),
			LogParams:    logimpl.ForOneShot(modeConf.LoggerName, "off", true)}),
		core.Bundle(),
	)

	if err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}

// removing these unused dependencies will cause silent crash due to fx framework
func run(_ secrets.Component, _ autodiscovery.Component, _ healthprobe.Component) {
	cloudService, logConfig, traceAgent, metricAgent, logsAgent := setup(modeConf)

	modeConf.Runner(logConfig)

	metric.AddShutdownMetric(cloudService.GetPrefix(), metricAgent.GetExtraTags(), time.Now(), metricAgent.Demux)
	lastFlush(logConfig.FlushTimeout, metricAgent, traceAgent, logsAgent)
}

func setup(mode.ModeConf) (cloudservice.CloudService, *serverlessInitLog.Config, *trace.ServerlessTraceAgent, *metrics.ServerlessMetricAgent, logsAgent.ServerlessLogsAgent) {
	tracelog.SetLogger(corelogger{})

	// load proxy settings
	pkgconfig.LoadProxyFromEnv(pkgconfig.Datadog)

	cloudService := cloudservice.GetCloudServiceType()

	log.Debugf("Detected cloud service: %s", cloudService.GetOrigin())

	// Ignore errors for now. Once we go GA, check for errors
	// and exit right away.
	_ = cloudService.Init()

	tags := serverlessTag.MergeWithOverwrite(
		serverlessTag.ArrayToMap(configUtils.GetConfiguredTags(pkgconfig.Datadog, false)),
		cloudService.GetTags())
	origin := cloudService.GetOrigin()
	prefix := cloudService.GetPrefix()

	agentLogConfig := serverlessInitLog.CreateConfig(origin)

	// The datadog-agent requires Load to be called or it could
	// panic down the line.
	_, err := pkgconfig.LoadWithoutSecret()
	if err != nil {
		log.Debugf("Error loading config: %v\n", err)
	}
	logsAgent := serverlessInitLog.SetupLogAgent(agentLogConfig, tags)

	traceAgent := &trace.ServerlessTraceAgent{}
	go setupTraceAgent(traceAgent, tags)

	metricAgent := setupMetricAgent(tags)
	metric.AddColdStartMetric(prefix, metricAgent.GetExtraTags(), time.Now(), metricAgent.Demux)

	setupOtlpAgent(metricAgent)

	go flushMetricsAgent(metricAgent)
	return cloudService, agentLogConfig, traceAgent, metricAgent, logsAgent
}
func setupTraceAgent(traceAgent *trace.ServerlessTraceAgent, tags map[string]string) {
	traceAgent.Start(pkgconfig.Datadog.GetBool("apm_config.enabled"), &trace.LoadConfig{Path: datadogConfigPath}, nil, random.Random.Uint64())
	traceAgent.SetTags(serverlessInitTag.GetBaseTagsMapWithMetadata(tags))
	for range time.Tick(3 * time.Second) {
		traceAgent.Flush()
	}
}

func setupMetricAgent(tags map[string]string) *metrics.ServerlessMetricAgent {
	pkgconfig.Datadog.Set("use_v2_api.series", false, model.SourceAgentRuntime)
	metricAgent := &metrics.ServerlessMetricAgent{
		SketchesBucketOffset: time.Second * 0,
	}
	// we don't want to add the container_id tag to metrics for cardinality reasons
	tags = serverlessInitTag.WithoutContainerID(tags)
	tagArray := serverlessInitTag.GetBaseTagsMapWithMetadata(tags)
	metricAgent.Start(5*time.Second, &metrics.MetricConfig{}, &metrics.MetricDogStatsD{})
	metricAgent.SetExtraTags(serverlessTag.MapToArray(tagArray))
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

func lastFlush(flushTimeout time.Duration, metricAgent serverless.FlushableAgent, traceAgent serverless.FlushableAgent, logsAgent logsAgent.ServerlessLogsAgent) bool {
	hasTimeout := atomic.NewInt32(0)
	wg := &sync.WaitGroup{}
	wg.Add(3)
	go flushAndWait(flushTimeout, wg, metricAgent, hasTimeout)
	go flushAndWait(flushTimeout, wg, traceAgent, hasTimeout)
	childCtx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	go func(wg *sync.WaitGroup, ctx context.Context) {
		if logsAgent != nil {
			logsAgent.Flush(ctx)
		}
		wg.Done()
	}(wg, childCtx)
	wg.Wait()
	return hasTimeout.Load() > 0
}

func flushWithContext(ctx context.Context, timeoutchan chan struct{}, flushFunction func()) {
	flushFunction()
	select {
	case timeoutchan <- struct{}{}:
		log.Debug("finished flushing")
	case <-ctx.Done():
		log.Error("timed out while flushing")
		return
	}
}

func flushAndWait(flushTimeout time.Duration, wg *sync.WaitGroup, agent serverless.FlushableAgent, hasTimeout *atomic.Int32) {
	childCtx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	ch := make(chan struct{}, 1)
	go flushWithContext(childCtx, ch, agent.Flush)
	select {
	case <-childCtx.Done():
		hasTimeout.Inc()
		break
	case <-ch:
		break
	}
	wg.Done()
}

func setEnvWithoutOverride(envToSet map[string]string) {
	for envName, envVal := range envToSet {
		if val, set := os.LookupEnv(envName); !set {
			os.Setenv(envName, envVal)
		} else {
			log.Debugf("%s already set with %s, skipping setting it", envName, val)
		}
	}
}
