// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package config // import "github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/datadogconfig"

import "go.opentelemetry.io/collector/config/confignet"

// OrchestratorExplorerConfig defines configuration for the Datadog orchestrator explorer.
type OrchestratorExplorerConfig struct {
	// TCPAddr.Endpoint is the host of the Datadog orchestrator intake server to send data to.
	// If unset, the value is obtained from the Site.
	confignet.TCPAddrConfig `mapstructure:",squash"`
	// Enabled enables the orchestrator explorer.
	Enabled bool `mapstructure:"enabled"`
}
