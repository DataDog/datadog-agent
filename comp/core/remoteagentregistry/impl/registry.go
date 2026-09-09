// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package remoteagentregistryimpl implements the remoteagentregistry component interface
package remoteagentregistryimpl

import (
	"context"
	"fmt"
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
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires defines the dependencies for the remoteagentregistry component
type Requires struct {
	Config           config.Component
	Ipc              ipc.Component
	Lifecycle        compdef.Lifecycle
	Telemetry        telemetry.Component
	Secrets          secrets.Component
	EventSubscribers []*remoteagentregistry.EventSubscriber `group:"remoteAgentEventSubscriber"`
}

// Provides defines the output of the remoteagentregistry component
type Provides struct {
	Comp          remoteagentregistry.Component
	FlareProvider flaretypes.Provider
	Status        status.InformationProvider
}

// NewComponent creates a new remoteagent component
func NewComponent(reqs Requires) Provides {
	enabled := reqs.Config.GetBool("remote_agent.registry.enabled")
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
	eventSubscribers := append([]*remoteagentregistry.EventSubscriber{}, reqs.EventSubscribers...)
	eventSubscribers = append(eventSubscribers, newSecretsRefreshEventSubscriber(reqs.Secrets))
	registry := &remoteAgentRegistry{
		conf:             reqs.Config,
		ipc:              reqs.Ipc,
		agentMap:         make(map[string]*remoteAgentClient),
		shutdownChan:     shutdownChan,
		telemetry:        reqs.Telemetry,
		telemetryStore:   newTelemetryStore(reqs.Telemetry),
		eventSubscribers: eventSubscribers,
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			registry.start()
			return nil
		},
		OnStop: func(context.Context) error {
			shutdownChan <- struct{}{}
			return nil
		},
	})

	return registry
}

// newSecretsRefreshEventSubscriber creates the subscriber that asks the secrets component to refresh when a remote
// agent reports that its API key was rejected. Refresh is asynchronous and applies its own configured throttle.
func newSecretsRefreshEventSubscriber(resolver secrets.Component) *remoteagentregistry.EventSubscriber {
	return &remoteagentregistry.EventSubscriber{
		Name: "secrets-refresh",
		Callback: func(_ remoteagentregistry.RegisteredAgent, events []remoteagentregistry.RemoteAgentEvent) {
			for _, event := range events {
				if _, ok := event.Details.(*remoteagentregistry.InvalidAPIKey); ok {
					// One refresh is sufficient for the whole report; the resolver coalesces and throttles requests.
					resolver.Refresh()
					return
				}
			}
		},
	}
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
			[]string{"remote_agent_name"},
			"Number of remote agents registered in the remote agent registry.",
			telemetry.Options{NoDoubleUnderscoreSep: true},
		),
		remoteAgentRegisteredError: telemetryComp.NewCounterWithOpts(
			internalTelemetryNamespace,
			"registered_error",
			[]string{"remote_agent_name"},
			"Number of remote agents that failed to register in the remote agent registry.",
			telemetry.Options{NoDoubleUnderscoreSep: true},
		),
		remoteAgentUpdated: telemetryComp.NewCounterWithOpts(
			internalTelemetryNamespace,
			"updated",
			[]string{"remote_agent_name"},
			"Number of remote agents updated in the remote agent registry.",
			telemetry.Options{NoDoubleUnderscoreSep: true},
		),
		remoteAgentUpdatedError: telemetryComp.NewCounterWithOpts(
			internalTelemetryNamespace,
			"updated_error",
			[]string{"remote_agent_name"},
			"Number of remote agents that failed to update in the remote agent registry.",
			telemetry.Options{NoDoubleUnderscoreSep: true},
		),
		remoteAgentActionDuration: telemetryComp.NewHistogramWithOpts(
			internalTelemetryNamespace,
			"action_duration_seconds",
			[]string{"remote_agent_name", "action"},
			"Duration of actions performed on the remote agent registry.",
			// The default prometheus buckets are adapted to measure response time of network services
			prometheus.DefBuckets,
			telemetry.Options{NoDoubleUnderscoreSep: true},
		),
		remoteAgentActionError: telemetryComp.NewCounterWithOpts(
			internalTelemetryNamespace,
			"action_error",
			[]string{"remote_agent_name", "action", "error"},
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

	// eventSubscribers receive Remote Agent events reported via ReportRemoteAgentEvent. The slice is
	// set once at construction and is immutable afterwards, so it needs no lock.
	eventSubscribers []*remoteagentregistry.EventSubscriber
}

