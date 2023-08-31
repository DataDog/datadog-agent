// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package remoteconfig schedule and unschedule integrations through remote-config
// using the AGENT_INTEGRATIONS product
package remoteconfig

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/secrets"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// Scheduler is the structure used to run checks with RC
type Scheduler struct {
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

// NewScheduler creates an instance of a remote config integration scheduler
func NewScheduler(scheduler *collector.CheckScheduler) *Scheduler {
	return &Scheduler{
		runningChecks: make([]integration.Config, 0),
		scheduler:     scheduler,
	}
}

// IntegrationScheduleCallback is called at every AGENT_INTEGRATIONS to schedule/unschedule integrations
func (sc *Scheduler) IntegrationScheduleCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	if sc.scheduler == nil {
		pkglog.Debugf("Scheduler is not ready to start remote-config integrations")
		return
	}

	// Unschedule every integrations, even if they haven't changed
	sc.scheduler.Unschedule(sc.runningChecks)
	sc.runningChecks = make([]integration.Config, 0)

	// Now schedule everything
	for cfgPath, intg := range updates {
		var d agentIntegration
		err := json.Unmarshal(intg.Config, &d)
		if err != nil {
			pkglog.Errorf("Can't decode agent configuration provided by remote-config: %v", err)
		}

		shouldSchedule := true
		applyStateCallback(cfgPath, state.ApplyStatus{State: state.ApplyStateUnacknowledged})

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
				applyStateCallback(cfgPath, state.ApplyStatus{
					State: state.ApplyStateError,
					Error: err.Error(),
				})
				shouldSchedule = false
				continue
			}
			configToSchedule.Instances = append(configToSchedule.Instances, integration.Data(decryptedInstance))
		}

		// We either apply all the instances of the config or none, no partial apply
		if !shouldSchedule {
			break
		}

		// Schedule all the checks from this configuration
		err = sc.scheduler.ScheduleWithErrors(configToSchedule)
		pkglog.Infof("Scheduled %d instances of %s check with remote-config", len(d.Instances), d.Name)

		// Report errors
		if err != nil {
			pkglog.Errorf("There were error while scheduling remote-configuration checks: %v", err)
			applyStateCallback(cfgPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
		} else {
			applyStateCallback(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		}
		sc.runningChecks = append(sc.runningChecks, configToSchedule)
	}
}
