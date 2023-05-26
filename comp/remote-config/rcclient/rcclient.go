package rcclient

import (
	"time"

	"github.com/pkg/errors"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// Component is the component type.
type Component interface {
	// Start the remote config client to listen to AGENT_TASK configurations
	Listen() error
}

// RCListener is the FX-compatible listener, so RC can push updates through it
type RCListener func(task state.AgentTaskConfig) (bool, error)

type rcClient struct {
	client        *remote.Client
	taskProcessed map[string]bool

	listeners []RCListener
}

type dependencies struct {
	fx.In

	Listeners []RCListener `group:"rCListener"` // <-- Fill automatically by Fx
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newRemoteConfig),
)

func newRemoteConfig(deps dependencies) (Component, error) {
	rc := rcClient{
		listeners: deps.Listeners,
		client:    nil,
	}

	return rc, nil
}

// Listen start the remote config client to listen to AGENT_TASK configurations
func (rc rcClient) Listen() error {
	c, err := remote.NewUnverifiedGRPCClient(
		"core-agent", version.AgentVersion, []data.Product{data.ProductAgentTask}, 1*time.Second,
	)
	if err != nil {
		return err
	}

	rc.client = c
	rc.taskProcessed = map[string]bool{}

	rc.client.RegisterAgentTaskUpdate(rc.agentTaskUpdateCallback)

	rc.client.Start()

	return nil
}

// agentTaskUpdateCallback is the callback function called when there is an AGENT_TASK config update
// The RCClient can directly call back listeners, because there would be no way to send back
// RCTE2 configuration applied state to RC backend.
func (rc rcClient) agentTaskUpdateCallback(updates map[string]state.AgentTaskConfig) {
	for configPath, c := range updates {
		// Check that the flare task wasn't already processed
		if !rc.taskProcessed[c.Config.UUID] {
			rc.taskProcessed[c.Config.UUID] = true

			// Mark it as unack first
			rc.client.UpdateApplyStatus(configPath, state.ApplyStatus{
				State: state.ApplyStateUnacknowledged,
			})

			var err error
			var processed bool
			// Call all the listeners component
			for _, l := range rc.listeners {
				oneProcessed, oneErr := l(c)
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

// ListenerProvider defines component that can receive RC updates
type ListenerProvider struct {
	fx.Out

	Listener RCListener `group:"rCListener"`
}
