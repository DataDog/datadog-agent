// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package remoteagentregistryimpl implements the remoteagentregistry component interface
package remoteagentregistryimpl

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	remoteagentregistryStatus "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/status"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires defines the dependencies for the remoteagentregistry component
type Requires struct {
	Config    config.Component
	Ipc       ipc.Component
	Lifecycle compdef.Lifecycle
	Telemetry telemetry.Component
}

// Provides defines the output of the remoteagentregistry component
type Provides struct {
	Comp          remoteagentregistry.Component
	FlareProvider flaretypes.Provider
	Status        status.InformationProvider
}

// NewComponent creates a new remoteagent component
func NewComponent(reqs Requires) Provides {
	enabled := reqs.Config.GetBool("remote_agent_registry.enabled")
	if !enabled {
		return Provides{}
	}

	registry := newRegistry(reqs)

	return Provides{
		Comp:          registry,
		FlareProvider: flaretypes.NewProvider(registry.fillFlare),
		Status:        status.NewInformationProvider(remoteagentregistryStatus.GetProvider(registry)),
	}
}

func newRegistry(reqs Requires) *remoteAgentRegistry {
	shutdownChan := make(chan struct{})
	registry := &remoteAgentRegistry{
		conf:           reqs.Config,
		ipc:            reqs.Ipc,
		agentMap:       make(map[string]*remoteAgentClient),
		shutdownChan:   shutdownChan,
		telemetry:      reqs.Telemetry,
		telemetryStore: newTelemetryStore(reqs.Telemetry),
		// Services currently supported by the remote agent registry
		remoteAgentServices: map[remoteAgentServiceName]struct{}{
			StatusServiceName:    {},
			FlareServiceName:     {},
			TelemetryServiceName: {},
		},
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			go registry.start()
			return nil
		},
		OnStop: func(context.Context) error {
			shutdownChan <- struct{}{}
			return nil
		},
	})

	return registry
}

type telemetryStore struct {
	// remoteAgentRegistered tracks how many remote agents are registered.
	remoteAgentRegistered telemetry.Gauge
	// remoteAgentRegisteredError tracks how many remote agents failed to register.
	remoteAgentRegisteredError telemetry.Counter
	// remoteAgentUpdated tracks how many remote agents are updated.
	remoteAgentUpdated telemetry.Counter
	// remoteAgentUpdatedError tracks how many remote agents failed to update.
	remoteAgentUpdatedError telemetry.Counter
	// remoteAgentActionError tracks the number of errors encountered while performing actions on the remote agent registry.
	remoteAgentActionError telemetry.Counter
	// remoteAgentActionDuration tracks the duration of actions performed on the remote agent registry.
	remoteAgentActionDuration telemetry.Histogram
	// remoteAgentActionTimeout tracks the number of times an action on the remote agent registry timed out.
	remoteAgentActionTimeout telemetry.Counter
}

const (
	internalTelemetryNamespace = "remote_agent_registry"
	sessionIDMismatch          = "SESSION_ID_MISMATCH"
)

func newTelemetryStore(telemetryComp telemetry.Component) *telemetryStore {
	return &telemetryStore{
		remoteAgentRegistered: telemetryComp.NewGaugeWithOpts(
			internalTelemetryNamespace,
			"registered",
			[]string{"name"},
			"Number of remote agents registered in the remote agent registry.",
			telemetry.Options{NoDoubleUnderscoreSep: true},
		),
		remoteAgentRegisteredError: telemetryComp.NewCounterWithOpts(
			internalTelemetryNamespace,
			"registered_error",
			[]string{"name"},
			"Number of remote agents that failed to register in the remote agent registry.",
			telemetry.Options{NoDoubleUnderscoreSep: true},
		),
		remoteAgentUpdated: telemetryComp.NewCounterWithOpts(
			internalTelemetryNamespace,
			"updated",
			[]string{"name"},
			"Number of remote agents updated in the remote agent registry.",
			telemetry.Options{NoDoubleUnderscoreSep: true},
		),
		remoteAgentUpdatedError: telemetryComp.NewCounterWithOpts(
			internalTelemetryNamespace,
			"updated_error",
			[]string{"name"},
			"Number of remote agents that failed to update in the remote agent registry.",
			telemetry.Options{NoDoubleUnderscoreSep: true},
		),
		remoteAgentActionDuration: telemetryComp.NewHistogramWithOpts(
			internalTelemetryNamespace,
			"action_duration_seconds",
			[]string{"name", "action"},
			"Duration of actions performed on the remote agent registry.",
			// The default prometheus buckets are adapted to measure response time of network services
			prometheus.DefBuckets,
			telemetry.Options{NoDoubleUnderscoreSep: true},
		),
		remoteAgentActionError: telemetryComp.NewCounterWithOpts(
			internalTelemetryNamespace,
			"action_error",
			[]string{"name", "action", "error"},
			"Number of errors encountered while performing actions on the remote agent registry.",
			telemetry.Options{NoDoubleUnderscoreSep: true},
		),
		remoteAgentActionTimeout: telemetryComp.NewCounterWithOpts(
			internalTelemetryNamespace,
			"action_timeout",
			[]string{"action"},
			"Number of times an action on the remote agent registry timed out.",
			telemetry.Options{NoDoubleUnderscoreSep: true},
		),
	}
}

