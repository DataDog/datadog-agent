// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package locallogtailerimpl implements the locallogtailer component.
package locallogtailerimpl

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	auditornoop "github.com/DataDog/datadog-agent/comp/logs/auditor/impl-none"
	locallogtailerdef "github.com/DataDog/datadog-agent/comp/logs/locallogtailer/def"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	filelauncher "github.com/DataDog/datadog-agent/pkg/logs/launchers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	logsadscheduler "github.com/DataDog/datadog-agent/pkg/logs/schedulers/ad"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires defines the dependencies for the localreader component.
type Requires struct {
	Lifecycle     compdef.Lifecycle
	Config        configComponent.Component
	Log           log.Component
	Autodiscovery autodiscovery.Component
	Observer      option.Option[observerdef.Component]
}

// Provides defines the output of the locallogtailer component.
type Provides struct {
	Comp locallogtailerdef.Component
}

// localLogTailer is the self-contained log tailing pipeline for the observer.
// It discovers log sources via AutoDiscovery, tails files locally, and
// forwards processed messages to the observer without shipping to the backend.
type localLogTailer struct {
	config         configComponent.Component
	log            log.Component
	ad             autodiscovery.Component
	observerHandle observerdef.Handle

	logSources       *sources.LogSources
	schedulersMgr    *schedulers.Schedulers
	pipelineProvider pipeline.Provider
	lnchrs           *launchers.Launchers
	outputChan       chan *message.Message
	stopCh           chan struct{}
}

// NewComponent creates a new locallogtailer component.
func NewComponent(reqs Requires) (Provides, error) {
	obs, ok := reqs.Observer.Get()
	if !ok || !reqs.Config.GetBool("observer.enabled") {
		return Provides{Comp: &noopLocalLogTailer{}}, nil
	}

	lr := &localLogTailer{
		config:         reqs.Config,
		log:            reqs.Log,
		ad:             reqs.Autodiscovery,
		observerHandle: obs.GetHandle("locallogtailer"),
		stopCh:         make(chan struct{}),
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: lr.start,
		OnStop:  lr.stop,
	})

	return Provides{Comp: lr}, nil
}

func (lr *localLogTailer) start(_ context.Context) error {
	processingRules, err := config.GlobalProcessingRules(lr.config)
	if err != nil {
		return err
	}

	fingerprintConfig, err := config.GlobalFingerprintConfig(lr.config)
	if err != nil {
		return err
	}

	diagnosticMessageReceiver := diagnostic.NewBufferedMessageReceiver(nil, nil)
	lr.pipelineProvider = pipeline.NewProcessorOnlyProvider(diagnosticMessageReceiver, processingRules, nil, lr.config)
	lr.pipelineProvider.Start()

	lr.logSources = sources.NewLogSources()
	auditor := auditornoop.NewAuditor()
	tracker := tailers.NewTailerTracker()

	fileLimits := lr.config.GetInt("logs_config.open_files_limit")
	fileValidatePodContainer := lr.config.GetBool("logs_config.validate_pod_container_id")
	fileScanPeriod := time.Duration(lr.config.GetFloat64("logs_config.file_scan_period") * float64(time.Second))
	fileWildcardSelectionMode := lr.config.GetString("logs_config.file_wildcard_selection_mode")

	fileOpener := opener.NewFileOpener()
	fingerprinter := file.NewFingerprinter(*fingerprintConfig, fileOpener)

	fileLauncher := filelauncher.NewLauncher(
		fileLimits,
		filelauncher.DefaultSleepDuration,
		fileValidatePodContainer,
		fileScanPeriod,
		fileWildcardSelectionMode,
		flare.NewFlareController(),
		nil,
		fileOpener,
		fingerprinter,
	)

	lr.lnchrs = launchers.NewLaunchers(lr.logSources, lr.pipelineProvider, auditor, tracker)
	lr.lnchrs.AddLauncher(fileLauncher)
	lr.lnchrs.Start()

	// Wire AutoDiscovery to log sources via a dedicated scheduler (not the
	// shared log-agent-scheduler Fx group used by the shipping logs agent).
	adScheduler := logsadscheduler.New(lr.ad)
	lr.schedulersMgr = schedulers.NewSchedulers(lr.logSources, nil)
	lr.schedulersMgr.AddScheduler(adScheduler)
	lr.schedulersMgr.Start()

	lr.outputChan = lr.pipelineProvider.GetOutputChan()
	go lr.drainOutputChan()

	return nil
}

func (lr *localLogTailer) stop(_ context.Context) error {
	close(lr.stopCh)
	lr.schedulersMgr.Stop()
	lr.lnchrs.Stop()
	lr.pipelineProvider.Stop()
	return nil
}

// drainOutputChan reads messages from the pipeline output channel and forwards
// them to the observer. It exits cleanly when the channel is closed or stopCh
// is signalled, draining remaining messages before exiting on stop.
func (lr *localLogTailer) drainOutputChan() {
	for {
		select {
		case msg, ok := <-lr.outputChan:
			if !ok {
				return
			}
			lr.observerHandle.ObserveLog(msg)
		case <-lr.stopCh:
			for {
				select {
				case msg, ok := <-lr.outputChan:
					if !ok {
						return
					}
					lr.observerHandle.ObserveLog(msg)
				default:
					return
				}
			}
		}
	}
}

// noopLocalLogTailer is returned when the observer is disabled.
type noopLocalLogTailer struct{}
