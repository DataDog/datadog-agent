// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/exitcode"
	serverlessInitLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	demultiplexer "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	demultiplexerimpl "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/impl"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	delegatedauthfx "github.com/DataDog/datadog-agent/comp/core/delegatedauth/fx"
	delegatedauthnooptypes "github.com/DataDog/datadog-agent/comp/core/delegatedauth/noop-impl/types"
	healthprobeDef "github.com/DataDog/datadog-agent/comp/core/healthprobe/def"
	healthprobeFx "github.com/DataDog/datadog-agent/comp/core/healthprobe/fx"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx"
	secretnooptypes "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl/types"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	localTaggerFx "github.com/DataDog/datadog-agent/comp/core/tagger/fx"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/fx-noop"
	"github.com/DataDog/datadog-agent/comp/dogstatsd"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server/def"
	filterlistfx "github.com/DataDog/datadog-agent/comp/filterlist/fx"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	defaultforwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformfx "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/fx"
	eventplatformreceiverimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/impl"
	orchestrator "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/def"
	orchestratorfx "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/fx"
	haagentfx "github.com/DataDog/datadog-agent/comp/haagent/fx"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	metricscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/pkg/aggregator"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	enhancedmetrics "github.com/DataDog/datadog-agent/cmd/serverless-init/enhanced-metrics"
	serverlessInitTag "github.com/DataDog/datadog-agent/cmd/serverless-init/tag"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent/def"
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

// Shutdown time budget for serverless-init. These values sum to ~9s, which
// fits within the tightest supported platform grace window. Platform defaults:
//
//	Cloud Run (tightest): 10s — https://docs.cloud.google.com/run/docs/container-contract#shutdown
//	Azure Container Apps:  30s — https://learn.microsoft.com/en-us/azure/container-apps/application-lifecycle-management#shutdown
//	Azure App Service:     30s via WEBSITES_SHUTDOWN_TIMEOUT (scale-in SIGTERM delivery is unreliable)
//
// Any change here is the only edit needed — all phases read these constants.
//
// The three metrics-pipeline bounds (aggregator → forwarder) are separate
// because the metrics path stages independently: the demux/aggregator drains
// its sample channel and serializes the final payload, then the forwarder
// purges any HTTP transactions queued during that drain.
const (
	// traceStopTimeout bounds Stop()'s wait for the trace agent's Run loop
	// to exit. Best-effort; logs a warning on overrun and continues shutdown.
	traceStopTimeout = 2500 * time.Millisecond

	// logsFlushTimeout bounds flushLogsAgent's combined drain-then-flush of
	// buffered customer log records: DrainTailers stops the file/channel
	// tailers so each performs one final EOF read into the pipeline, then
	// Flush ships it. Strict ctx shared by both steps; cancels in-progress
	// work on overrun. The drain step is normally sub-millisecond (one final
	// read of the small cold-start file), so the budget is unchanged from
	// when this bounded only the flush.
	logsFlushTimeout = 1500 * time.Millisecond

	// metricsAggregatorStopTimeoutSeconds bounds the demux Stop: forces a
	// final flush of the metrics pipeline, including incomplete dogstatsd
	// buckets, into the serializer and through the forwarder. Set via the
	// aggregator_stop_timeout config key (integer seconds).
	metricsAggregatorStopTimeoutSeconds = 2

	// metricsForwarderStopTimeoutSeconds bounds the purge phase of the
	// DefaultForwarder. In-flight HTTP requests continue in background
	// goroutines after Stop returns. Set via the forwarder_stop_timeout
	// config key (integer seconds). Sized a second above the other phases so a
	// slow final-flush POST has room to land before the process exits.
	metricsForwarderStopTimeoutSeconds = 3

	// metricsFlushInterval is the demux periodic flush cadence during the
	// run (not shutdown). Kept here so the full picture is in one place: a
	// tick that fires close to SIGTERM consumes part of the shutdown budget.
	metricsFlushInterval = 3 * time.Second

	// shutdownBudgetWatchdog fires a debug log if total shutdown elapsed time
	// exceeds this. Four shutdown phases run, each independently
	// timeout-bounded: trace (2.5 s) + logs (1.5 s) + demux Stop (2 s) +
	// forwarder purge (3 s). The DogStatsD server and demux Stop phases now
	// drain the worker batchers and the aggregator sample channel internally
	// (via the dogstatsd_flush_incomplete_buckets gate wired in preloadEarly),
	// still bounded by aggregator_stop_timeout
	// (2 s) — so the old explicit ServerlessFlush (1 s) and metric-drain
	// (0.1 s) phases are gone. Worst-case sum is ≈9 s — strictly under both
	// the watchdog and Cloud Run's 10 s grace window. In practice each phase
	// finishes in milliseconds (the worker batchers are small, the sample
	// channels short, and HTTP requests either land or were already in
	// flight), so the watchdog only fires on a genuine overrun rather than a
	// guaranteed-late tripwire. The debug log is observability, not a
	// guarantee.
	shutdownBudgetWatchdog = 9*time.Second + 500*time.Millisecond
)