// remoteAgentRegistry is the main registry for remote agents. It tracks which remote agents are currently registered, when
// they were last seen, and handles collecting status and flare data from them on request.
type remoteAgentRegistry struct {
	conf           config.Component
	ipc            ipc.Component
	agentMap       map[string]*remoteAgentClient
	agentMapMu     sync.Mutex
	shutdownChan   chan struct{}
	telemetry      telemetry.Component
	telemetryStore *telemetryStore

	// Define the services that the remote agent supports
	remoteAgentServices map[remoteAgentServiceName]struct{}
}

// RegisterRemoteAgent registers a remote agent with the registry.
//
// It returns the session ID, the recommended refresh interval, and an error if the registration fails.
func (ra *remoteAgentRegistry) RegisterRemoteAgent(registration *remoteagentregistry.RegistrationData) (string, uint32, error) {
	recommendedRefreshInterval := uint32(ra.conf.GetDuration("remote_agent_registry.recommended_refresh_interval").Seconds())

	ra.agentMapMu.Lock()
	defer ra.agentMapMu.Unlock()

	// create the new remote Agent instance abnd fetch availale services
	remoteAgentClient, err := ra.newRemoteAgentClient(registration)
	if err != nil {
		ra.telemetryStore.remoteAgentRegisteredError.Inc(sanitizeString(registration.AgentDisplayName))
		return "", 0, err
	}

	log.Infof("Remote agent '%s' (flavor: %s, session_id: %s) registered. (exposed services: %v)", remoteAgentClient.RegisteredAgent.DisplayName, remoteAgentClient.RegisteredAgent.Flavor, remoteAgentClient.RegisteredAgent.SessionID, remoteAgentClient.services)
	// indexing remoteAgent client by its sessionID
	ra.agentMap[remoteAgentClient.RegisteredAgent.SessionID] = remoteAgentClient
	ra.telemetryStore.remoteAgentRegistered.Inc(remoteAgentClient.RegisteredAgent.SanitizedDisplayName)

	return remoteAgentClient.RegisteredAgent.SessionID, recommendedRefreshInterval, nil
}

// RefreshRemoteAgent refreshes the last seen time of a remote agent.
//
// It returns true if the remote agent was found and refreshed, false otherwise.
func (ra *remoteAgentRegistry) RefreshRemoteAgent(sessionID string) bool {
	ra.agentMapMu.Lock()
	defer ra.agentMapMu.Unlock()

	agentClient, ok := ra.agentMap[sessionID]
	if !ok {
		return false
	}
	agentClient.RegisteredAgent.LastSeen = time.Now()
	return ok
}

// Start starts the remote agent registry, which periodically checks for idle remote agents and deregisters them.
func (ra *remoteAgentRegistry) start() {
	remoteAgentIdleTimeout := ra.conf.GetDuration("remote_agent_registry.idle_timeout")
	ra.registerCollector()

	go func() {
		log.Info("Remote Agent registry started.")

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ra.shutdownChan:
				log.Info("Remote Agent registry stopped.")
				return
			case <-ticker.C:
				ra.agentMapMu.Lock()

				agentsToRemove := make([]string, 0)
				for sessionID, details := range ra.agentMap {
					if time.Since(details.RegisteredAgent.LastSeen) > remoteAgentIdleTimeout || details.unhealthy {
						agentsToRemove = append(agentsToRemove, sessionID)
					}
				}

				for _, sessionID := range agentsToRemove {
					remoteAgentClient, ok := ra.agentMap[sessionID]
					if ok {
						if remoteAgentClient.unhealthy {
							log.Warnf("Remote agent '%s' deregistered: %v", remoteAgentClient.RegisteredAgent.DisplayName, remoteAgentClient.unhealthyReason)
						} else {
							log.Infof("Remote agent '%s' deregistered after being idle for %s.", remoteAgentClient.RegisteredAgent.DisplayName, remoteAgentIdleTimeout)
						}
						ra.telemetryStore.remoteAgentRegistered.Dec(remoteAgentClient.RegisteredAgent.SanitizedDisplayName)
						// close the remote agent client and remove it from the registry
						_ = remoteAgentClient.close()
						delete(ra.agentMap, sessionID)
					}
				}

				ra.agentMapMu.Unlock()
			}
		}
	}()
}

func (ra *remoteAgentRegistry) GetRegisteredAgents() []remoteagentregistry.RegisteredAgent {
	ra.agentMapMu.Lock()
	defer ra.agentMapMu.Unlock()

	agents := make([]remoteagentregistry.RegisteredAgent, 0, len(ra.agentMap))
	for _, details := range ra.agentMap {
		agents = append(agents, details.RegisteredAgent)
	}

	return agents
}

func grpcErrorMessage(err error) string {
	errorString := codes.Unknown.String()
	status, ok := grpcStatus.FromError(err)
	if ok {
		errorString = status.Code().String()
	}
	return errorString
}
