// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcclient

import (
	"regexp"
	"sync"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// TaskType contains the type of the remote config task to execute
type TaskType string

const (
	// TaskFlare is the task sent to request a flare from the agent
	TaskFlare TaskType = "flare"
)

const agentConfigOrderID = "configuration_order"

// matches datadog/<int>/<string>/<string>/<string> for datadog/<org_id>/<product>/<config_id>/<file>
var datadogPathRegexp = regexp.MustCompile(`^datadog/(\d+)/([^/]+)/([^/]+)/([^/]+)$`)

// RCAgentTaskListener is the FX-compatible listener, so RC can push updates through it
type RCAgentTaskListener func(taskType TaskType, task AgentTaskConfig) (bool, error)

// RCAgentConfigListener is the FX-compatible listener, so RC can push updates through it
type RCAgentConfigListener func(config ConfigContent) error

type rcClient struct {
	client        *remote.Client
	m             *sync.Mutex
	taskProcessed map[string]bool

	taskListeners   []RCAgentTaskListener
	configListeners []RCAgentConfigListener
}

type dependencies struct {
	fx.In

	TaskListeners   []RCAgentTaskListener   `group:"rCAgentTaskListener"`   // <-- Fill automatically by Fx
	ConfigListeners []RCAgentConfigListener `group:"rCAgentConfigListener"` // <-- Fill automatically by Fx
}

func newRemoteConfigClient(deps dependencies) (Component, error) {
	rc := rcClient{
		taskListeners:   deps.TaskListeners,
		configListeners: deps.ConfigListeners,
		m:               &sync.Mutex{},
		client:          nil,
	}

	return rc, nil
}

// Listen start the remote config client to listen to AGENT_TASK configurations
func (rc rcClient) Listen() error {
	c, err := remote.NewUnverifiedGRPCClient(
		"core-agent", version.AgentVersion, []data.Product{
			data.ProductAgentTask,
			data.ProductAgentConfig,
		}, 1*time.Second,
	)
	if err != nil {
		return err
	}

	rc.client = c
	rc.taskProcessed = map[string]bool{}

	rc.client.Subscribe(state.ProductAgentTask, rc.agentTaskUpdateCallback)
	rc.client.Subscribe(state.ProductAgentConfig, rc.agentConfigUpdateCallback)

	rc.client.Start()

	return nil
}

// agentConfigUpdateCallback is the callback function called when there is an AGENT_CONFIG config update
// The RCClient can directly call back listeners, because there would be no way to send back
// RCTE2 configuration applied state to RC backend.
func (rc rcClient) agentConfigUpdateCallback(updates map[string]state.RawConfig) {
	var orderFile AgentConfigOrder
	parsedLayers := map[string]AgentConfig{}
	hasError := false

	for configPath, c := range updates {
		parsedConfigPath, err := data.ParseConfigPath(configPath)
		if err != nil {
			hasError = true
			rc.client.UpdateApplyStatus(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			// If a layer is wrong, fail later to parse the rest and check them all
			continue
		}

		// Ignore the configuration order file
		if parsedConfigPath.ConfigID == agentConfigOrderID {
			orderFile, err = parseConfigAgentConfigOrder(c.Config, c.Metadata)
			if err != nil {
				hasError = true
				rc.client.UpdateApplyStatus(configPath, state.ApplyStatus{
					State: state.ApplyStateError,
					Error: err.Error(),
				})
				// If a layer is wrong, fail later to parse the rest and check them all
				continue
			}
		} else {
			cfg, err := parseConfigAgentConfig(c.Config, c.Metadata)
			if err != nil {
				hasError = true
				rc.client.UpdateApplyStatus(configPath, state.ApplyStatus{
					State: state.ApplyStateError,
					Error: err.Error(),
				})
				// If a layer is wrong, fail later to parse the rest and check them all
				continue
			}
			parsedLayers[parsedConfigPath.ConfigID] = cfg
		}
	}

	// If there was at least one error, don't apply any config
	if hasError || (len(orderFile.Config.Order) == 0 && len(orderFile.Config.InternalOrder) == 0) {
		return
	}

	// Go through all the layers that were sent, and apply them one by one to the merged structure
	mergedConfig := ConfigContent{}
	for i := len(orderFile.Config.Order) - 1; i >= 0; i-- {
		if layer, found := parsedLayers[orderFile.Config.Order[i]]; found {
			mergedConfig.LogLevel = layer.Config.Config.LogLevel
		}
	}
	// Same for internal config
	for i := len(orderFile.Config.InternalOrder) - 1; i >= 0; i-- {
		if layer, found := parsedLayers[orderFile.Config.InternalOrder[i]]; found {
			mergedConfig.LogLevel = layer.Config.Config.LogLevel
		}
	}

	log.Warnf("[RCM] Merged config %+v", mergedConfig)

	if len(mergedConfig.LogLevel) > 0 {
		settings.SetRuntimeSetting("log_level", mergedConfig.LogLevel)
	}

	// Call all the listeners to the config change
	var err error
	for _, l := range rc.configListeners {
		oneErr := l(mergedConfig)
		if oneErr != nil {
			err = errors.Wrap(err, oneErr.Error())
		}
	}
}

// agentTaskUpdateCallback is the callback function called when there is an AGENT_TASK config update
// The RCClient can directly call back listeners, because there would be no way to send back
// RCTE2 configuration applied state to RC backend.
func (rc rcClient) agentTaskUpdateCallback(updates map[string]state.RawConfig) {
	rc.m.Lock()
	defer rc.m.Unlock()
	for configPath, c := range updates {
		task, err := parseConfigAgentTask(c.Config, c.Metadata)
		if err != nil {
			rc.client.UpdateApplyStatus(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}

		// Check that the flare task wasn't already processed
		if !rc.taskProcessed[task.Config.UUID] {
			rc.taskProcessed[task.Config.UUID] = true

			// Mark it as unack first
			rc.client.UpdateApplyStatus(configPath, state.ApplyStatus{
				State: state.ApplyStateUnacknowledged,
			})

			var err error
			var processed bool
			// Call all the listeners component
			for _, l := range rc.taskListeners {
				oneProcessed, oneErr := l(TaskType(task.Config.TaskType), task)
				// Check if the task was processed at least once
				processed = oneProcessed || processed
				if oneErr != nil {
					err = errors.Wrap(err, oneErr.Error())
				}
			}
			if processed && err != nil {
				// One failure
				rc.client.UpdateApplyStatus(configPath, state.ApplyStatus{
					State: state.ApplyStateError,
					Error: err.Error(),
				})
			} else if processed && err == nil {
				// Only success
				rc.client.UpdateApplyStatus(configPath, state.ApplyStatus{
					State: state.ApplyStateAcknowledged,
				})
			} else {
				rc.client.UpdateApplyStatus(configPath, state.ApplyStatus{
					State: state.ApplyStateUnknown,
				})
			}
		}
	}
}

// TaskListenerProvider defines component that can receive RC updates
type TaskListenerProvider struct {
	fx.Out

	Listener RCAgentTaskListener `group:"rCAgentTaskListener"`
}

// ConfigListenerProvider defines component that can receive RC updates
type ConfigListenerProvider struct {
	fx.Out

	Listener RCAgentTaskListener `group:"rCAgentConfigListener"`
}
