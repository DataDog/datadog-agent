// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package endpoint

import (
	"fmt"
	"net/url"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
)

func GetAPIEndpoints(config ddconfig.ConfigReader) (eps []apicfg.Endpoint, err error) {
	return getAPIEndpointsWithKeys(config, "https://process.", "process_config.process_dd_url", "process_config.additional_endpoints")
}

func GetEventsAPIEndpoints(config ddconfig.ConfigReader) (eps []apicfg.Endpoint, err error) {
	return getAPIEndpointsWithKeys(config, "https://process-events.", "process_config.events_dd_url", "process_config.events_additional_endpoints")
}

func getAPIEndpointsWithKeys(config ddconfig.ConfigReader, prefix, defaultEpKey, additionalEpsKey string) (eps []apicfg.Endpoint, err error) {
	// Setup main endpoint
	mainEndpointURL, err := url.Parse(utils.GetMainEndpoint(ddconfig.Datadog, prefix, defaultEpKey))
	if err != nil {
		return nil, fmt.Errorf("error parsing %s: %s", defaultEpKey, err)
	}
	eps = append(eps, apicfg.Endpoint{
		APIKey:   configUtils.SanitizeAPIKey(config.GetString("api_key")),
		Endpoint: mainEndpointURL,
	})

	// Optional additional pairs of endpoint_url => []apiKeys to submit to other locations.
	for endpointURL, apiKeys := range config.GetStringMapStringSlice(additionalEpsKey) {
		u, err := url.Parse(endpointURL)
		if err != nil {
			return nil, fmt.Errorf("invalid %s url '%s': %s", additionalEpsKey, endpointURL, err)
		}
		for _, k := range apiKeys {
			eps = append(eps, apicfg.Endpoint{
				APIKey:   configUtils.SanitizeAPIKey(k),
				Endpoint: u,
			})
		}
	}
	return
}
