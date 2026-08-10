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
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
)

// GetAPIEndpoints returns the list of api endpoints from the config
func GetAPIEndpoints(config pkgconfigmodel.Reader) (eps []apicfg.Endpoint, err error) {
	return getAPIEndpointsWithKeys(config, "https://process.", "process_config.process_dd_url", "process_config.additional_endpoints")
}

func getAPIEndpointsWithKeys(config pkgconfigmodel.Reader, prefix, defaultEpKey, additionalEpsKey string) (eps []apicfg.Endpoint, err error) {
	// Setup main endpoint
	mainEndpointURL, err := url.Parse(utils.GetMainEndpoint(config, prefix, defaultEpKey))
	if err != nil {
		return nil, fmt.Errorf("error parsing %s: %s", defaultEpKey, err)
	}
	eps = append(eps, apicfg.Endpoint{
		APIKey:            utils.SanitizeAPIKey(config.GetString("api_key")),
		Endpoint:          mainEndpointURL,
		ConfigSettingPath: "api_key",
	})

	// Optional additional pairs of endpoint_url => []apiKeys to submit to other locations.
	for endpointURL, apiKeys := range config.GetStringMapStringSlice(additionalEpsKey) {
		u, err := url.Parse(endpointURL)
		if err != nil {
			return nil, fmt.Errorf("invalid %s url '%s': %s", additionalEpsKey, endpointURL, err)
		}
		realKeys, hasPendingDelegatedAuth := utils.PartitionRealAndPendingKeys(apiKeys)
		for _, k := range realKeys {
			eps = append(eps, apicfg.Endpoint{
				APIKey:            utils.SanitizeAPIKey(k),
				Endpoint:          u,
				ConfigSettingPath: additionalEpsKey,
			})
		}
		if len(realKeys) == 0 && hasPendingDelegatedAuth {
			// Every key for this domain is still pending - keep a placeholder endpoint so the
			// domain still gets a resolver. resolver.OnUpdateConfig registers its config-update
			// listener per resolver (keyed by ConfigSettingPath), so a domain with no resolver at
			// all would never see the real key once delegated auth resolves it and writes it back
			// into this same config slot.
			eps = append(eps, apicfg.Endpoint{
				APIKey:            "",
				Endpoint:          u,
				ConfigSettingPath: additionalEpsKey,
			})
		}
	}
	return
}
