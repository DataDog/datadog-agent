// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/redact"
	"github.com/DataDog/datadog-agent/pkg/process/util/api"
	coreutil "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	orchestratorNS  = "orchestrator_explorer"
	processNS       = "process_config"
	defaultEndpoint = "https://orchestrator.datadoghq.com"
	maxMessageBatch = 100
)

// OrchestratorConfig is the global config for the Orchestrator related packages. This information
// is sourced from config files and the environment variables.
type OrchestratorConfig struct {
	OrchestrationCollectionEnabled bool
	KubeClusterName                string
	IsScrubbingEnabled             bool
	Scrubber                       *redact.DataScrubber
	OrchestratorEndpoints          []api.Endpoint
	MaxPerMessage                  int
	PodQueueBytes                  int // The total number of bytes that can be enqueued for delivery to the orchestrator endpoint
	ExtraTags                      []string
}

// NewDefaultOrchestratorConfig returns an NewDefaultOrchestratorConfig using a configuration file. It can be nil
// if there is no file available. In this case we'll configure only via environment.
func NewDefaultOrchestratorConfig() *OrchestratorConfig {
	orchestratorEndpoint, err := url.Parse(defaultEndpoint)
	if err != nil {
		// This is a hardcoded URL so parsing it should not fail
		panic(err)
	}
	oc := OrchestratorConfig{
		Scrubber:              redact.NewDefaultDataScrubber(),
		MaxPerMessage:         100,
		OrchestratorEndpoints: []api.Endpoint{{Endpoint: orchestratorEndpoint}},
		PodQueueBytes:         15 * 1000 * 1000,
	}
	return &oc
}

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
}

// LoadYamlConfig load orchestrator-specific configuration
func (oc *OrchestratorConfig) LoadYamlConfig(path string) error {
	loadEnvVariables()
	// Resolve any secrets
	if err := config.ResolveSecrets(config.Datadog, filepath.Base(path)); err != nil {
		return err
	}

	URL, err := extractOrchestratorDDUrl()
	if err != nil {
		return err
	}
	oc.OrchestratorEndpoints[0].Endpoint = URL

	if key := "api_key"; config.Datadog.IsSet(key) {
		oc.OrchestratorEndpoints[0].APIKey = config.Datadog.GetString(key)
	}

	if err := extractOrchestratorAdditionalEndpoints(URL, &oc.OrchestratorEndpoints); err != nil {
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

	if k := key(processNS, "pod_queue_bytes"); config.Datadog.IsSet(k) {
		if queueBytes := config.Datadog.GetInt(k); queueBytes > 0 {
			oc.PodQueueBytes = queueBytes
		}
	}

	// Orchestrator Explorer
	if config.Datadog.GetBool("orchestrator_explorer.enabled") {
		oc.OrchestrationCollectionEnabled = true
		// Set clustername
		hostname, _ := coreutil.GetHostname()
		if clusterName := clustername.GetClusterName(hostname); clusterName != "" {
			oc.KubeClusterName = clusterName
		}
	}
	oc.IsScrubbingEnabled = config.Datadog.GetBool("orchestrator_explorer.container_scrubbing.enabled")
	oc.ExtraTags = config.Datadog.GetStringSlice("orchestrator_explorer.extra_tags")

	return nil
}

func loadEnvVariables() {
	if v := os.Getenv("DD_ORCHESTRATOR_CUSTOM_SENSITIVE_WORDS"); v != "" {
		config.Datadog.Set(key(orchestratorNS, "custom_sensitive_words"), strings.Split(v, ","))
	}
}

func extractOrchestratorAdditionalEndpoints(URL *url.URL, orchestratorEndpoints *[]api.Endpoint) error {
	if k := key(orchestratorNS, "orchestrator_additional_endpoints"); config.Datadog.IsSet(k) {
		if err := extractEndpoints(URL, k, orchestratorEndpoints); err != nil {
			return err
		}
	} else if k := key(processNS, "orchestrator_additional_endpoints"); config.Datadog.IsSet(k) {
		if err := extractEndpoints(URL, k, orchestratorEndpoints); err != nil {
			return err
		}
	}
	return nil
}

func extractEndpoints(URL *url.URL, k string, endpoints *[]api.Endpoint) error {
	for endpointURL, apiKeys := range config.Datadog.GetStringMapStringSlice(k) {
		u, err := URL.Parse(endpointURL)
		if err != nil {
			return fmt.Errorf("invalid additional endpoint url '%s': %s", endpointURL, err)
		}
		for _, k := range apiKeys {
			*endpoints = append(*endpoints, api.Endpoint{
				APIKey:   k,
				Endpoint: u,
			})
		}
	}
	return nil
}

// extractOrchestratorDDUrl contains backward compatible config parsing code.
func extractOrchestratorDDUrl() (*url.URL, error) {
	orchestratorURL := key(orchestratorNS, "orchestrator_dd_url")
	processURL := key(processNS, "orchestrator_dd_url")
	URL, err := url.Parse(config.GetMainEndpointWithConfigBackwardCompatible(config.Datadog, "https://orchestrator.", orchestratorURL, processURL))
	if err != nil {
		return nil, fmt.Errorf("error parsing orchestrator_dd_url: %s", err)
	}
	return URL, nil
}
