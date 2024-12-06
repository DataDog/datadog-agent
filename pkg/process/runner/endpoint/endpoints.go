// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package endpoint

import (
	"fmt"
	"net/url"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
)

// GetAPIEndpoints returns the list of api endpoints from the config
func GetAPIEndpoints(config pkgconfigmodel.Reader) (eps []apicfg.Endpoint, err error) {
	return getAPIEndpointsWithKeys(config, "https://process.", "process_config.process_dd_url", "process_config.additional_endpoints")
}

// GetEventsAPIEndpoints returns the list of api event endpoints from the config
func GetEventsAPIEndpoints(config pkgconfigmodel.Reader) (eps []apicfg.Endpoint, err error) {
	return getAPIEndpointsWithKeys(config, "https://process-events.", "process_config.events_dd_url", "process_config.events_additional_endpoints")
}

func getAPIEndpointsWithKeys(config pkgconfigmodel.Reader, prefix, defaultEpKey, additionalEpsKey string) (eps []apicfg.Endpoint, err error) {
	// Setup main endpoint
	mainEndpointURL, err := url.Parse(utils.GetMainEndpoint(pkgconfigsetup.Datadog(), prefix, defaultEpKey))
	if err != nil {
		return nil, fmt.Errorf("error parsing %s: %s", defaultEpKey, err)
	}
	eps = append(eps, apicfg.Endpoint{
		APIKey:   utils.SanitizeAPIKey(config.GetString("api_key")),
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
				APIKey:   utils.SanitizeAPIKey(k),
				Endpoint: u,
			})
		}
	}
	return
}
