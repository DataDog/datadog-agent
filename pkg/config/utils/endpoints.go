// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// GetMainEndpointBackwardCompatible implements the logic to extract the DD URL from a config, based on `site`,ddURLKey and a backward compatible key
func GetMainEndpointBackwardCompatible(c config.Reader, prefix string, ddURLKey string, backwardKey string) string {
	return pkgconfigsetup.GetMainEndpointBackwardCompatible(c, prefix, ddURLKey, backwardKey)
}

// GetMultipleEndpoints returns the api keys per domain specified in the main agent config
func GetMultipleEndpoints(c config.Reader) (map[string][]string, error) {
	return pkgconfigsetup.GetMultipleEndpoints(c)
}

// GetMainEndpoint returns the main DD URL defined in the config, based on `site` and the prefix, or ddURLKey
func GetMainEndpoint(c config.Reader, prefix string, ddURLKey string) string {
	return pkgconfigsetup.GetMainEndpoint(c, prefix, ddURLKey)
}

// GetInfraEndpoint returns the main DD Infra URL defined in config, based on the value of `site` and `dd_url`
func GetInfraEndpoint(c config.Reader) string {
	return pkgconfigsetup.GetInfraEndpoint(c)
}

// AddAgentVersionToDomain prefixes the domain with the agent version: X-Y-Z.domain
func AddAgentVersionToDomain(DDURL string, app string) (string, error) {
	return pkgconfigsetup.AddAgentVersionToDomain(DDURL, app)
}
