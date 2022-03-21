// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"context"
	"time"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/channel"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/container"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/docker"
	filelauncher "github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/journald"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/listener"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/traps"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/windowsevent"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// Agent represents the data pipeline that collects, decodes,
// processes and sends logs to the backend.  See the package README for
// a description of its operation.
type Agent struct {
	sources                   *config.LogSources
	services                  *service.Services
	schedulers                *schedulers.Schedulers
	auditor                   auditor.Auditor
	destinationsCtx           *client.DestinationsContext
	pipelineProvider          pipeline.Provider
	inputs                    []startstop.StartStoppable
	health                    *health.Handle
	diagnosticMessageReceiver *diagnostic.BufferedMessageReceiver

	// started is true if the agent has ever been started
	started bool
}

// NewAgent returns a new Logs Agent
func NewAgent(sources *config.LogSources, services *service.Services, processingRules []*config.ProcessingRule, endpoints *config.Endpoints) *Agent {
	health := health.RegisterLiveness("logs-agent")

	// setup the auditor
	// We pass the health handle to the auditor because it's the end of the pipeline and the most
	// critical part. Arguably it could also be plugged to the destination.
	auditorTTL := time.Duration(coreConfig.Datadog.GetInt("logs_config.auditor_ttl")) * time.Hour
	auditor := auditor.New(coreConfig.Datadog.GetString("logs_config.run_path"), auditor.DefaultRegistryFilename, auditorTTL, health)
	destinationsCtx := client.NewDestinationsContext()
	diagnosticMessageReceiver := diagnostic.NewBufferedMessageReceiver()

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(config.NumberOfPipelines, auditor, diagnosticMessageReceiver, processingRules, endpoints, destinationsCtx)

	containerLaunchables := []container.Launchable{
		{
			IsAvailable: docker.IsAvailable,
			Launcher: func() startstop.StartStoppable {
				return docker.NewLauncher(
					time.Duration(coreConfig.Datadog.GetInt("logs_config.docker_client_read_timeout"))*time.Second,
					sources,
					services,
					pipelineProvider,
					auditor,
					coreConfig.Datadog.GetBool("logs_config.docker_container_use_file"),
					coreConfig.Datadog.GetBool("logs_config.docker_container_force_use_file"))
			},
		},
		{
			IsAvailable: kubernetes.IsAvailable,
			Launcher: func() startstop.StartStoppable {
				return kubernetes.NewLauncher(sources, services, coreConfig.Datadog.GetBool("logs_config.container_collect_all"))
			},
		},
	}

	// when k8s_container_use_file is true, always attempt to use the kubernetes launcher first
	if coreConfig.Datadog.GetBool("logs_config.k8s_container_use_file") {
		containerLaunchables[0], containerLaunchables[1] = containerLaunchables[1], containerLaunchables[0]
	}

	validatePodContainerID := coreConfig.Datadog.GetBool("logs_config.validate_pod_container_id")

	// setup the inputs
	inputs := []startstop.StartStoppable{
		filelauncher.NewLauncher(sources, coreConfig.Datadog.GetInt("logs_config.open_files_limit"), pipelineProvider, auditor,
			filelauncher.DefaultSleepDuration, validatePodContainerID, time.Duration(coreConfig.Datadog.GetFloat64("logs_config.file_scan_period")*float64(time.Second))),
		listener.NewLauncher(sources, coreConfig.Datadog.GetInt("logs_config.frame_size"), pipelineProvider),
		journald.NewLauncher(sources, pipelineProvider, auditor),
		windowsevent.NewLauncher(sources, pipelineProvider),
		traps.NewLauncher(sources, pipelineProvider),
	}

	// Only try to start the container launchers if Docker or Kubernetes is available
	if coreConfig.IsFeaturePresent(coreConfig.Docker) || coreConfig.IsFeaturePresent(coreConfig.Kubernetes) {
		inputs = append(inputs, container.NewLauncher(containerLaunchables))
	}

	return &Agent{
		sources:                   sources,
		services:                  services,
		schedulers:                schedulers.NewSchedulers(sources, services),
		auditor:                   auditor,
		destinationsCtx:           destinationsCtx,
		pipelineProvider:          pipelineProvider,
		inputs:                    inputs,
		health:                    health,
		diagnosticMessageReceiver: diagnosticMessageReceiver,
	}
}

// NewServerless returns a Logs Agent instance to run in a serverless environment.
// The Serverless Logs Agent has only one input being the channel to receive the logs to process.
// It is using a NullAuditor because we've nothing to do after having sent the logs to the intake.
func NewServerless(sources *config.LogSources, services *service.Services, processingRules []*config.ProcessingRule, endpoints *config.Endpoints) *Agent {
	health := health.RegisterLiveness("logs-agent")

	diagnosticMessageReceiver := diagnostic.NewBufferedMessageReceiver()

	// setup the a null auditor, not tracking data in any registry
	auditor := auditor.NewNullAuditor()
	destinationsCtx := client.NewDestinationsContext()

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewServerlessProvider(config.NumberOfPipelines, auditor, processingRules, endpoints, destinationsCtx)

	// setup the inputs
	inputs := []startstop.StartStoppable{
		channel.NewLauncher(sources, pipelineProvider),
	}

	return &Agent{
		sources:                   sources,
		services:                  services,
		schedulers:                schedulers.NewSchedulers(sources, services),
		auditor:                   auditor,
		destinationsCtx:           destinationsCtx,
		pipelineProvider:          pipelineProvider,
		inputs:                    inputs,
		health:                    health,
		diagnosticMessageReceiver: diagnosticMessageReceiver,
	}
}

// Start starts all the elements of the data pipeline
// in the right order to prevent data loss
func (a *Agent) Start() {
	if a.started {
		panic("logs agent cannot be started more than once")
	}
	a.started = true

	inputs := startstop.NewStarter()
	for _, input := range a.inputs {
		inputs.Add(input)
	}
	starter := startstop.NewStarter(
		a.destinationsCtx,
		a.auditor,
		a.pipelineProvider,
		a.diagnosticMessageReceiver,
		inputs,
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
	inputs := startstop.NewParallelStopper()
	for _, input := range a.inputs {
		inputs.Add(input)
	}
	stopper := startstop.NewSerialStopper(
		a.schedulers,
		inputs,
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