var modeConf mode.Conf

// preloadEarly mutates the global Datadog() config with the overrides that the
// forwarder, DogStatsD server and demultiplexer read at Fx-construction time.
// These must be applied before fxutil.OneShot so that they win over any values
// loaded from env vars or datadog.yaml (SourceAgentRuntime has higher priority
// than SourceEnvVar/SourceFile).
func preloadEarly() {
	// Serverless containers don't persist across restarts, so disk-spill of
	// undelivered transactions has no value. Disable it explicitly even though
	// the default is already 0 — keeps behavior deterministic under user
	// overrides or future default changes.
	setOverride("forwarder_storage_max_size_in_bytes", 0)

	// Bound DefaultForwarder.Stop()'s purge window so shutdown stays inside
	// the platform grace budget (Cloud Run default 10s), leaving room for
	// the aggregator flush, logs flush, and trace agent stop.
	setOverride("forwarder_stop_timeout", metricsForwarderStopTimeoutSeconds)

	// Wait for in-flight HTTP transactions to complete before cancelling the
	// worker context so the final flush is not dropped by a racing context
	// cancel. The outer bound is still forwarder_stop_timeout
	// (metricsForwarderStopTimeoutSeconds above).
	setOverride("forwarder_stop_wait_for_inflight", true)

	// Bound AgentDemultiplexer.Stop()'s flush window. Explicit here so all
	// metrics-pipeline bounds are in one place and readable alongside the
	// forwarder timeout above.
	setOverride("aggregator_stop_timeout", metricsAggregatorStopTimeoutSeconds)

	// Prevent any UDP packets from being stuck in the buffer and not parsed:
	// with this option set to 1ms, all packets received are immediately sent
	// to the parser.
	setOverride("dogstatsd_packet_buffer_flush_timeout", 1*time.Millisecond)

	// DogStatsD in serverless-init listens on UDP only — suppress the default
	// UDS listener, which requires a filesystem socket that has no use here.
	setOverride("dogstatsd_socket", "")

	// Also suppress the UDS streams listener. Its default is already empty (so
	// it never starts), but set it explicitly so a user override can't turn on
	// a filesystem socket that has no use in serverless-init.
	setOverride("dogstatsd_stream_socket", "")

	// Force series v2 API for the serializer.
	setOverride("use_v2_api.series", true)

	// Send metric payloads uncompressed, as the bespoke serverless demultiplexer
	// did before this bundle replaced it. Left unset, the serializer would use
	// the default compressor kind, which serverless-init's build tags may not
	// compile in. Adopting a compressor here is tracked in SVLS-9451.
	setOverride("serializer_compressor_kind", "none")

	// Disable UDS listener for the APM receiver — traces are sent via HTTP to
	// localhost in serverless. Avoids noisy error logs.
	setOverride("apm_config.receiver_socket", "")

	// Opt the whole metrics pipeline into flushing everything it holds on
	// shutdown. Long-running agents leave this off (its default) and discard
	// in-flight samples on stop; serverless-init terminates after a single
	// workload, so the final samples must be reported. This single flag drives
	// both shutdown stages, which fire in reverse Fx construction order:
	//   - dsdServer.stop() drains each DogStatsD worker batcher into the time
	//     sampler as the workers exit, then
	//   - demux.Stop() drains the aggregator sample channel and includes the
	//     incomplete bucket in the final ForceFlushToSerializer (otherwise a
	//     workload that ends mid-bucket would drop its last bucket), then
	//   - SharedForwarder.Stop() drains in-flight transactions.
	// All bounded by aggregator_stop_timeout / forwarder_stop_timeout. It only
	// affects the shutdown flush: AgentDemultiplexer's periodic flushLoop
	// hardcodes forceFlushAll=false on ticks, so bucket-aligned flushes during
	// the run are preserved.
	setOverride("dogstatsd_flush_incomplete_buckets", true)

	// The non-atomic registry writer MkdirAll's logs_config.run_path, so the
	// file-tailer registry directory doesn't need to pre-exist. Unlike the
	// overrides above this one is meant to remain user-overridable (see
	// registryRunPathDefault), so it can't use setOverride's fixed
	// SourceAgentRuntime.
	setOverride("logs_config.atomic_registry_write", false)
	if runPath := registryRunPathDefault(); runPath != "" {
		cfg := pkgconfigsetup.Datadog()
		// Only replace the built-in default: a user-configured run_path
		// (DD_LOGS_CONFIG_RUN_PATH, or logs_config.run_path in datadog.yaml,
		// loaded after preloadEarly) must still win. Setting our value at
		// SourceDefault - rather than setOverride's SourceAgentRuntime -
		// keeps it below both of those priorities.
		if cfg.GetSource("logs_config.run_path") == model.SourceDefault {
			cfg.Set("logs_config.run_path", runPath, model.SourceDefault)
		}
	}
}

