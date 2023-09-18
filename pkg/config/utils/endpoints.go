// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"github.com/DataDog/datadog-agent/pkg/conf/utils"
)

// GetMainEndpointBackwardCompatible implements the logic to extract the DD URL from a config, based on `site`,ddURLKey and a backward compatible key
var GetMainEndpointBackwardCompatible = utils.GetMainEndpointBackwardCompatible

// GetMultipleEndpoints returns the api keys per domain specified in the main agent config
var GetMultipleEndpoints = utils.GetMultipleEndpoints

// GetMainEndpoint returns the main DD URL defined in the config, based on `site` and the prefix, or ddURLKey
var GetMainEndpoint = utils.GetMainEndpoint

// GetInfraEndpoint returns the main DD Infra URL defined in config, based on the value of `site` and `dd_url`
var GetInfraEndpoint = utils.GetInfraEndpoint

// AddAgentVersionToDomain prefixes the domain with the agent version: X-Y-Z.domain
var AddAgentVersionToDomain = utils.AddAgentVersionToDomain
