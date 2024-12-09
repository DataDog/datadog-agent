// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package server implements a component that runs the traps server.
// It listens for SNMP trap messages on a configured port, parses and
// reformats them, and sends the resulting data to the backend.
package server

// team: ndm-core

// Component is the SNMP traps server. It listens for SNMP traps messages and
// sends traps data to the DD backend.
type Component interface {
	// Running indicates whether the server is currently running.
	Running() bool
	// Error records any error that happened while starting the server.
	// If it is not nil, Running() should be false.
	Error() error
}