// registryRunPathDefault returns the directory the file-tailer registry
// should persist into, so its offsets survive a restart within the same
// instance and "beginning" tailing (cmd/serverless-init/log/log.go) doesn't
// re-ship the whole file. Cloud Run, Cloud Run Jobs and Container Apps tail
// the path in DD_SERVERLESS_LOG_PATH, which sits on the same volume the
// customer app writes to - so the registry goes into that path's directory.
// Azure App Service doesn't use that env var (its log source is
// origin=="appservice" && DD_AAS_INSTANCE_LOGGING_ENABLED, resolved later via
// cloudservice.GetCloudServiceType, too late for preloadEarly); it's detected
// here directly via the same WEBSITE_STACK env var cloudservice/appservice.go
// uses, and pointed at serverlessInitLog.AASPersistentLogDir - the directory
// AAS persists across instance restarts - so "beginning" is safe there too.
// Returns "" when neither signal is present, leaving logs_config.run_path at
// its built-in default.
func registryRunPathDefault() string {
	if logPath := os.Getenv("DD_SERVERLESS_LOG_PATH"); logPath != "" {
		return filepath.Dir(logPath)
	}
	if _, isAppService := os.LookupEnv(cloudservice.WebsiteStack); isAppService {
		return serverlessInitLog.AASPersistentLogDir
	}
	return ""
}

// setOverride sets key to val with SourceAgentRuntime priority, logging a
// debug message if the config already has a user-supplied value that differs.
// This makes it visible in debug logs when serverless-init silently overrides
// a user-configured setting.
//
// Note: the `current != val` comparison uses interface{} equality, so if the
// config layer returns a different numeric type for the stored value (e.g.
// int64 vs int for timeout constants), the log fires even when the values are
// semantically identical. This is debug-only; no behavioral impact.
//
// Note: setOverride is called from preloadEarly, which runs before
// LoadDatadog. At that point only viper defaults and env-var bindings are
// visible — yaml-configured values from datadog.yaml are not yet loaded. So
// the "overriding user-configured" log catches env-var overrides reliably
// but misses yaml ones. Do not rely on this log as an authoritative record
// of every silently-overridden user setting.
func setOverride(key string, val interface{}) {
	cfg := pkgconfigsetup.Datadog()
	current := cfg.Get(key)
	cfg.Set(key, val, model.SourceAgentRuntime)
	if current != nil && current != val {
		log.Debugf("serverless-init: overriding user-configured %s=%v with %v", key, current, val)
	}
}

