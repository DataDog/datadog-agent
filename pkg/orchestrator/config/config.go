// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build orchestrator

package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	orchestratorNS  = "orchestrator_explorer"
	maxMessageBatch = 100
)

// OrchestratorConfig is the global config for the Orchestrator related packages. This information
// is sourced from config files and the environment variables.
type OrchestratorConfig struct {
	Scrubber      *orchestrator.DataScrubber
	MaxPerMessage int
}

// NewDefaultOrchestratorConfig returns an NewDefaultOrchestratorConfig using a configuration file. It can be nil
// if there is no file available. In this case we'll configure only via environment.
func NewDefaultOrchestratorConfig() *OrchestratorConfig {
	oc := OrchestratorConfig{
		Scrubber:      orchestrator.NewDefaultDataScrubber(),
		MaxPerMessage: 100,
	}
	return &oc
}

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
}

// LoadYamlConfig load orchestrator-specific configuration
func (oc OrchestratorConfig) LoadYamlConfig(path string) error {
	loadEnvVariables()
	// Resolve any secrets
	if err := config.ResolveSecrets(config.Datadog, filepath.Base(path)); err != nil {
		return err
	}

	// A custom word list to enhance the default one used by the DataScrubber
	if k := key(orchestratorNS, "custom_sensitive_words"); config.Datadog.IsSet(k) {
		oc.Scrubber.AddCustomSensitiveWords(config.Datadog.GetStringSlice(k))
	}

	// The maximum number of pods, nodes, replicaSets, deployments and services per message. Note: Only change if the defaults are causing issues.
	if k := key(orchestratorNS, "max_per_message"); config.Datadog.IsSet(k) {
		if maxPerMessage := config.Datadog.GetInt(k); maxPerMessage <= 0 {
			log.Warn("Invalid item count per message (<= 0), ignoring...")
		} else if maxPerMessage <= maxMessageBatch {
			oc.MaxPerMessage = maxPerMessage
		} else if maxPerMessage > 0 {
			log.Warn("Overriding the configured item count per message limit because it exceeds maximum")
		}
	}

	return nil
}

func loadEnvVariables() {
	if v := os.Getenv("DD_ORCHESTRATOR_CUSTOM_SENSITIVE_WORDS"); v != "" {
		config.Datadog.Set(key(orchestratorNS, "custom_sensitive_words"), strings.Split(v, ","))
	}
}
