// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package remoteagentregistry

import "time"

// RegisteredAgent contains the information about a registered remote agent
type RegisteredAgent struct {
	Flavor               string
	DisplayName          string
	SanitizedDisplayName string
	PID                  string
	LastSeen             time.Time
	SessionID            string
}

func (a *RegisteredAgent) String() string {
	return a.DisplayName + "-" + a.Flavor + "-" + a.SessionID
}

// StatusSection is a map of key-value pairs that represent a section of the status data
type StatusSection map[string]string

// StatusData contains the status data for a remote agent
type StatusData struct {
	RegisteredAgent
	FailureReason string
	MainSection   StatusSection
	NamedSections map[string]StatusSection
}

// FlareData contains the flare data for a remote agent
type FlareData struct {
	RegisteredAgent
	Files map[string][]byte
}

// RegistrationData contains the registration information for a remote agent
type RegistrationData struct {
	AgentFlavor      string
	AgentDisplayName string
	AgentPID         string
	APIEndpointURI   string
	Services         []string
}

// EventDetails is implemented by each remote agent event detail variant. It mirrors the `details` oneof in the
// RemoteAgent protobuf definition: consumers type-assert the concrete variant (e.g. *InvalidAPIKey) to handle a
// specific kind of event, or call EventType for a stable string identifier without casting.
type EventDetails interface {
	// EventType returns a stable string identifier for the kind of event.
	EventType() string
}

// InvalidAPIKey indicates a remote agent detected an invalid API key when forwarding.
type InvalidAPIKey struct{}

// EventType implements EventDetails.
func (*InvalidAPIKey) EventType() string { return "invalid_api_key" }

// RemoteAgentEvent is a single discrete event reported by a remote agent.
type RemoteAgentEvent struct {
	// Message is a simple human-friendly description of the event.
	Message string
	// Details carries the type-specific details of the event, or nil if the event carried no recognized details.
	Details EventDetails
}
