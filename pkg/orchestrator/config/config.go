/*
 * Unless explicitly stated otherwise all files in this repository are licensed
 * under the Apache License Version 2.0.
 * This product includes software developed at Datadog (https://www.datadoghq.com/).
 * Copyright 2016-2020 Datadog, Inc.
 */

package config

import (
	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/process/util/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultOrchestratorEndpoint  = "https://orchestrator.datadoghq.com"
	orchestratorNS               = "orchestrator_explorer"
	maxMessageBatch              = 100
)

// OrchestratorConfig is the global config for the Orchestrator related packages. This information
// is sourced from config files and the environment variables.
type OrchestratorConfig struct {
	Scrubber      *orchestrator.DataScrubber
	MaxPerMessage int
	// Orchestrator collection configuration
	OrchestrationCollectionEnabled bool
	KubeClusterName                string
	IsScrubbingEnabled             bool
	OrchestratorEndpoints          []api.Endpoint
}

func NewDefaultOrchestratorConfig() *OrchestratorConfig {
	orchestratorEndpoint, err := url.Parse(defaultOrchestratorEndpoint)
	if err != nil {
		// This is a hardcoded URL so parsing it should not fail
		panic(err)
	}

	oc := OrchestratorConfig{
		Scrubber:                       orchestrator.NewDefaultDataScrubber(),
		MaxPerMessage:                  100,
		OrchestrationCollectionEnabled: false,
		KubeClusterName:                "",
		IsScrubbingEnabled:             false,
		OrchestratorEndpoints:          []api.Endpoint{{Endpoint: orchestratorEndpoint}},
	}
	return &oc
}

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
}

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
	// The following environment variables will be loaded in the order listed, meaning variables
	// further down the list may override prior variables.
	for _, variable := range []struct{ env, cfg string }{
		{"DD_ORCHESTRATOR_URL", "orchestrator_explorer.orchestrator_dd_url"},
		{"DD_HOSTNAME", "hostname"},
		{"DD_DOGSTATSD_PORT", "dogstatsd_port"},
		{"DD_BIND_HOST", "bind_host"},
		{"HTTPS_PROXY", "proxy.https"},
		{"DD_PROXY_HTTPS", "proxy.https"},

		{"DD_LOGS_STDOUT", "log_to_console"},
		{"LOG_TO_CONSOLE", "log_to_console"},
		{"DD_LOG_TO_CONSOLE", "log_to_console"},
		{"LOG_LEVEL", "log_level"}, // Support LOG_LEVEL and DD_LOG_LEVEL but prefer DD_LOG_LEVEL
		{"DD_LOG_LEVEL", "log_level"},
	} {
		if v, ok := os.LookupEnv(variable.env); ok {
			config.Datadog.Set(variable.cfg, v)
		}
	}

	if v := os.Getenv("DD_ORCHESTRATOR_CUSTOM_SENSITIVE_WORDS"); v != "" {
		config.Datadog.Set(key(orchestratorNS, "custom_sensitive_words"), strings.Split(v, ","))
	}

	if v := os.Getenv("DD_ORCHESTRATOR_ADDITIONAL_ENDPOINTS"); v != "" {
		endpoints := make(map[string][]string)
		if err := json.Unmarshal([]byte(v), &endpoints); err != nil {
			log.Errorf(`Could not parse DD_ORCHESTRATOR_ADDITIONAL_ENDPOINTS: %v. It must be of the form '{"https://process.agent.datadoghq.com": ["apikey1", ...], ...}'.`, err)
		} else {
			config.Datadog.Set("orchestrator_explorer.orchestrator_additional_endpoints", endpoints)
		}
	}
}
