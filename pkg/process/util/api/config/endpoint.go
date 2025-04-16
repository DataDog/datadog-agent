// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package config

import (
	"fmt"
	"net/url"
)

// Endpoint is a single endpoint where process data will be submitted.
type Endpoint struct {
	APIKey   string
	Endpoint *url.URL
}

// KeysPerDomains turns a list of endpoints into a map of URL -> []APIKey
func KeysPerDomains(endpoints []Endpoint) map[string][]string {
	keysPerDomains := make(map[string][]string)

	for _, ep := range endpoints {
		domain := removePathIfPresent(ep.Endpoint)
		keysPerDomains[domain] = append(keysPerDomains[domain], ep.APIKey)
	}

	return keysPerDomains
}

// removePathIfPresent removes the path component from the URL if it is present
func removePathIfPresent(url *url.URL) string {
	return fmt.Sprintf("%s://%s", url.Scheme, url.Host)
}
