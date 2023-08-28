package state

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/secrets"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

type RemoteConfigScheduler struct {
	scheduler     *collector.CheckScheduler
	runningChecks []integration.Config
}

type agentIntegration struct {
	Name       string            `json:"name"`
	Instances  []json.RawMessage `json:"instances"`
	InitConfig json.RawMessage   `json:"init_config"`
}

// secretsDecrypt allows tests to intercept calls to secrets.Decrypt.
var secretsDecrypt = secrets.Decrypt

// NewRemoteConfigScheduler creates an instance of a remote config integration scheduler
func NewRemoteConfigScheduler() *RemoteConfigScheduler {
	return &RemoteConfigScheduler{
		runningChecks: make([]integration.Config, 0),
	}
}

// Start creates the remote-config scheduler
func (sc *RemoteConfigScheduler) Start(coll *collector.Collector) error {
	if sc.scheduler != nil {
		return fmt.Errorf("Remote-config scheduler is already initiated")
	}

	sc.scheduler = collector.InitCheckScheduler(coll, aggregator.GetSenderManager())
	return nil
}

// IntegrationScheduleCallback is called at every AGENT_INTEGRATIONS to schedule/unschedule integrations
func (sc *RemoteConfigScheduler) IntegrationScheduleCallback(updates map[string]RawConfig) {
	// Unschedule every integrations, even if they haven't changed
	sc.scheduler.Unschedule(sc.runningChecks)
	sc.runningChecks = make([]integration.Config, 0)

	// Now schedule everything
	for _, intg := range updates {
		var d agentIntegration
		err := json.Unmarshal(intg.Config, &d)
		if err != nil {
			pkglog.Errorf("Can't decode agent configuration provided by remote-config: %v", err)
		}

		configToSchedule := integration.Config{
			Name:       d.Name,
			Instances:  []integration.Data{},
			InitConfig: integration.Data(d.InitConfig),
		}
		for _, instance := range d.Instances {
			// Resolve the ENC[] configuration, and fetch the actual secret in the config backend
			decryptedInstance, err := secretsDecrypt(instance, d.Name)
			if err != nil {
				pkglog.Errorf("Couldn't decrypt remote-config integration %s secret: %s", d.Name, err)
				// TODO apply status
				continue
			}
			configToSchedule.Instances = append(configToSchedule.Instances, integration.Data(decryptedInstance))
		}

		scheduleErrs := sc.scheduler.ScheduleWithErrors(configToSchedule)
		pkglog.Infof("Scheduled %d instances of %s check with remote-config", len(d.Instances), d.Name)
		if len(scheduleErrs) == 0 {
			// TODO: apply state ok
		}
		sc.runningChecks = append(sc.runningChecks, configToSchedule)
	}
}
