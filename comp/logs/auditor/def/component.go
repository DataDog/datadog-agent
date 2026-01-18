// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package auditor records the log files the agent is tracking. It tracks
// filename, time last updated, offset (how far into the file the agent has
// read), and tailing mode for each log file.
package auditor

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// team: agent-log-pipelines

// Component is the component type.
type Component interface {
	Registry

	// Start starts the auditor
	Start()

	// Stop stops the auditor
	Stop()

	// Flush immediately writes the current registry to disk
	Flush()

	// Channel returns the channel to use to communicate with the auditor or nil
	// if the auditor is currently stopped.
	Channel() chan *message.Payload
}

// RegistryWriter defines the interface for writing registry data
type RegistryWriter interface {
	WriteRegistry(registryPath string, registryDirPath string, registryTmpFile string, data []byte) error
}
