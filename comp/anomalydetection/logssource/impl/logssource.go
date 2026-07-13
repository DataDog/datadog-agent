// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logssourceimpl implements the logssource component.
package logssourceimpl

import (
	"context"
	"time"

	anomalydetectionconfig "github.com/DataDog/datadog-agent/comp/anomalydetection/config"
	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logsfilter"
	logssource "github.com/DataDog/datadog-agent/comp/anomalydetection/logssource/def"
	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	containerLauncher "github.com/DataDog/datadog-agent/pkg/logs/launchers/container"
	filelauncher "github.com/DataDog/datadog-agent/pkg/logs/launchers/file"
	journaldlauncher "github.com/DataDog/datadog-agent/pkg/logs/launchers/journald"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	logsadscheduler "github.com/DataDog/datadog-agent/pkg/logs/schedulers/ad"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	fileTailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires defines the dependencies for the logssource component.
type Requires struct {
	compdef.In

	Lc          compdef.Lifecycle
	Log         log.Component
	Config      config.Component
	Hostname    hostname.Component
	WMeta       option.Option[workloadmeta.Component]
	Tagger      tagger.Component
	Auditor     auditor.Component
	Observer    option.Option[observer.Component]
	FilterStore option.Option[workloadfilter.Component]

	// Autodiscovery is optional: when absent the AD scheduler is simply not started
	// and the observer falls back to generic container and kubelet log collection
	// without AD-scheduled config overlays.
	Autodiscovery autodiscovery.Component `fx:"optional"`
}

// Provides defines the output of the logssource component.
type Provides struct {
	compdef.Out
	Comp logssource.Component
}

type logssourceComponent struct{}

// NewComponent creates the logssource component.
//
// anomaly_detection.logs.enabled is the main toggle for all log ingestion:
// setting it to false disables container and kubelet sources wired here.
// anomaly_detection.logs.containers.enabled controls workloadmeta generic
// container sources and AD-scheduled container log configs.
// anomaly_detection.logs.kubelet.enabled controls the kubelet journald source.
// Agent-internal logs are wired separately by the observer via
// anomaly_detection.logs.internal.enabled (see observer/impl/observer.go).
//
// The component is a no-op when any of these are true:
//   - the observer is unavailable
//   - no observer-requiring gate is enabled and anomaly_detection.recording.enabled is false
//   - anomaly_detection.logs.enabled is false and anomaly_detection.recording.enabled is false
//   - only container sources are enabled and workloadmeta is unavailable
//   - all source-specific gates are disabled
//
// The component itself has no build-tag constraints. Capability differences
// across builds are handled transparently by the underlying launchers:
//   - container logs require the kubelet or docker build tag (no-op otherwise)
//   - journald logs (incl. the kubelet.service source) require the systemd
//     build tag (no-op otherwise)
//   - file logs are always supported
func NewComponent(deps Requires) (Provides, error) {
	obs, obsOk := deps.Observer.Get()
	wmeta, wmetaOk := deps.WMeta.Get()

	observerRequired := anomalydetectionconfig.ObserverRequired(deps.Config)
	logSourceSettings := newLogSourceSettings(deps.Config)
	recordingEnabled := anomalydetectionconfig.RecordingEnabled(deps.Config)

	// Skip when the observer is absent, neither logs ingestion nor recording is
	// requested, or no enabled source can start.
	if !logSourceSettings.shouldStart(obsOk, wmetaOk, observerRequired, recordingEnabled) {
		return Provides{Comp: &logssourceComponent{}}, nil
	}
	containerSourcesActive := logSourceSettings.containerSourcesEnabled && wmetaOk

	observerHandle := obs.GetHandle("logs")

	const logsProcessingRulesKey = "anomaly_detection.logs.processing_rules"
	logsRules, err := logsfilter.LoadRules(deps.Config, logsProcessingRulesKey)
	if err != nil {
		deps.Log.Warnf("[observer/logssource] %s: invalid rules, proceeding without log filtering: %v", logsProcessingRulesKey, err)
		logsRules = &logsfilter.Rules{}
	}

	processingRules, err := logsconfig.GlobalProcessingRules(deps.Config)
	if err != nil {
		deps.Log.Warnf("observer logssource: invalid global processing rules, proceeding without them: %v", err)
		processingRules = nil
	}

	var pauseFilter workloadfilter.FilterBundle
	if fs, ok := deps.FilterStore.Get(); ok {
		pauseFilter = fs.GetContainerPausedFilters()
	}

	var samplerOnDropped func(source, priority string)
	if obsOk {
		samplerOnDropped = obs.RecordSamplerDropped
	}
	sampler := newLogSamplerFromConfig(deps.Config, samplerOnDropped)
	pipeline := newObserverPipeline(deps.Config, processingRules, deps.Hostname, observerHandle, sampler, logsRules)
	logSources := sources.NewLogSources()
	tracker := tailers.NewTailerTracker()
	launchersMgr := launchers.NewLaunchers(logSources, pipeline, deps.Auditor, tracker)

	var sp *sourceProvider
	var adMgr *adSourceManager
	var adScheduler schedulers.Scheduler

	if containerSourcesActive {
		fingerprintCfg, err := logsconfig.GlobalFingerprintConfig(deps.Config)
		if err != nil {
			deps.Log.Warnf("observer logssource: invalid fingerprint config, proceeding with defaults: %v", err)
			fingerprintCfg = &types.FingerprintConfig{}
		}
		fileOpener := opener.NewFileOpener()
		fileLauncher := filelauncher.NewLauncher(
			deps.Config.GetInt("logs_config.open_files_limit"),
			filelauncher.DefaultSleepDuration,
			deps.Config.GetBool("logs_config.validate_pod_container_id"),
			time.Duration(deps.Config.GetFloat64("logs_config.file_scan_period")*float64(time.Second)),
			deps.Config.GetString("logs_config.file_wildcard_selection_mode"),
			flare.NewFlareController(),
			deps.Tagger,
			fileOpener,
			fileTailer.NewFingerprinter(*fingerprintCfg, fileOpener),
		)
		launchersMgr.AddLauncher(fileLauncher)
		launchersMgr.AddLauncher(containerLauncher.NewLauncher(logSources, option.New(wmeta), deps.Tagger))

		sp = newSourceProvider(wmeta, logSources, pauseFilter)

		services := service.NewServices()
		adMgr = newADSourceManager(logSources, services, sp)

		if deps.Autodiscovery != nil {
			adScheduler = logsadscheduler.NewNamed(deps.Autodiscovery, "observer-logssource AD scheduler")
		}
	} else if logSourceSettings.containerSourcesEnabled {
		deps.Log.Debugf("[observer/logssource] container log sources not started: workloadmeta unavailable")
	}

	if containerSourcesActive || logSourceSettings.kubeletSourceEnabled {
		launchersMgr.AddLauncher(journaldlauncher.NewLauncher(flare.NewFlareController(), deps.Tagger))
	}
	if logSourceSettings.kubeletSourceEnabled {
		registerKubeletJournaldSource(logSources, deps.Log)
	}

	ctx, cancel := context.WithCancel(context.Background())

	deps.Lc.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			deps.Log.Infof("[observer/logssource] starting log pipeline")
			pipeline.start()
			launchersMgr.Start()
			if adScheduler != nil {
				adScheduler.Start(adMgr)
			}
			if sp != nil {
				sp.run(ctx)
			}
			return nil
		},
		OnStop: func(_ context.Context) error {
			// Shutdown ordering is load-bearing — do NOT reorder.
			// 1. Cancel context and wait for source provider to exit fully.
			cancel()
			if sp != nil {
				sp.wait()
			}
			// 2. Stop the AD scheduler so it does not add more sources.
			if adScheduler != nil {
				adScheduler.Stop()
			}
			// 3. Stop all tailers; blocks until the last message is written to inputChan.
			launchersMgr.Stop()
			// 4. Drain inputChan; proc writes surviving messages to outputChan then exits.
			pipeline.proc.Stop()
			// 5. Signal the drain goroutine to exit (safe: proc.Stop returned = no more writes).
			close(pipeline.outputChan)
			// 6. Wait for the drain goroutine to finish.
			<-pipeline.drainDone
			return nil
		},
	})

	return Provides{Comp: &logssourceComponent{}}, nil
}