func main() {

	preloadEarly()

	modeConf = mode.DetectMode()
	setEnvWithoutOverride(modeConf.EnvDefaults)

	// Load the config file early so that yaml-configured values (e.g.
	// api_key, dogstatsd_tags, tags, extra_tags) are visible to the tag
	// computation, the merge, and the api_key check below. setup() calls
	// LoadDatadog again with the real Fx-injected components; that is
	// intentional — the second call resolves secrets and applies any
	// delegated-auth overrides. The noop implementations used here are
	// consistent with the pattern in cmd/agent/common/import.go.
	//
	// Caveat: because this early load uses noop secrets, ENC[...] secret
	// references inside yaml-configured `tags` / `extra_tags` are NOT
	// resolved here, so metricTags below is snapshotted from this pre-secrets
	// view. The second LoadDatadog in setup() resolves them for the config,
	// but the already-snapshotted metricTags keep the raw values.
	if err := pkgconfigsetup.LoadDatadog(pkgconfigsetup.Datadog(), &secretnooptypes.SecretNoop{}, &delegatedauthnooptypes.DelegatedAuthNoop{}, nil); err != nil {
		log.Debugf("early config load error (non-fatal): %v", err)
	}

	cloudService := cloudservice.GetCloudServiceType()
	log.Debugf("Detected cloud service: %s", cloudService.GetOrigin())

	// Compute tags after the early LoadDatadog so that yaml-configured
	// `tags` and `extra_tags` (read by configUtils.GetConfiguredTags inside
	// configureTags) are picked up. Env-based DD_TAGS / DD_EXTRA_TAGS work
	// either way via viper BindEnv, but yaml values are only visible once
	// the config file has been parsed.
	tagConfig := configureTags(cloudService)
	metricAgentTags := serverlessTag.MapToArray(serverlessInitTag.MakeMetricAgentTags(tagConfig.Tags))

	// Merge user-configured dogstatsd_tags (from env/file, loaded above) with
	// the cloud-service-derived metric-agent tags. The previous serverless
	// metric-agent flow appended metric-agent tags to the server's existing
	// extraTags; restore that semantics so users setting DD_DOGSTATSD_TAGS /
	// dogstatsd_tags in yaml don't lose their tags when the metric-agent tags
	// are merged in. newServerCompat dedups via sort.UniqInPlace, so plain
	// append is sufficient.
	mergedDogstatsdTags := append(pkgconfigsetup.Datadog().GetStringSlice("dogstatsd_tags"), metricAgentTags...)
	pkgconfigsetup.Datadog().Set("dogstatsd_tags", mergedDogstatsdTags, model.SourceAgentRuntime)

	// Fast-fail: if no API key is configured (via env var or yaml at this
	// point), the dogstatsd server has no useful work — every sample it
	// ingests would be dropped by the forwarder. Skip its Fx lifecycle by
	// disabling use_dogstatsd. The forwarder and demux are still wired up
	// (matching Core Agent behavior) but stay quiet because nothing produces
	// samples.
	//
	// This gate reads api_key once, here. That is sufficient on the platforms
	// serverless-init ships to — Cloud Run, Cloud Run Jobs, Container Apps and
	// App Service (see cmd/serverless-init/cloudservice/service.go) — because
	// none of them use delegated auth (the only provider is AWS, ProviderAWS in
	// comp/core/delegatedauth/api/cloudauth/config/config.go), so api_key cannot
	// appear or change after this point.
	//
	// We only check the top-level api_key here. apm_config.api_key is
	// checked separately inside setup() and is unaffected.
	if configUtils.SanitizeAPIKey(pkgconfigsetup.Datadog().GetString("api_key")) == "" {
		pkgconfigsetup.Datadog().Set("use_dogstatsd", false, model.SourceAgentRuntime)
	}

	metricTags := metrics.Tags{
		Metric:              metricAgentTags,
		EnhancedMetric:      serverlessTag.MapToArray(tagConfig.EnhancedMetricTags),
		EnhancedUsageMetric: serverlessTag.MapToArray(tagConfig.EnhancedUsageMetricTags),
	}

	err := fxutil.OneShot(
		run,
		fx.Provide(func() cloudservice.CloudService { return cloudService }),
		fx.Supply(tagConfig),
		fx.Supply(metricTags),
		delegatedauthfx.Module(),
		healthplatform.Bundle(),
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
		metricscompressionfx.Module(),
		filterlistfx.Module(),
		orchestratorfx.Module(orchestrator.NewNoopParams()),
		eventplatformfx.Module(eventplatform.NewDisabledParams()),
		eventplatformreceiverimpl.Module(),
		haagentfx.Module(),
		forwarder.Bundle(defaultforwarder.NewParams()),
		demultiplexerimpl.Module(demultiplexerimpl.NewDefaultParams(
			demultiplexerimpl.WithFlushInterval(metricsFlushInterval),
			demultiplexerimpl.WithContinueOnMissingHostname(),
		)),
		dogstatsd.Bundle(dogstatsdServer.Params{Serverless: true}),
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
func run(
	secretComp secrets.Component,
	delegatedAuthComp delegatedauth.Component,
	_ healthprobeDef.Component,
	tagger tagger.Component,
	logsCompression logscompression.Component,
	hostname hostnameinterface.Component,
	_ defaultforwarder.Component,
	_ demultiplexer.Component,
	demux aggregator.Demultiplexer,
	// dsdServer is injected so Fx constructs and starts the DogStatsD server
	// (and fires its OnStop flush hook on shutdown); the body no longer calls
	// into it directly now that flush-on-stop is internal to the component.
	_ dogstatsdServer.Component,
	cloudService cloudservice.CloudService,
	tagConfig tagConfiguration,
	metricTags metrics.Tags,
) error {
	cloudService, logConfig, tracingCtx, metricAgent, logsAgent, enhancedMetricsCollector, enhancedMetricsEnabled := setup(
		secretComp, delegatedAuthComp, modeConf, tagger, logsCompression, hostname,
		cloudService, tagConfig, metricTags, demux,
	)

	err := modeConf.Runner(logConfig)

	// Defers are LIFO. Order of execution:
	//   1. Watchdog timer starts (debug log if shutdown exceeds the budget).
	//   2. cloudService.Shutdown submits the task.ended metric; the enhanced
	//      metrics collector stops emitting. These final samples are enqueued
	//      via Demux.AggregateSample (asynchronous) and are picked up by the
	//      time-sampler workers during step 5's demux drain.
	//   3. trace agent stops (drains traces, flushes stats, sends) — bounded
	//      by traceStopTimeout (2.5 s).
	//   4. logs agent drains its tailers (forcing a final EOF read of any
	//      line written just before SIGTERM) and then flushes any buffered
	//      records — both bounded together by logsFlushTimeout (1.5 s).
	//   5. run() returns; Fx OnStop hooks fire in reverse construction order,
	//      and the metrics pipeline flushes itself (no external orchestration):
	//        5a. dsdServer.stop() — gated by dogstatsd_flush_incomplete_buckets
	//            (set in preloadEarly) — closes stopChan; each worker flushes its
	//            batcher (batcher.flush() → AggregateSamples → samplesChan push)
	//            as it exits its run loop, and stop() waits on workerWg so every
	//            flush has completed before it returns. The samples from steps 2
	//            and 5a are therefore in the samplers' sample channels before
	//            step 5b runs. (The listener→worker packetsIn queue is still NOT
	//            drained, so UDP packets arriving after the last worker run may
	//            be dropped as the server tears down.)
	//        5b. demux.Stop() — gated by dogstatsd_flush_incomplete_buckets
	//            (set in preloadEarly) — first drains each time-sampler worker's
	//            sample channel (worker.shutdown/waitForShutdown) so the samples
	//            from steps 2 and 5a land in the samplers, then forces a final
	//            flush (incomplete buckets included) into the serializer. Both
	//            the drain and the flush are bounded together by
	//            aggregator_stop_timeout (2 s).
	//        5c. SharedForwarder.Stop() — drains in-flight HTTP transactions —
	//            bounded by forwarder_stop_timeout (2 s).
	defer flushLogsAgent(logConfig.FlushTimeout, logsAgent)
	defer tracingCtx.TraceAgent.Stop()
	defer func() {
		cloudService.Shutdown(metricAgent, enhancedMetricsEnabled, err)

		if enhancedMetricsCollector != nil {
			enhancedMetricsCollector.Stop()
		}
	}()
	// Watchdog: log a single debug line if shutdown overruns the budget.
	// Declared last → executes first → captures shutdown start. The timer
	// runs in the background; if shutdown finishes early the log just never
	// fires (the goroutine is killed when the process exits). No cleanup
	// needed.
	defer func() {
		start := time.Now()
		time.AfterFunc(shutdownBudgetWatchdog, func() {
			log.Debugf("serverless-init shutdown exceeded %v budget (elapsed: %v)",
				shutdownBudgetWatchdog, time.Since(start))
		})
	}()

	return err
}

func setup(
	secretComp secrets.Component,
	delegatedAuthComp delegatedauth.Component,
	_ mode.Conf,
	tagger tagger.Component,
	compression logscompression.Component,
	hostname hostnameinterface.Component,
	cloudService cloudservice.CloudService,
	tagConfig tagConfiguration,
	metricTags metrics.Tags,
	demux aggregator.Demultiplexer,
) (cloudservice.CloudService, *serverlessInitLog.Config, *cloudservice.TracingContext, *metrics.ServerlessMetricAgent, logsAgent.ServerlessLogsAgent, *enhancedmetrics.Collector, bool) {
	tracelog.SetLogger(log.NewWrapper(3))

	// load proxy settings
	pkgconfigsetup.LoadProxyFromEnv(pkgconfigsetup.Datadog())

	defaultSource := cloudService.GetDefaultLogsSource()
	agentLogConfig := serverlessInitLog.CreateConfig(defaultSource, logsFlushTimeout)

	// The datadog-agent requires Load to be called or it could
	// panic down the line.
	err := pkgconfigsetup.LoadDatadog(pkgconfigsetup.Datadog(), secretComp, delegatedAuthComp, nil)
	if err != nil {
		log.Debugf("Error loading config: %v\n", err)
	}

	origin := cloudService.GetOrigin()
	// Note: we do not modify tags for the LogsAgent.
	logsAgent := serverlessInitLog.SetupLogAgent(agentLogConfig, tagConfig.Tags, tagger, compression, hostname, origin)

	// When no API key is configured, skip trace agent initialization
	// to avoid noisy error logs. The process wrapper and logs agent still function normally.
	// Also check the deprecated apm_config.api_key, which the trace agent still honors.
	// NOTE: the Fx-managed forwarder, demultiplexer and DogStatsD server are
	// constructed and started unconditionally during fxutil.OneShot startup.
	// Without an API key the forwarder will log HTTP errors, but the process
	// continues to run (matching Core Agent behavior).
	apiKey := configUtils.SanitizeAPIKey(pkgconfigsetup.Datadog().GetString("api_key"))
	apmAPIKey := configUtils.SanitizeAPIKey(pkgconfigsetup.Datadog().GetString("apm_config.api_key"))
	if apiKey == "" && apmAPIKey == "" {
		log.Warnf("DD_API_KEY is not set; trace and metric collection are disabled. Set DD_API_KEY to enable monitoring.")
		traceAgent := trace.NewNoopTraceAgent()
		tracingCtx := &cloudservice.TracingContext{TraceAgent: traceAgent}
		return cloudService, agentLogConfig, tracingCtx, nil, logsAgent, nil, false
	}

	traceTags := serverlessInitTag.MakeTraceAgentTags(tagConfig.Tags)
	traceAgent := setupTraceAgent(traceTags, tagConfig.ConfiguredTags, tagger, origin)

	tracingCtx := &cloudservice.TracingContext{
		TraceAgent: traceAgent,
		SpanTags:   traceTags,
	}

	// TODO check for errors and exit
	_ = cloudService.Init(tracingCtx)

	metricAgent := metrics.New(demux, metricTags)

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
	//
	// Note: the deprecated DD_APM_SPAN_DERIVED_PRIMARY_TAGS option is honored for
	// serverless-init (and the AAS extension) inside comp/trace/config/impl/setup.go
	// (gated on serverless.enabled || IsAzureAppServicesExtension()). It lives there
	// rather than here because the AAS extension shares the same gate but doesn't go
	// through StartServerlessTraceAgent. Treat that block as serverless-only despite
	// its location in shared trace-agent config code.
	functionTags := strings.Join(configuredTags, ",")
	traceAgent := trace.StartServerlessTraceAgent(trace.StartServerlessTraceAgentArgs{
		Enabled:               pkgconfigsetup.Datadog().GetBool("apm_config.enabled"),
		LoadConfig:            &trace.LoadConfig{Path: datadogConfigPath, Tagger: tagger},
		AdditionalProfileTags: profileTags,
		FunctionTags:          functionTags,
		StopTimeout:           traceStopTimeout,
	})
	traceAgent.SetTags(tags)
	go func() {
		for range time.Tick(3 * time.Second) {
			traceAgent.Flush()
		}
	}()
	return traceAgent
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

// flushLogsAgent drains the logs agent's tailers and flushes it, bounded by
// a single timeout. Draining first forces the file tailer's final EOF read
// (see ServerlessLogsAgent.DrainTailers) so a line written right before
// SIGTERM - which the CPU-throttled tailer never got scheduled to read - is
// captured before the pipeline is flushed and shipped. Metrics are flushed
// by the demultiplexer's Fx OnStop hook (demux.Stop()), so this helper is
// logs-only.
func flushLogsAgent(flushTimeout time.Duration, agent logsAgent.ServerlessLogsAgent) {
	if agent == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	agent.DrainTailers(ctx)
	agent.Flush(ctx)
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
