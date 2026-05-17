// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logssourceimpl implements the logssource component.
package logssourceimpl

import (
	"context"
	"time"

	logssource "github.com/DataDog/datadog-agent/comp/anomalydetection/logssource/def"
	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
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
	// and the observer falls back to generic container log collection only.
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
// setting it to false disables both container/AD logs (this component) and
// agent-internal logs (observer's agent_logs tap). Defaults to false.
// anomaly_detection.agent_logs.enabled additionally controls the agent-internal
// log tap and defaults to true when logs.enabled is true.
//
// The component is a no-op when any of these are true:
//   - the observer is unavailable
//   - workloadmeta is unavailable
//   - anomaly_detection.enabled is false and anomaly_detection.recording.enabled is false
//   - anomaly_detection.logs.enabled is false and anomaly_detection.recording.enabled is false
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

	analysisEnabled := deps.Config.GetBool("anomaly_detection.enabled")
	logsEnabled := !deps.Config.IsConfigured("anomaly_detection.logs.enabled") || deps.Config.GetBool("anomaly_detection.logs.enabled")
	recordingEnabled := deps.Config.GetBool("anomaly_detection.recording.enabled")

	// Skip when the observer is absent, workloadmeta is absent,
	// or neither logs ingestion nor recording is requested.
	if !obsOk || !wmetaOk || (!logsEnabled && !recordingEnabled) || (!analysisEnabled && !recordingEnabled) {
		return Provides{Comp: &logssourceComponent{}}, nil
	}

	observerHandle := obs.GetHandle("logs")

	processingRules, err := logsconfig.GlobalProcessingRules(deps.Config)
	if err != nil {
		deps.Log.Warnf("observer logssource: invalid global processing rules, proceeding without them: %v", err)
		processingRules = nil
	}

	var pauseFilter workloadfilter.FilterBundle
	if fs, ok := deps.FilterStore.Get(); ok {
		pauseFilter = fs.GetContainerPausedFilters()
	}

	pipeline := newObserverPipeline(deps.Config, processingRules, deps.Hostname, observerHandle)
	logSources := sources.NewLogSources()
	tracker := tailers.NewTailerTracker()

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

	launcher := containerLauncher.NewLauncher(logSources, option.New(wmeta), deps.Tagger)
	launchersMgr := launchers.NewLaunchers(logSources, pipeline, deps.Auditor, tracker)
	launchersMgr.AddLauncher(fileLauncher)
	launchersMgr.AddLauncher(launcher)
	launchersMgr.AddLauncher(journaldlauncher.NewLauncher(flare.NewFlareController(), deps.Tagger, option.None[storedef.Component]()))

	registerKubeletJournaldSource(logSources, deps.Log)

	sp := newSourceProvider(wmeta, logSources, pauseFilter)

	services := service.NewServices()
	adMgr := newADSourceManager(logSources, services, sp)

	var adScheduler schedulers.Scheduler
	if deps.Autodiscovery != nil {
		adScheduler = logsadscheduler.NewNamed(deps.Autodiscovery, "observer-logssource AD scheduler")
	}

	ctx, cancel := context.WithCancel(context.Background())

	deps.Lc.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			deps.Log.Infof("[observer/logssource] starting container log pipeline")
			pipeline.start()
			launchersMgr.Start()
			if adScheduler != nil {
				adScheduler.Start(adMgr)
			}
			sp.run(ctx)
			return nil
		},
		OnStop: func(_ context.Context) error {
			// Shutdown ordering is load-bearing — do NOT reorder.
			// 1. Cancel context and wait for source provider to exit fully.
			cancel()
			sp.wait()
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
