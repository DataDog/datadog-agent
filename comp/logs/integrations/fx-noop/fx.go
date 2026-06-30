// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides a print-only integrations component for use with
// the `agent check` subcommand. Logs submitted via send_log are printed to
// stderr instead of being forwarded to the backend.
package fx

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// team: agent-log-pipelines

// Module defines the fx options for this component. It supplies an
// option.Option[integrations.Component] wrapping a print-only receiver,
// so subcommands like `agent check` that don't run the full logs agent can
// still satisfy `send_log` calls from Python checks.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(newComponent),
	)
}

// newComponent constructs the print-only integrations receiver as an Option
// so it slots into the same dependency slot as the real logs receiver.
func newComponent() option.Option[integrations.Component] {
	return option.New[integrations.Component](NewNoopComponent())
}

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
