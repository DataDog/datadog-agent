// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fxnoop provides a print-only integrations component for use with
// the `agent check` subcommand. Logs submitted via send_log are printed to
// stderr instead of being forwarded to the backend.
package fxnoop

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
)

// noopIntegrations is a print-only implementation of integrations.Component.
// It satisfies the interface so that Python checks can call send_log without
// hitting a "no receiver" error, and it prints received logs to stderr so the
// operator can see what would have been forwarded.
type noopIntegrations struct{}

// NewNoopComponent returns a new print-only integrations.Component.
func NewNoopComponent() integrations.Component {
	return &noopIntegrations{}
}

// RegisterIntegration is a no-op; agent check does not forward integrations to the pipeline.
func (n *noopIntegrations) RegisterIntegration(_ string, _ integration.Config) {}

// SubscribeIntegration returns a nil channel; agent check has no subscriber.
func (n *noopIntegrations) SubscribeIntegration() chan integrations.IntegrationConfig {
	return nil
}

// Subscribe returns a nil channel; agent check has no subscriber.
func (n *noopIntegrations) Subscribe() chan integrations.IntegrationLog {
	return nil
}

// SendLog prints the log line to stderr so the operator can see it without
// polluting stdout, which may carry JSON-formatted output in --json mode.
func (n *noopIntegrations) SendLog(log, integrationID string) {
	fmt.Fprintf(os.Stderr, "Log from integration %s: %s\n", integrationID, log)
}
