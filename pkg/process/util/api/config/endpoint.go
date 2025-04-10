// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package config

import (
	"fmt"
	"net/url"

	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

// Endpoint is a single endpoint where process data will be submitted.
type Endpoint struct {
	APIKey   string
	Endpoint *url.URL

	// The path of the config used to get the API key. This path is used to listen for configuration updates from
	// the config.
	ConfigSettingPath string
}

// KeysPerDomains turns a list of endpoints into a map of URL -> []APIKey
func KeysPerDomains(endpoints []Endpoint) map[string][]utils.APIKeys {
	keysPerDomains := make(map[string][]utils.APIKeys)

	for _, ep := range endpoints {
		domain := removePathIfPresent(ep.Endpoint)
		keysPerDomains[domain] = append(keysPerDomains[domain], utils.APIKeys{
			ConfigSettingPath: ep.ConfigSettingPath,
			Keys:              []string{ep.APIKey},
		})
	}

	return keysPerDomains
}

// removePathIfPresent removes the path component from the URL if it is present
func removePathIfPresent(url *url.URL) string {
	return fmt.Sprintf("%s://%s", url.Scheme, url.Host)
}
