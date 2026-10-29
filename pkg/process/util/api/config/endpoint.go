// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package config

import (
	"fmt"
	"net/url"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Endpoint is a single endpoint where process data will be submitted.
type Endpoint struct {
	APIKey   string
	Endpoint *url.URL

	// The path of the config used to get the API key. This path is used to listen for configuration updates from
	// the config.
	ConfigSettingPath string

	// HasPendingDelegatedAuth marks an endpoint whose only configured keys are DELA[...] directives
	// still waiting for the delegatedauth component to write back a resolved API key. APIKey is
	// empty for such endpoints; the forwarder keeps the domain alive until the config updates.
	HasPendingDelegatedAuth bool

	// ConfigKey is the additional_endpoints map key this endpoint was parsed from, as written in
	// config; empty for the main endpoint.
	ConfigKey string
}

// KeysPerDomains turns a list of endpoints into a map of URL -> []APIKey
//
// Deprecated: use EndpointDescriptors, which keeps the config key each domain came from so API
// key updates can find it.
func KeysPerDomains(endpoints []Endpoint) map[string][]utils.APIKeys {
	keysPerDomains := make(map[string][]utils.APIKeys)

	for _, ep := range endpoints {
		domain := removePathIfPresent(ep.Endpoint)
		// Only the delegated-auth pending marker contributes no key: the domain stays
		// alive through HasPendingDelegatedAuth. A plain empty key (e.g. the main api_key
		// during async delegated-auth startup) is kept as-is so its resolver is not
		// dropped before the resolved key is written back.
		var keys []string
		if !ep.HasPendingDelegatedAuth {
			keys = []string{ep.APIKey}
		}
		keysPerDomains[domain] = append(keysPerDomains[domain], utils.APIKeys{
			ConfigSettingPath:       ep.ConfigSettingPath,
			Keys:                    keys,
			HasPendingDelegatedAuth: ep.HasPendingDelegatedAuth,
		})
	}

	return keysPerDomains
}

// EndpointDescriptors groups endpoints by domain (scheme://host). The domain can differ from the
// additional_endpoints key it came from (e.g. a trailing slash or path), so when a domain has
// exactly one such key it becomes the descriptor's ConfigName, which API key updates look up.
func EndpointDescriptors(endpoints []Endpoint) utils.EndpointDescriptorSet {
	configKeys := make(map[string][]string)
	for _, ep := range endpoints {
		domain := removePathIfPresent(ep.Endpoint)
		if ep.ConfigKey != "" && !slices.Contains(configKeys[domain], ep.ConfigKey) {
			configKeys[domain] = append(configKeys[domain], ep.ConfigKey)
		}
	}

	eds := utils.EndpointDescriptorSet{}
	for domain, keyset := range KeysPerDomains(endpoints) {
		configName := ""
		switch keys := configKeys[domain]; {
		case len(keys) == 1 && keys[0] != domain:
			configName = keys[0]
		case len(keys) > 1:
			// A resolver tracks one config key, so an update only keeps the entry written as the domain.
			slices.Sort(keys)
			log.Warnf("additional endpoints %v all send to %s; after an API key update, only the keys of an entry written exactly as %s are kept until restart", keys, domain, domain)
		}
		eds[domain] = utils.NewEndpointDescriptorWithConfigName(configName, domain, keyset)
	}
	return eds
}

// removePathIfPresent removes the path component from the URL if it is present
func removePathIfPresent(url *url.URL) string {
	return fmt.Sprintf("%s://%s", url.Scheme, url.Host)
}
