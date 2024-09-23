// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(CAPP) Fix revive linter
package config

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/redact"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	orchestratorNS  = "orchestrator_explorer"
	processNS       = "process_config"
	defaultEndpoint = "https://orchestrator.datadoghq.com"
	maxMessageBatch = 100
	maxMessageSize  = 50 * 1e6 // 50 MB
)

// OrchestratorConfig is the global config for the Orchestrator related packages. This information
// is sourced from config files and the environment variables.
type OrchestratorConfig struct {
	CollectorDiscoveryEnabled      bool
	OrchestrationCollectionEnabled bool
	KubeClusterName                string
	IsScrubbingEnabled             bool
	Scrubber                       *redact.DataScrubber
	OrchestratorEndpoints          []apicfg.Endpoint
	MaxPerMessage                  int
	MaxWeightPerMessageBytes       int
	PodQueueBytes                  int // The total number of bytes that can be enqueued for delivery to the orchestrator endpoint
	ExtraTags                      []string
	IsManifestCollectionEnabled    bool
	BufferedManifestEnabled        bool
	ManifestBufferFlushInterval    time.Duration
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
		Scrubber:                 redact.NewDefaultDataScrubber(),
		MaxPerMessage:            100,
		MaxWeightPerMessageBytes: 10000000,
		OrchestratorEndpoints:    []apicfg.Endpoint{{Endpoint: orchestratorEndpoint}},
		PodQueueBytes:            15 * 1000 * 1000,
	}
	return &oc
}

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
}

// OrchestratorNSKey get the config name key for orchestratorNS
func OrchestratorNSKey(pieces ...string) string {
	fullKey := append([]string{orchestratorNS}, pieces...)
	return key(fullKey...)
}

// Load loads orchestrator-specific configuration
// at this point secrets should already be resolved by the core/process/cluster agent
func (oc *OrchestratorConfig) Load() error {
	URL, err := extractOrchestratorDDUrl()
	if err != nil {
		return err
	}
	oc.OrchestratorEndpoints[0].Endpoint = URL

	if key := "api_key"; pkgconfigsetup.Datadog().IsSet(key) {
		oc.OrchestratorEndpoints[0].APIKey = utils.SanitizeAPIKey(pkgconfigsetup.Datadog().GetString(key))
	}

	if err := extractOrchestratorAdditionalEndpoints(URL, &oc.OrchestratorEndpoints); err != nil {
		return err
	}

	// A custom word list to enhance the default one used by the DataScrubber
	if k := OrchestratorNSKey("custom_sensitive_words"); pkgconfigsetup.Datadog().IsSet(k) {
		oc.Scrubber.AddCustomSensitiveWords(pkgconfigsetup.Datadog().GetStringSlice(k))
	}

	if k := OrchestratorNSKey("custom_sensitive_annotations_labels"); pkgconfigsetup.Datadog().IsSet(k) {
		redact.UpdateSensitiveAnnotationsAndLabels(pkgconfigsetup.Datadog().GetStringSlice(k))
	}

	// The maximum number of resources per message and the maximum message size.
	// Note: Only change if the defaults are causing issues.
	setBoundedConfigIntValue(OrchestratorNSKey("max_per_message"), maxMessageBatch, func(v int) { oc.MaxPerMessage = v })
	setBoundedConfigIntValue(OrchestratorNSKey("max_message_bytes"), maxMessageSize, func(v int) { oc.MaxWeightPerMessageBytes = v })

	if k := key(processNS, "pod_queue_bytes"); pkgconfigsetup.Datadog().IsSet(k) {
		if queueBytes := pkgconfigsetup.Datadog().GetInt(k); queueBytes > 0 {
			oc.PodQueueBytes = queueBytes
		}
	}

	// Orchestrator Explorer
	oc.OrchestrationCollectionEnabled, oc.KubeClusterName = IsOrchestratorEnabled()

	oc.CollectorDiscoveryEnabled = pkgconfigsetup.Datadog().GetBool(OrchestratorNSKey("collector_discovery.enabled"))
	oc.IsScrubbingEnabled = pkgconfigsetup.Datadog().GetBool(OrchestratorNSKey("container_scrubbing.enabled"))
	oc.ExtraTags = pkgconfigsetup.Datadog().GetStringSlice(OrchestratorNSKey("extra_tags"))
	oc.IsManifestCollectionEnabled = pkgconfigsetup.Datadog().GetBool(OrchestratorNSKey("manifest_collection.enabled"))
	oc.BufferedManifestEnabled = pkgconfigsetup.Datadog().GetBool(OrchestratorNSKey("manifest_collection.buffer_manifest"))
	oc.ManifestBufferFlushInterval = pkgconfigsetup.Datadog().GetDuration(OrchestratorNSKey("manifest_collection.buffer_flush_interval"))

	return nil
}

