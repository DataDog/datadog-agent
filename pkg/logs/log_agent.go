package logs

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/input/container"
	"github.com/DataDog/datadog-agent/pkg/logs/input/docker"
	"github.com/DataDog/datadog-agent/pkg/logs/input/file"
	"github.com/DataDog/datadog-agent/pkg/logs/input/journald"
	"github.com/DataDog/datadog-agent/pkg/logs/input/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/logs/input/listener"
	"github.com/DataDog/datadog-agent/pkg/logs/input/traps"
	"github.com/DataDog/datadog-agent/pkg/logs/input/windowsevent"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

// LogAgent represents the Logs Agent
type LogAgent struct {
}

// create returns a new Logs Agent
func (s *LogAgent) create(sources *config.LogSources, services *service.Services, processingRules []*config.ProcessingRule, endpoints *config.Endpoints) *Agent {
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
			Launcher: func() restart.Restartable {
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
			Launcher: func() restart.Restartable {
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
	inputs := []restart.Restartable{
		file.NewScanner(sources, coreConfig.Datadog.GetInt("logs_config.open_files_limit"), pipelineProvider, auditor,
			file.DefaultSleepDuration, validatePodContainerID, time.Duration(coreConfig.Datadog.GetFloat64("logs_config.file_scan_period")*float64(time.Second))),
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
		auditor:                   auditor,
		destinationsCtx:           destinationsCtx,
		pipelineProvider:          pipelineProvider,
		inputs:                    inputs,
		health:                    health,
		diagnosticMessageReceiver: diagnosticMessageReceiver,
	}
}

// Start starts an instance of the Logs Agent.
func Start(getAC func() *autodiscovery.AutoConfig) error {
	return start(getAC, false, nil, nil, &LogAgent{})
}
