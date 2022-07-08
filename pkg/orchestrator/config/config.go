// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/redact"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
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
	OrchestratorEndpoints          []apicfg.Endpoint
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
		OrchestratorEndpoints: []apicfg.Endpoint{{Endpoint: orchestratorEndpoint}},
		PodQueueBytes:         15 * 1000 * 1000,
	}
	return &oc
}

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
}

// Load loads orchestrator-specific configuration
// at this point secrets should already be resolved by the core/process/cluster agent
func (oc *OrchestratorConfig) Load() error {
	URL, err := extractOrchestratorDDUrl()
	if err != nil {
		return err
	}
	oc.OrchestratorEndpoints[0].Endpoint = URL

	if key := "api_key"; config.Datadog.IsSet(key) {
		oc.OrchestratorEndpoints[0].APIKey = config.SanitizeAPIKey(config.Datadog.GetString(key))
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
	if config.Datadog.GetBool(key(orchestratorNS, "enabled")) {
		oc.OrchestrationCollectionEnabled = true
		// Set clustername
		hname, _ := hostname.Get(context.TODO())
		if clusterName := clustername.GetClusterName(context.TODO(), hname); clusterName != "" {
			oc.KubeClusterName = clusterName
		}
	}
	oc.IsScrubbingEnabled = config.Datadog.GetBool(key(orchestratorNS, "container_scrubbing.enabled"))
	oc.ExtraTags = config.Datadog.GetStringSlice(key(orchestratorNS, "extra_tags"))

	return nil
}

func extractOrchestratorAdditionalEndpoints(URL *url.URL, orchestratorEndpoints *[]apicfg.Endpoint) error {
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

func extractEndpoints(URL *url.URL, k string, endpoints *[]apicfg.Endpoint) error {
	for endpointURL, apiKeys := range config.Datadog.GetStringMapStringSlice(k) {
		u, err := URL.Parse(endpointURL)
		if err != nil {
			return fmt.Errorf("invalid additional endpoint url '%s': %s", endpointURL, err)
		}
		for _, k := range apiKeys {
			*endpoints = append(*endpoints, apicfg.Endpoint{
				APIKey:   config.SanitizeAPIKey(k),
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

// NewOrchestratorForwarder returns an orchestratorForwarder
// if the feature is activated on the cluster-agent/cluster-check runner, nil otherwise
func NewOrchestratorForwarder() forwarder.Forwarder {
	if !config.Datadog.GetBool(key(orchestratorNS, "enabled")) {
		return nil
	}
	if flavor.GetFlavor() == flavor.DefaultAgent && !config.IsCLCRunner() {
		return nil
	}
	orchestratorCfg := NewDefaultOrchestratorConfig()
	if err := orchestratorCfg.Load(); err != nil {
		log.Errorf("Error loading the orchestrator config: %s", err)
	}
	keysPerDomain := apicfg.KeysPerDomains(orchestratorCfg.OrchestratorEndpoints)
	orchestratorForwarderOpts := forwarder.NewOptionsWithResolvers(resolver.NewSingleDomainResolvers(keysPerDomain))
	orchestratorForwarderOpts.DisableAPIKeyChecking = true

	return forwarder.NewDefaultForwarder(orchestratorForwarderOpts)
}
