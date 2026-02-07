// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/exitcode"
	serverlessInitLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	healthprobeDef "github.com/DataDog/datadog-agent/comp/core/healthprobe/def"
	healthprobeFx "github.com/DataDog/datadog-agent/comp/core/healthprobe/fx"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	localTaggerFx "github.com/DataDog/datadog-agent/comp/core/tagger/fx"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	workloadfilterfx "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"

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
	serverlessTag "github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	tracelog "github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const datadogConfigPath = "datadog.yaml"

// agents holds the serverless agents for metrics, traces, and logs.
type agents struct {
	metric  *metrics.ServerlessMetricAgent
	tracing *cloudservice.TracingContext
	logs    logsAgent.ServerlessLogsAgent
}

// setupResult holds all components initialized during setup.
type setupResult struct {
	cloudService cloudservice.CloudService
	logConfig    *serverlessInitLog.Config
	agents       *agents
}

var modeConf mode.Conf

func main() {

	modeConf = mode.DetectMode()
	setEnvWithoutOverride(modeConf.EnvDefaults)

	err := fxutil.OneShot(
		run,
		workloadfilterfx.Module(),
		autodiscoveryimpl.Module(),
		fx.Provide(func(config coreconfig.Component) healthprobeDef.Options {
			return healthprobeDef.Options{
				Port:           config.GetInt("health_port"),
				LogsGoroutines: config.GetBool("log_all_goroutines_when_unhealthy"),
			}
		}),
		localTaggerFx.Module(),
		healthprobeFx.Module(),
		workloadmetafx.Module(workloadmeta.NewParams()),
		fx.Supply(coreconfig.NewParams("")),
		coreconfig.Module(),
		logscompressionfx.Module(),
		secretsfx.Module(),
		fx.Supply(logdef.ForOneShot(modeConf.LoggerName, "error", true)),
		logfx.Module(),
		nooptelemetry.Module(),
		hostnameimpl.Module(),
	)

	if err != nil {
		log.Error(err)
		exitCode := exitcode.From(err)
		log.Debugf("propagating exit code %v", exitCode)
		log.Flush()
		os.Exit(exitCode)
	}
}

// removing these unused dependencies will cause silent crash due to fx framework
func run(secretComp secrets.Component, _ autodiscovery.Component, _ healthprobeDef.Component, tagger tagger.Component, compression logscompression.Component, hostname hostnameinterface.Component) error {
	result, cleanup, err := setup(secretComp, modeConf, tagger, compression, hostname)
	defer func() { cleanup(err) }()

	if err != nil {
		return err
	}

	err = modeConf.Runner(result.logConfig)
	return err
}

func setup(secretComp secrets.Component, _ mode.Conf, tagger tagger.Component, compression logscompression.Component, hostname hostnameinterface.Component) (*setupResult, func(error), error) {
	tracelog.SetLogger(log.NewWrapper(3))

	// load proxy settings
	pkgconfigsetup.LoadProxyFromEnv(pkgconfigsetup.Datadog())

	cloudService := cloudservice.GetCloudServiceType()

	log.Debugf("Detected cloud service: %s", cloudService.GetOrigin())

	configuredTags := configUtils.GetConfiguredTags(pkgconfigsetup.Datadog(), false)
	tags := serverlessInitTag.GetBaseTagsMapWithMetadata(
		serverlessTag.MergeWithOverwrite(
			serverlessTag.ArrayToMap(
				configuredTags,
			),
			cloudService.GetTags()),
		modeConf.TagVersionMode)

	defaultSource := cloudService.GetDefaultLogsSource()
	agentLogConfig := serverlessInitLog.CreateConfig(defaultSource)

	// The datadog-agent requires Load to be called or it could
	// panic down the line.
	err := pkgconfigsetup.LoadDatadog(pkgconfigsetup.Datadog(), secretComp, nil)
	if err != nil {
		log.Debugf("Error loading config: %v\n", err)
	}

	// Disable UDS listener for serverless environments - traces are sent via HTTP to localhost instead.
	// This avoids noisy error logs.
	pkgconfigsetup.Datadog().Set("apm_config.receiver_socket", "", model.SourceAgentRuntime)

	origin := cloudService.GetOrigin()
	// Note: we do not modify tags for the LogsAgent.
	logsAgent := serverlessInitLog.SetupLogAgent(agentLogConfig, tags, tagger, compression, hostname, origin)

	traceTags := serverlessInitTag.MakeTraceAgentTags(tags)
	traceAgent := setupTraceAgent(traceTags, configuredTags, tagger, origin)

	tracingCtx := &cloudservice.TracingContext{
		TraceAgent: traceAgent,
		SpanTags:   traceTags,
	}

	if err := cloudService.Init(tracingCtx); err != nil {
		// Cleanup for early exit: only trace and logs agents exist, no metric agent yet
		cleanup := func(_ error) {
			lastFlush(agentLogConfig.FlushTimeout, nil, traceAgent, logsAgent)
		}
		return nil, cleanup, err
	}

	metricTags := serverlessInitTag.MakeMetricAgentTags(tags)
	metricAgent := setupMetricAgent(metricTags, tagger, cloudService.ShouldForceFlushAllOnForceFlushToSerializer())

	metric.Add(cloudService.GetStartMetricName(), 1.0, cloudService.GetSource(), *metricAgent)

	setupOtlpAgent(metricAgent, tagger)

	go flushMetricsAgent(metricAgent)

	// Cleanup for success: all agents exist
	cleanup := func(err error) {
		cloudService.Shutdown(*metricAgent, err)
		lastFlush(agentLogConfig.FlushTimeout, metricAgent, tracingCtx.TraceAgent, logsAgent)
	}

	return &setupResult{
		cloudService: cloudService,
		logConfig:    agentLogConfig,
		agents: &agents{
			metric:  metricAgent,
			tracing: tracingCtx,
			logs:    logsAgent,
		},
	}, cleanup, nil
}

