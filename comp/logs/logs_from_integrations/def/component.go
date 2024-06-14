// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package logs_from_integrations the logs_from_integrations component for the
// Datadog Agent
//
// The logs_from_integrations component is a basic interface for integrations to
// send logs from one place to another. It has two faces: integrations can use
// the SendLog() function to send logs, and consumers can use the Subscribe()
// function to receive a channel that receives all the logs integrations send.
package logs_from_integrations

// team: agent-metrics-logs

// Component is the component type.
type Component interface {
	Subscribe() chan IntegrationLog

	SendLog(log, integrationID string)
}
