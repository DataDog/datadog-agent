// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package remoteagentregistry

// RegisteredAgent contains the information about a registered remote agent
type RegisteredAgent struct {
	DisplayName  string
	LastSeenUnix int64
}

// StatusSection is a map of key-value pairs that represent a section of the status data
type StatusSection map[string]string

// StatusData contains the status data for a remote agent
type StatusData struct {
	MainSection   StatusSection
	NamedSections map[string]StatusSection
	AgentID       string
	DisplayName   string
	FailureReason string
}

// FlareData contains the flare data for a remote agent
type FlareData struct {
	Files   map[string][]byte
	AgentID string
}

// RegistrationData contains the registration information for a remote agent
type RegistrationData struct {
	AgentID     string
	DisplayName string
	APIEndpoint string
	AuthToken   string
}
