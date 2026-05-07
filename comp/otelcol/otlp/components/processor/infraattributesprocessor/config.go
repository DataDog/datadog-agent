// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"go.opentelemetry.io/collector/component"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

// Config defines configuration for processor.
type Config struct {
	// Cardinality controls which tag cardinality is enriched onto the signal.
	// Accepted values: 0 = low (host-level, default), 1 = orchestrator
	// (per-pod/task), 2 = high (per-container/request).
	Cardinality           types.TagCardinality `mapstructure:"cardinality"`
	AllowHostnameOverride bool                 `mapstructure:"allow_hostname_override"`
}

var _ component.Config = (*Config)(nil)

// Validate configuration
func (cfg *Config) Validate() error {
	return nil
}