var serverlessProfileTags = []string{
	// Azure tags
	"subscription_id",
	"resource_group",
	"resource_id",
	"replicate_name",
	"aca.subscription.id",
	"aca.resource.group",
	"aca.resource.id",
	"aca.replica.name",
	"aas.subscription.id",
	"aas.resource.group",
	"aas.resource.id",
	// Cloud-agnostic origin tag
	"_dd.origin",
}

func setupTraceAgent(tags map[string]string, configuredTags []string, tagger tagger.Component, origin string) trace.ServerlessTraceAgent {
	profileTags := make(map[string]string)
	for _, serverlessProfileTag := range serverlessProfileTags {
		if value, ok := tags[serverlessProfileTag]; ok {
			profileTags[serverlessProfileTag] = value
		}
	}

	// For Google Cloud Run Functions, add functionname tag to profiles so the profiling team can filter by functions
	if origin == cloudservice.CloudRunOrigin {
		_, functionTargetExists := os.LookupEnv("FUNCTION_TARGET")

		if functionTargetExists {
			profileTags["functionname"] = os.Getenv(cloudservice.ServiceNameEnvVar)
		}
	}

	// Note: serverless trace tag logic also in comp/trace/payload-modifier/impl/payloadmodifier_test.go
	functionTags := strings.Join(configuredTags, ",")
	traceAgent := trace.StartServerlessTraceAgent(trace.StartServerlessTraceAgentArgs{
		Enabled:               pkgconfigsetup.Datadog().GetBool("apm_config.enabled"),
		LoadConfig:            &trace.LoadConfig{Path: datadogConfigPath, Tagger: tagger},
		AdditionalProfileTags: profileTags,
		FunctionTags:          functionTags,
	})
	traceAgent.SetTags(tags)
	go func() {
		for range time.Tick(3 * time.Second) {
			traceAgent.Flush()
		}
	}()
	return traceAgent
}

func setupMetricAgent(tags map[string]string, tagger tagger.Component, shouldForceFlushAllOnForceFlushToSerializer bool) *metrics.ServerlessMetricAgent {
	pkgconfigsetup.Datadog().Set("use_v2_api.series", false, model.SourceAgentRuntime)
	pkgconfigsetup.Datadog().Set("dogstatsd_socket", "", model.SourceAgentRuntime)

	metricAgent := &metrics.ServerlessMetricAgent{
		SketchesBucketOffset: time.Second * 0,
		Tagger:               tagger,
	}
	metricAgent.Start(5*time.Second, &metrics.MetricConfig{}, &metrics.MetricDogStatsD{}, shouldForceFlushAllOnForceFlushToSerializer)
	metricAgent.SetExtraTags(serverlessTag.MapToArray(tags))
	return metricAgent
}

func setupOtlpAgent(metricAgent *metrics.ServerlessMetricAgent, tagger tagger.Component) {
	if !otlp.IsEnabled() {
		log.Debugf("otlp endpoint disabled")
		return
	}
	otlpAgent := otlp.NewServerlessOTLPAgent(metricAgent.Demux.Serializer(), tagger)
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
	defer wg.Done()
	if agent == nil {
		return
	}
	childCtx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	ch := make(chan struct{}, 1)
	go flushWithContext(childCtx, ch, agent.Flush)
	select {
	case <-childCtx.Done():
		hasTimeout.Inc()
	case <-ch:
	}
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
