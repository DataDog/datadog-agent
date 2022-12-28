// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
)

const (
	ns = "process_config"
)

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
}

// LoadAgentConfig loads process-agent specific configurations based on the global Config object
func (a *AgentConfig) LoadAgentConfig(path string) error {
	loadEnvVariables()

	// Resolve any secrets
	if err := config.ResolveSecrets(config.Datadog, filepath.Base(path)); err != nil {
		return err
	}

	if config.Datadog.IsSet("hostname") {
		a.HostName = config.Datadog.GetString("hostname")
	}

	return nil
}
