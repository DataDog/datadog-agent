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
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	delegatedauthfx "github.com/DataDog/datadog-agent/comp/core/delegatedauth/fx"
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
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	"github.com/DataDog/datadog-agent/pkg/util/option"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/pkg/serverless"

	"go.uber.org/atomic"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	enhancedmetrics "github.com/DataDog/datadog-agent/cmd/serverless-init/enhanced-metrics"
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

var modeConf mode.Conf

func main() {

	modeConf = mode.DetectMode()
	setEnvWithoutOverride(modeConf.EnvDefaults)

	err := fxutil.OneShot(
		run,
		delegatedauthfx.Module(),
		workloadfilterfx.Module(),
		autodiscoveryimpl.Module(),
		fx.Provide(func() option.Option[healthplatform.Component] {
			return option.None[healthplatform.Component]()
		}),
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
func run(secretComp secrets.Component, delegatedAuthComp delegatedauth.Component, _ autodiscovery.Component, _ healthprobeDef.Component, tagger tagger.Component, compression logscompression.Component, hostname hostnameinterface.Component) error {
	cloudService, logConfig, tracingCtx, metricAgent, logsAgent, enhancedMetricsCollector, enhancedMetricsEnabled := setup(secretComp, delegatedAuthComp, modeConf, tagger, compression, hostname)

	err := modeConf.Runner(logConfig)

	// Defers are LIFO. We want to run the cloud service shutdown logic before last flush.
	defer lastFlush(logConfig.FlushTimeout, metricAgent, logsAgent)
	defer tracingCtx.TraceAgent.Stop() // synchronous: drains traces, flushes stats, sends to network
	defer func() {
		cloudService.Shutdown(*metricAgent, enhancedMetricsEnabled, err) // submits task.ended metric

		if enhancedMetricsCollector != nil {
			enhancedMetricsCollector.Stop()
		}

		metricAgent.WaitForPendingSamples() // wait for worker to consume them
	}()

	return err
}

func setup(secretComp secrets.Component, delegatedAuthComp delegatedauth.Component, _ mode.Conf, tagger tagger.Component, compression logscompression.Component, hostname hostnameinterface.Component) (cloudservice.CloudService, *serverlessInitLog.Config, *cloudservice.TracingContext, *metrics.ServerlessMetricAgent, logsAgent.ServerlessLogsAgent, *enhancedmetrics.Collector, bool) {
	tracelog.SetLogger(log.NewWrapper(3))

	// load proxy settings
	pkgconfigsetup.LoadProxyFromEnv(pkgconfigsetup.Datadog())

	cloudService := cloudservice.GetCloudServiceType()

	log.Debugf("Detected cloud service: %s", cloudService.GetOrigin())

	tagConfig := configureTags(cloudService)

	defaultSource := cloudService.GetDefaultLogsSource()
	agentLogConfig := serverlessInitLog.CreateConfig(defaultSource)

	// The datadog-agent requires Load to be called or it could
	// panic down the line.
	err := pkgconfigsetup.LoadDatadog(pkgconfigsetup.Datadog(), secretComp, delegatedAuthComp, nil)
	if err != nil {
		log.Debugf("Error loading config: %v\n", err)
	}

	// Disable UDS listener for serverless environments - traces are sent via HTTP to localhost instead.
	// This avoids noisy error logs.
	pkgconfigsetup.Datadog().Set("apm_config.receiver_socket", "", model.SourceAgentRuntime)

	origin := cloudService.GetOrigin()
	// Note: we do not modify tags for the LogsAgent.
	logsAgent := serverlessInitLog.SetupLogAgent(agentLogConfig, tagConfig.Tags, tagger, compression, hostname, origin)

	// When no API key is configured, skip trace and metric agent initialization
	// to avoid noisy error logs. The process wrapper and logs agent still function normally.
	// Also check the deprecated apm_config.api_key, which the trace agent still honors.
	apiKey := configUtils.SanitizeAPIKey(pkgconfigsetup.Datadog().GetString("api_key"))
	apmAPIKey := configUtils.SanitizeAPIKey(pkgconfigsetup.Datadog().GetString("apm_config.api_key"))
	if apiKey == "" && apmAPIKey == "" {
		log.Warnf("DD_API_KEY is not set; trace and metric collection are disabled. Set DD_API_KEY to enable monitoring.")
		traceAgent := trace.NewNoopTraceAgent()
		tracingCtx := &cloudservice.TracingContext{TraceAgent: traceAgent}
		metricAgent := &metrics.ServerlessMetricAgent{
			Tagger: tagger,
		}
		return cloudService, agentLogConfig, tracingCtx, metricAgent, logsAgent, nil, false
	}

	traceTags := serverlessInitTag.MakeTraceAgentTags(tagConfig.Tags)
	traceAgent := setupTraceAgent(traceTags, tagConfig.ConfiguredTags, tagger, origin)

	tracingCtx := &cloudservice.TracingContext{
		TraceAgent: traceAgent,
		SpanTags:   traceTags,
	}

	// TODO check for errors and exit
	_ = cloudService.Init(tracingCtx)

	metricAgent := setupMetricAgent(tagConfig.Tags, tagConfig.EnhancedMetricTags, tagConfig.EnhancedUsageMetricTags, tagger, cloudService.ShouldForceFlushAllOnForceFlushToSerializer())

	enhancedMetricsEnabled := pkgconfigsetup.Datadog().GetBool("enhanced_metrics")
	if enhancedMetricsEnabled {
		cloudService.AddStartMetric(metricAgent)
	}

	setupOtlpAgent(metricAgent, tagger)

	var enhancedMetricsCollector *enhancedmetrics.Collector
	if enhancedMetricsEnabled {
		enhancedMetricsCollector, err = enhancedmetrics.NewCollector(metricAgent, cloudService.GetSource(), cloudService.GetMetricPrefix(), cloudService.GetUsageMetricSuffix(), 3*time.Second)
		if err != nil {
			log.Warnf("Failed to initialize enhanced metrics collector: %v", err)
		} else {
			go enhancedMetricsCollector.Start()
		}
	}

	go flushMetricsAgent(metricAgent)
	return cloudService, agentLogConfig, tracingCtx, metricAgent, logsAgent, enhancedMetricsCollector, enhancedMetricsEnabled
}

// tagConfiguration holds the various tag sets for telemetry.
type tagConfiguration struct {
	ConfiguredTags []string // tags derived from DD_TAGS and DD_EXTRA_TAGS

	// tags derived from DD_TAGS and DD_EXTRA_TAGS, service, env, version, and tags derived from cloud service.
	// for use on dogstatsd metrics, legacy enhanced metrics, logs, and traces.
	Tags                    map[string]string
	EnhancedMetricTags      map[string]string // subset of tags derived from cloud service for enhanced metrics.
	EnhancedUsageMetricTags map[string]string // subset of tags derived from cloud service for enhanced usage metrics, including a high cardinality instance/replica tag.
}

func configureTags(cloudService cloudservice.CloudService) tagConfiguration {
	configuredTags := configUtils.GetConfiguredTags(pkgconfigsetup.Datadog(), false)
	configuredTagsMap := serverlessTag.ArrayToMap(configuredTags)

	baseTags := serverlessInitTag.GetBaseTagsMap()
	cloudTags := cloudService.GetTags()

	tags := serverlessTag.MergeWithOverwrite(baseTags, configuredTagsMap, cloudTags)

	serverlessInitTag.SetVersionMode(tags, modeConf.TagVersionMode)

	enhancedMetricTagSets := cloudService.GetEnhancedMetricTags(cloudTags)
	enhancedMetricTags := serverlessTag.MergeWithOverwrite(baseTags, configuredTagsMap, enhancedMetricTagSets.Base)

	serverlessInitTag.SetVersionMode(enhancedMetricTags, modeConf.TagVersionModeEnhancedMetrics)
	serverlessInitTag.SetSidecarModeTag(enhancedMetricTags, modeConf.SidecarMode)

	serverlessInitTag.SetVersionMode(enhancedMetricTagSets.Usage, modeConf.TagVersionModeEnhancedMetrics)
	serverlessInitTag.SetSidecarModeTag(enhancedMetricTagSets.Usage, modeConf.SidecarMode)

	return tagConfiguration{
		ConfiguredTags:          configuredTags,
		Tags:                    tags,
		EnhancedMetricTags:      enhancedMetricTags,
		EnhancedUsageMetricTags: enhancedMetricTagSets.Usage,
	}
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

func setupMetricAgent(tags map[string]string, enhancedMetricTags map[string]string, enhancedUsageMetricTags map[string]string, tagger tagger.Component, shouldForceFlushAllOnForceFlushToSerializer bool) *metrics.ServerlessMetricAgent {
	pkgconfigsetup.Datadog().Set("use_v2_api.series", true, model.SourceAgentRuntime)
	pkgconfigsetup.Datadog().Set("dogstatsd_socket", "", model.SourceAgentRuntime)

	metricTags := serverlessInitTag.MakeMetricAgentTags(tags)

	metricAgent := &metrics.ServerlessMetricAgent{
		SketchesBucketOffset: time.Second * 0,
		Tagger:               tagger,
	}
	metricAgent.Start(5*time.Second, &metrics.MetricConfig{}, &metrics.MetricDogStatsD{}, shouldForceFlushAllOnForceFlushToSerializer, serverlessTag.MapToArray(metricTags), serverlessTag.MapToArray(enhancedMetricTags), serverlessTag.MapToArray(enhancedUsageMetricTags))
	return metricAgent
}

func setupOtlpAgent(metricAgent *metrics.ServerlessMetricAgent, tagger tagger.Component) {
	if !otlp.IsEnabled() {
		log.Debugf("otlp endpoint disabled")
		return
	}

	if metricAgent == nil || metricAgent.Demux == nil {
		log.Warn("metric agent or demux not ready, skipping OTLP agent setup")
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

func lastFlush(flushTimeout time.Duration, metricAgent serverless.FlushableAgent, logsAgent logsAgent.ServerlessLogsAgent) bool {
	hasTimeout := atomic.NewInt32(0)
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go flushAndWait(flushTimeout, wg, metricAgent, hasTimeout)
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
