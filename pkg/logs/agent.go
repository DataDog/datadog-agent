// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"context"
	"time"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// Agent represents the data pipeline that collects, decodes,
// processes and sends logs to the backend.  See the package README for
// a description of its operation.
type Agent struct {
	sources                   *sources.LogSources
	services                  *service.Services
	schedulers                *schedulers.Schedulers
	auditor                   auditor.Auditor
	destinationsCtx           *client.DestinationsContext
	pipelineProvider          pipeline.Provider
	launchers                 *launchers.Launchers
	health                    *health.Handle
	diagnosticMessageReceiver *diagnostic.BufferedMessageReceiver

	// started is true if the agent has ever been started
	started bool
}

// Start starts all the elements of the data pipeline
// in the right order to prevent data loss
func (a *Agent) Start() {
	if a.started {
		panic("logs agent cannot be started more than once")
	}
	a.started = true

	starter := startstop.NewStarter(
		a.destinationsCtx,
		a.auditor,
		a.pipelineProvider,
		a.diagnosticMessageReceiver,
		a.launchers,
		a.schedulers,
	)
	starter.Start()
}

// Flush flushes synchronously the pipelines managed by the Logs Agent.
func (a *Agent) Flush(ctx context.Context) {
	a.pipelineProvider.Flush(ctx)
}

// Stop stops all the elements of the data pipeline
// in the right order to prevent data loss
func (a *Agent) Stop() {
	stopper := startstop.NewSerialStopper(
		a.schedulers,
		a.launchers,
		a.pipelineProvider,
		a.auditor,
		a.destinationsCtx,
		a.diagnosticMessageReceiver,
	)

	// This will try to stop everything in order, including the potentially blocking
	// parts like the sender. After StopTimeout it will just stop the last part of the
	// pipeline, disconnecting it from the auditor, to make sure that the pipeline is
	// flushed before stopping.
	// TODO: Add this feature in the stopper.
	c := make(chan struct{})
	go func() {
		stopper.Stop()
		close(c)
	}()
	timeout := time.Duration(coreConfig.Datadog.GetInt("logs_config.stop_grace_period")) * time.Second
	select {
	case <-c:
	case <-time.After(timeout):
		log.Info("Timed out when stopping logs-agent, forcing it to stop now")
		// We force all destinations to read/flush all the messages they get without
		// trying to write to the network.
		a.destinationsCtx.Stop()
		// Wait again for the stopper to complete.
		// In some situation, the stopper unfortunately never succeed to complete,
		// we've already reached the grace period, give it some more seconds and
		// then force quit.
		timeout := time.NewTimer(5 * time.Second)
		select {
		case <-c:
		case <-timeout.C:
			log.Warn("Force close of the Logs Agent, dumping the Go routines.")
			if stack, err := util.GetGoRoutinesDump(); err != nil {
				log.Warnf("can't get the Go routines dump: %s\n", err)
			} else {
				log.Warn(stack)
			}
		}
	}
}

// AddScheduler adds the given scheduler to the agent.
func (a *Agent) AddScheduler(scheduler schedulers.Scheduler) {
	a.schedulers.AddScheduler(scheduler)
}
