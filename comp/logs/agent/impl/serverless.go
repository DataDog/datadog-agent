// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"context"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log/impl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	kubehealthimpl "github.com/DataDog/datadog-agent/comp/logs-library/kubehealth/impl"
	agent "github.com/DataDog/datadog-agent/comp/logs/agent/def"
	flareController "github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	auditorimpl "github.com/DataDog/datadog-agent/comp/logs/auditor/impl"
	auditornoop "github.com/DataDog/datadog-agent/comp/logs/auditor/impl-none"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
)

// NewServerlessLogsAgent creates a new instance of the logs agent for
// serverless. useRegistryAuditor selects between the two auditors this
// caller base needs:
//
//   - true: a registry auditor that persists tailer offsets to
//     registry.json under logs_config.run_path, so a cold-start instance
//     (no registry yet) reads from the beginning and captures the app's
//     startup line, while a restart within the same instance resumes from
//     the persisted offset instead of re-reading it. See
//     cmd/serverless-init/log/log.go for the tailingMode this pairs with,
//     and cmd/serverless-init/main.go's preloadEarly for the run_path
//     default.
//   - false: a no-op auditor that persists nothing.
//
// This is an explicit parameter rather than always constructing the
// registry auditor because pkg/serverless/logs.SetupLogAgent, which calls
// into this function, is also imported directly by the datadog-lambda-extension
// repository outside this codebase. Defaulting to the registry auditor
// would make that external caller start writing registry.json to disk on
// its next dependency bump with no corresponding code change on its side.
// Requiring the bool forces every caller, in-repo or out-of-repo, to make
// that choice at compile time.
func NewServerlessLogsAgent(tagger tagger.Component, compression logscompression.Component, hostname hostnameinterface.Component, useRegistryAuditor bool) agent.ServerlessLogsAgent {

	log := logComponent.NewTemporaryLoggerWithoutInit()

	var registryOrNoopAuditor auditor.Component
	if useRegistryAuditor {
		registryOrNoopAuditor = auditorimpl.NewComponent(auditorimpl.Dependencies{
			Log:        log,
			Config:     pkgconfigsetup.Datadog(),
			KubeHealth: kubehealthimpl.NewComponent().Comp,
		}).Comp
	} else {
		registryOrNoopAuditor = auditornoop.NewAuditor()
	}

	logsAgent := &logAgent{
		log:     log,
		config:  pkgconfigsetup.Datadog(),
		started: atomic.NewUint32(0),

		auditor:         registryOrNoopAuditor,
		sources:         sources.NewLogSources(),
		services:        service.NewServices(),
		tracker:         tailers.NewTailerTracker(),
		flarecontroller: flareController.NewFlareController(),
		tagger:          tagger,
		compression:     compression,
		hostname:        hostname,
	}
	return logsAgent
}

func (a *logAgent) Start() error {
	return a.start(context.TODO())
}

func (a *logAgent) Stop() {
	_ = a.stop(context.TODO())
}

// Flush flushes synchronously the running instance of the Logs Agent.
// Use a WithTimeout context in order to have a flush that can be cancelled.
func (a *logAgent) Flush(ctx context.Context) {
	a.log.Info("Triggering a flush in the logs-agent")
	a.pipelineProvider.Flush(ctx)
	a.log.Debug("Flush in the logs-agent done.")
}

// DrainTailers stops the source launchers (file, channel, ...) so every
// tailer performs one final read to EOF - via Tailer.Stop(), which blocks
// until its decoder has flushed into the pipeline - before returning. The
// pipeline itself (pipelineProvider, auditor, destinationsCtx) is left
// running so the caller's subsequent Flush can still ship the drained
// messages. Runs the stop in a goroutine and bounds it with ctx so a stuck
// tailer cannot consume the whole shutdown budget.
func (a *logAgent) DrainTailers(ctx context.Context) {
	a.log.Info("Draining logs-agent tailers before flush")
	done := make(chan struct{})
	go func() {
		a.launchers.Stop()
		close(done)
	}()
	select {
	case <-done:
		a.log.Debug("Drain of logs-agent tailers done.")
	case <-ctx.Done():
		a.log.Warn("DrainTailers hit its deadline before all tailers finished")
	}
}
