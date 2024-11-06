// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	serverlessInitLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	healthprobeDef "github.com/DataDog/datadog-agent/comp/core/healthprobe/def"
	healthprobeFx "github.com/DataDog/datadog-agent/comp/core/healthprobe/fx"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/pkg/serverless"

	"go.uber.org/atomic"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/metric"
	serverlessInitTag "github.com/DataDog/datadog-agent/cmd/serverless-init/tag"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/otlp"
	"github.com/DataDog/datadog-agent/pkg/serverless/random"
	serverlessTag "github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	tracelog "github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const datadogConfigPath = "datadog.yaml"

var modeConf mode.Conf

func main() {

	modeConf = mode.DetectMode()
	setEnvWithoutOverride(modeConf.EnvDefaults)

	err := fxutil.OneShot(
		run,
		autodiscoveryimpl.Module(),
		fx.Provide(func(config coreconfig.Component) healthprobeDef.Options {
			return healthprobeDef.Options{
				Port:           config.GetInt("health_port"),
				LogsGoroutines: config.GetBool("log_all_goroutines_when_unhealthy"),
			}
		}),
		taggerimpl.Module(),
		healthprobeFx.Module(),
		workloadmetafx.Module(workloadmeta.NewParams()),
		fx.Supply(tagger.NewTaggerParams()),
		fx.Supply(coreconfig.NewParams("", coreconfig.WithConfigMissingOK(true))),
		coreconfig.Module(),
		fx.Supply(secrets.NewEnabledParams()),
		secretsimpl.Module(),
		fx.Provide(func(secrets secrets.Component) optional.Option[secrets.Component] { return optional.NewOption(secrets) }),
		fx.Supply(logdef.ForOneShot(modeConf.LoggerName, "off", true)),
		logfx.Module(),
		nooptelemetry.Module(),
	)

	if err != nil {
		log.Error(err)
		exitCode := errorExitCode(err)
		log.Flush()
		os.Exit(exitCode)
	}
}

// removing these unused dependencies will cause silent crash due to fx framework
func run(_ secrets.Component, _ autodiscovery.Component, _ healthprobeDef.Component, tagger tagger.Component) error {
	cloudService, logConfig, traceAgent, metricAgent, logsAgent := setup(modeConf, tagger)

	err := modeConf.Runner(logConfig)

	metric.AddShutdownMetric(cloudService.GetPrefix(), metricAgent.GetExtraTags(), time.Now(), metricAgent.Demux)
	lastFlush(logConfig.FlushTimeout, metricAgent, traceAgent, logsAgent)

	return err
}

func setup(_ mode.Conf, tagger tagger.Component) (cloudservice.CloudService, *serverlessInitLog.Config, trace.ServerlessTraceAgent, *metrics.ServerlessMetricAgent, logsAgent.ServerlessLogsAgent) {
	tracelog.SetLogger(corelogger{})

	// load proxy settings
	pkgconfigsetup.LoadProxyFromEnv(pkgconfigsetup.Datadog())

	cloudService := cloudservice.GetCloudServiceType()

	log.Debugf("Detected cloud service: %s", cloudService.GetOrigin())

	// Ignore errors for now. Once we go GA, check for errors
	// and exit right away.
	_ = cloudService.Init()

	tags := serverlessInitTag.GetBaseTagsMapWithMetadata(
		serverlessTag.MergeWithOverwrite(
			serverlessTag.ArrayToMap(configUtils.GetConfiguredTags(pkgconfigsetup.Datadog(), false)),
			cloudService.GetTags()),
		modeConf.TagVersionMode)

	origin := cloudService.GetOrigin()
	prefix := cloudService.GetPrefix()

	agentLogConfig := serverlessInitLog.CreateConfig(origin)

	// The datadog-agent requires Load to be called or it could
	// panic down the line.
	_, err := pkgconfigsetup.LoadWithoutSecret(pkgconfigsetup.Datadog(), nil)
	if err != nil {
		log.Debugf("Error loading config: %v\n", err)
	}
	logsAgent := serverlessInitLog.SetupLogAgent(agentLogConfig, tags, tagger)

	traceAgent := setupTraceAgent(tags, tagger)

	metricAgent := setupMetricAgent(tags, tagger)
	metric.AddColdStartMetric(prefix, metricAgent.GetExtraTags(), time.Now(), metricAgent.Demux)

	setupOtlpAgent(metricAgent)

	go flushMetricsAgent(metricAgent)
	return cloudService, agentLogConfig, traceAgent, metricAgent, logsAgent
}

var azureContainerAppTags = []string{
	"subscription_id",
	"resource_group",
	"resource_id",
	"replicate_name",
	"aca.subscription.id",
	"aca.resource.group",
	"aca.resource.id",
	"aca.replica.name",
}

func setupTraceAgent(tags map[string]string, tagger tagger.Component) trace.ServerlessTraceAgent {
	var azureTags strings.Builder
	for _, azureContainerAppTag := range azureContainerAppTags {
		if value, ok := tags[azureContainerAppTag]; ok {
			azureTags.WriteString(fmt.Sprintf(",%s:%s", azureContainerAppTag, value))
		}
	}
	traceAgent := trace.StartServerlessTraceAgent(trace.StartServerlessTraceAgentArgs{
		Enabled:               pkgconfigsetup.Datadog().GetBool("apm_config.enabled"),
		LoadConfig:            &trace.LoadConfig{Path: datadogConfigPath, Tagger: tagger},
		ColdStartSpanID:       random.Random.Uint64(),
		AzureContainerAppTags: azureTags.String(),
	})
	go func() {
		for range time.Tick(3 * time.Second) {
			traceAgent.Flush()
		}
	}()
	return traceAgent
}

func setupMetricAgent(tags map[string]string, tagger tagger.Component) *metrics.ServerlessMetricAgent {
	pkgconfigsetup.Datadog().Set("use_v2_api.series", false, model.SourceAgentRuntime)
	pkgconfigsetup.Datadog().Set("dogstatsd_socket", "", model.SourceAgentRuntime)

	metricAgent := &metrics.ServerlessMetricAgent{
		SketchesBucketOffset: time.Second * 0,
		Tagger:               tagger,
	}
	// we don't want to add the container_id tag to metrics for cardinality reasons
	tags = serverlessInitTag.WithoutContainerID(tags)
	metricAgent.Start(5*time.Second, &metrics.MetricConfig{}, &metrics.MetricDogStatsD{})
	metricAgent.SetExtraTags(serverlessTag.MapToArray(tags))
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

func errorExitCode(err error) int {
	// if error is of type exec.ExitError then propagate the exit code
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		exitCode := exitError.ExitCode()
		log.Debugf("propagating exit code %v", exitCode)
		return exitCode
	}

	// use exit code 1 if there is no exit code in the error to propagate
	log.Debug("using default exit code 1")
	return 1
}
