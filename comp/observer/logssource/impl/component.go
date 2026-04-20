// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || docker

// Package logssourceimpl implements the logssource component.
package logssourceimpl

import (
	"context"
	"time"

	"go.uber.org/fx"

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
	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	logssource "github.com/DataDog/datadog-agent/comp/observer/logssource/def"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	containerLauncher "github.com/DataDog/datadog-agent/pkg/logs/launchers/container"
	filelauncher "github.com/DataDog/datadog-agent/pkg/logs/launchers/file"
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

	Lc          fx.Lifecycle
	Log         log.Component
	Config      config.Component
	Hostname    hostname.Component
	WMeta       option.Option[workloadmeta.Component]
	Tagger      tagger.Component
	Auditor     auditor.Component
	Observer    option.Option[observer.Component]
	FilterStore option.Option[workloadfilter.Component]
}

// Provides defines the output of the logssource component.
type Provides struct {
	compdef.Out
	Comp logssource.Component
}

type logssourceComponent struct{}

// NewComponent creates the logssource component.
//
// The component is a no-op when any of these are true:
//   - the observer is unavailable
//   - workloadmeta is unavailable
//   - logs_enabled is true (anomaly detection is only for non-log-management customers)
func NewComponent(deps Requires) (Provides, error) {
	obs, obsOk := deps.Observer.Get()
	wmeta, wmetaOk := deps.WMeta.Get()

	if !obsOk || !wmetaOk || deps.Config.GetBool("logs_enabled") || deps.Config.GetBool("log_enabled") {
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
	sp := newSourceProvider(wmeta, logSources, pauseFilter)

	ctx, cancel := context.WithCancel(context.Background())

	deps.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			deps.Log.Infof("[observer/logssource] starting container log pipeline")
			pipeline.start()
			launchersMgr.Start()
			sp.run(ctx)
			return nil
		},
		OnStop: func(_ context.Context) error {
			// Shutdown ordering is load-bearing — do NOT reorder.
			// 1. Signal the source provider to stop, then wait for it to fully exit.
			//    Without the wait, handleSet could call AddSource on an unbuffered channel
			//    after the launcher has stopped reading — deadlock.
			cancel()
			sp.wait()
			// 2. Stop all tailers; blocks until the last message is written to inputChan.
			launchersMgr.Stop()
			// 3. Drain inputChan; proc writes surviving messages to outputChan then exits.
			pipeline.proc.Stop()
			// 4. Signal the drain goroutine to exit (safe: proc.Stop returned = no more writes).
			close(pipeline.outputChan)
			// 5. Wait for the drain goroutine to finish.
			<-pipeline.drainDone
			return nil
		},
	})

	return Provides{Comp: &logssourceComponent{}}, nil
}