// RegisterRemoteAgent registers a remote agent with the registry.
//
// It returns the session ID, the recommended refresh interval, and an error if the registration fails.
func (ra *remoteAgentRegistry) RegisterRemoteAgent(registration *remoteagentregistry.RegistrationData) (string, uint32, error) {
	recommendedRefreshInterval := uint32(ra.conf.GetDuration("remote_agent.registry.recommended_refresh_interval").Seconds())

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

// ReportRemoteAgentEvent records one or more events reported by a remote agent and broadcasts them to
// every registered event subscriber.
//
// It returns an error if no remote agent is registered with the given session ID.
func (ra *remoteAgentRegistry) ReportRemoteAgentEvent(sessionID string, events []remoteagentregistry.RemoteAgentEvent) error {
	ra.agentMapMu.Lock()
	agentClient, ok := ra.agentMap[sessionID]
	var agent remoteagentregistry.RegisteredAgent
	if ok {
		agent = agentClient.RegisteredAgent
	}
	ra.agentMapMu.Unlock()

	if !ok {
		return fmt.Errorf("no remote agent found with session ID %q", sessionID)
	}

	for _, event := range events {
		eventType := "unknown"
		if event.Details != nil {
			eventType = event.Details.EventType()
		}
		log.Debugf("Remote agent '%s' reported event (type: %s): %s", agent.DisplayName, eventType, event.Message)
	}

	// For each subscriber, dispatch the events through `dispatchEvents` which provides panic recovery behavior so
	// that we don't bork the entire gRPC handler.
	for _, subscriber := range ra.eventSubscribers {
		if subscriber == nil || subscriber.Callback == nil {
			continue
		}
		ra.dispatchEvents(subscriber, agent, events)
	}

	return nil
}

// dispatchEvents invokes a single subscriber's callback, recovering from any panic so that a
// misbehaving subscriber can neither fail the reporting RPC nor prevent the remaining subscribers from
// being notified.
func (ra *remoteAgentRegistry) dispatchEvents(subscriber *remoteagentregistry.EventSubscriber, agent remoteagentregistry.RegisteredAgent, events []remoteagentregistry.RemoteAgentEvent) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Remote Agent event subscriber %q panicked while handling events: %v", subscriber.Name, r)
		}
	}()

	subscriber.Callback(agent, events)
}

// Start starts the remote agent registry, which periodically checks for idle remote agents and deregisters them.
func (ra *remoteAgentRegistry) start() {
	remoteAgentIdleTimeout := ra.conf.GetDuration("remote_agent.registry.idle_timeout")
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

				for sessionID, details := range ra.agentMap {
					details.unhealthyMu.Lock()
					reason := details.unhealthyReason
					details.unhealthyMu.Unlock()
					if time.Since(details.RegisteredAgent.LastSeen) > remoteAgentIdleTimeout || reason != nil {
						if reason != nil {
							log.Warnf("Remote agent '%s' deregistered: %v", details.RegisteredAgent.DisplayName, reason)
						} else {
							log.Infof("Remote agent '%s' deregistered after being idle for %s.", details.RegisteredAgent.DisplayName, remoteAgentIdleTimeout)
						}
						ra.telemetryStore.remoteAgentRegistered.Dec(details.RegisteredAgent.SanitizedDisplayName)
						_ = details.close()
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
