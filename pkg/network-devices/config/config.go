// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	coreutil "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	networkDevicesNS = "network_devices"
	defaultEndpoint  = "https://network-devices.datadoghq.com"
	maxMessageBatch  = 100
)

// NetworkDevicesConfig is the global config for the Network Devices related packages. This information
// is sourced from config files and the environment variables.
type NetworkDevicesConfig struct {
	OrchestrationCollectionEnabled bool
	KubeClusterName                string
	NetworkDevicesEndpoints        []apicfg.Endpoint
	MaxPerMessage                  int
	ExtraTags                      []string
}

// NewDefaultNetworkDevicesConfig returns an NewDefaultNetworkDevicesConfig using a configuration file. It can be nil
// if there is no file available. In this case we'll configure only via environment.
func NewDefaultNetworkDevicesConfig() *NetworkDevicesConfig {
	netorkDevicesEndpoint, err := url.Parse(defaultEndpoint)
	if err != nil {
		// This is a hardcoded URL so parsing it should not fail
		panic(err)
	}
	oc := NetworkDevicesConfig{
		MaxPerMessage:           100,
		NetworkDevicesEndpoints: []apicfg.Endpoint{{Endpoint: netorkDevicesEndpoint}},
	}
	return &oc
}

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
}

// Load load network-devices-specific configuration
// at this point secrets should already be resolved by the core/process/cluster agent
func (oc *NetworkDevicesConfig) Load() error {
	URL, err := extractNetworkDevicesDDUrl()
	if err != nil {
		return err
	}
	oc.NetworkDevicesEndpoints[0].Endpoint = URL

	if key := "api_key"; config.Datadog.IsSet(key) {
		oc.NetworkDevicesEndpoints[0].APIKey = config.SanitizeAPIKey(config.Datadog.GetString(key))
	}

	if err := extractNetworkDevicesAdditionalEndpoints(URL, &oc.NetworkDevicesEndpoints); err != nil {
		return err
	}

	// The maximum number of pods, nodes, replicaSets, deployments and services per message. Note: Only change if the defaults are causing issues.
	if k := key(networkDevicesNS, "max_per_message"); config.Datadog.IsSet(k) {
		if maxPerMessage := config.Datadog.GetInt(k); maxPerMessage <= 0 {
			log.Warn("Invalid item count per message (<= 0), ignoring...")
		} else if maxPerMessage <= maxMessageBatch {
			oc.MaxPerMessage = maxPerMessage
		} else if maxPerMessage > 0 {
			log.Warn("Overriding the configured item count per message limit because it exceeds maximum")
		}
	}

	// Network Devices
	if config.Datadog.GetBool("network_devices.enabled") {
		oc.OrchestrationCollectionEnabled = true
		// Set clustername
		hostname, _ := coreutil.GetHostname()
		if clusterName := clustername.GetClusterName(hostname); clusterName != "" {
			oc.KubeClusterName = clusterName
		}
	}
	oc.ExtraTags = config.Datadog.GetStringSlice("network_devices.extra_tags")

	return nil
}

func extractNetworkDevicesAdditionalEndpoints(URL *url.URL, networkDevicesEndpoints *[]apicfg.Endpoint) error {
	if k := key(networkDevicesNS, "network_devices_additional_endpoints"); config.Datadog.IsSet(k) {
		if err := extractEndpoints(URL, k, networkDevicesEndpoints); err != nil {
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

// extractNetworkDevicesDDUrl contains backward compatible config parsing code.
func extractNetworkDevicesDDUrl() (*url.URL, error) {
	networkDevicesURL := key(networkDevicesNS, "network_devices_dd_url")
	URL, err := url.Parse(config.GetMainEndpointWithConfig(config.Datadog, "https://network-devices.", networkDevicesURL))
	if err != nil {
		return nil, fmt.Errorf("error parsing network_devices_dd_url: %s", err)
	}
	return URL, nil
}

// NewNetworkDevicesForwarder returns an networkDevicesForwarder
// if the feature is activated on the cluster-agent/cluster-check runner, nil otherwise
func NewNetworkDevicesForwarder() *forwarder.DefaultForwarder {
	if !config.Datadog.GetBool("network_devices.enabled") {
		return nil
	}
	//if flavor.GetFlavor() == flavor.DefaultAgent && !config.IsCLCRunner() {
	//	return nil
	//}
	netorkDevicesCfg := NewDefaultNetworkDevicesConfig()
	if err := netorkDevicesCfg.Load(); err != nil {
		log.Errorf("Error loading the network-devices config: %s", err)
	}
	keysPerDomain := apicfg.KeysPerDomains(netorkDevicesCfg.NetworkDevicesEndpoints)
	networkDevicesForwarderOpts := forwarder.NewOptions(keysPerDomain)
	networkDevicesForwarderOpts.DisableAPIKeyChecking = true

	return forwarder.NewDefaultForwarder(networkDevicesForwarderOpts)
}
