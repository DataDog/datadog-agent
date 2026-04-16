// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package remoteagentregistry

import (
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

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

// CommandData contains a remote command definition and which agent provides it
type CommandData struct {
	RegisteredAgent
	Commands []*pb.Command
}

// CommandResult contains the result of executing a remote command
type CommandResult struct {
	ExitCode     int32
	Stdout       string
	Stderr       string
	BinaryOutput []byte
}