func extractOrchestratorAdditionalEndpoints(URL *url.URL, orchestratorEndpoints *[]apicfg.Endpoint) error {
	if k := OrchestratorNSKey("orchestrator_additional_endpoints"); pkgconfigsetup.Datadog().IsSet(k) {
		if err := extractEndpoints(URL, k, orchestratorEndpoints); err != nil {
			return err
		}
	} else if k := key(processNS, "orchestrator_additional_endpoints"); pkgconfigsetup.Datadog().IsSet(k) {
		if err := extractEndpoints(URL, k, orchestratorEndpoints); err != nil {
			return err
		}
	}
	return nil
}

func extractEndpoints(URL *url.URL, k string, endpoints *[]apicfg.Endpoint) error {
	for endpointURL, apiKeys := range pkgconfigsetup.Datadog().GetStringMapStringSlice(k) {
		u, err := URL.Parse(endpointURL)
		if err != nil {
			return fmt.Errorf("invalid additional endpoint url '%s': %s", endpointURL, err)
		}
		for _, k := range apiKeys {
			*endpoints = append(*endpoints, apicfg.Endpoint{
				APIKey:   utils.SanitizeAPIKey(k),
				Endpoint: u,
			})
		}
	}
	return nil
}

// extractOrchestratorDDUrl contains backward compatible config parsing code.
func extractOrchestratorDDUrl() (*url.URL, error) {
	orchestratorURL := OrchestratorNSKey("orchestrator_dd_url")
	processURL := key(processNS, "orchestrator_dd_url")
	URL, err := url.Parse(utils.GetMainEndpointBackwardCompatible(pkgconfigsetup.Datadog(), "https://orchestrator.", orchestratorURL, processURL))
	if err != nil {
		return nil, fmt.Errorf("error parsing orchestrator_dd_url: %s", err)
	}
	return URL, nil
}

func setBoundedConfigIntValue(configKey string, upperBound int, setter func(v int)) {
	if !pkgconfigsetup.Datadog().IsSet(configKey) {
		return
	}

	val := pkgconfigsetup.Datadog().GetInt(configKey)

	if val <= 0 {
		pkglog.Warnf("Ignoring invalid value for setting %s (<=0)", configKey)
		return
	}
	if val > upperBound {
		pkglog.Warnf("Ignoring invalid value for setting %s (exceeds maximum allowed value %d)", configKey, upperBound)
		return
	}

	setter(val)
}

// IsOrchestratorEnabled checks if orchestrator explorer features are enabled, it returns the boolean and the cluster name
func IsOrchestratorEnabled() (bool, string) {
	enabled := pkgconfigsetup.Datadog().GetBool(OrchestratorNSKey("enabled"))
	var clusterName string
	if enabled {
		// Set clustername
		hname, _ := hostname.Get(context.TODO())
		clusterName = clustername.GetRFC1123CompliantClusterName(context.TODO(), hname)
	}
	return enabled, clusterName
}

// IsOrchestratorECSExplorerEnabled checks if orchestrator ecs explorer features are enabled
func IsOrchestratorECSExplorerEnabled() bool {
	if !pkgconfigsetup.Datadog().GetBool(OrchestratorNSKey("enabled")) {
		return false
	}

	if !pkgconfigsetup.Datadog().GetBool("ecs_task_collection_enabled") {
		return false
	}

	if env.IsECS() || env.IsECSFargate() {
		return true
	}

	return false
}
