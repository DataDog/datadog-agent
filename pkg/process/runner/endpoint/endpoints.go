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
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
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
	mainAPIKey := utils.SanitizeAPIKey(config.GetString("api_key"))
	if utils.IsDelaDirective(config.GetString("api_key")) {
		log.Warnf("api_key holds a delegated-auth directive; only settings enrolled for delegated auth " +
			"(such as %s) are resolved, the directive will be sent as-is to %s", additionalEpsKey, mainEndpointURL)
	}
	eps = append(eps, apicfg.Endpoint{
		APIKey:            mainAPIKey,
		Endpoint:          mainEndpointURL,
		ConfigSettingPath: "api_key",
	})

	// Optional additional pairs of endpoint_url => []apiKeys to submit to other locations.
	// DELA[...] directives are held aside as pending: the domain stays alive (with no key)
	// until the delegatedauth component writes the resolved key back into the config.
	for endpointURL, apiKeys := range config.GetStringMapStringSlice(additionalEpsKey) {
		u, err := url.Parse(endpointURL)
		if err != nil {
			return nil, fmt.Errorf("invalid %s url '%s': %s", additionalEpsKey, endpointURL, err)
		}
		realKeys, hasPending := utils.PartitionRealAndPendingKeys(apiKeys)
		for _, k := range realKeys {
			eps = append(eps, apicfg.Endpoint{
				APIKey:            utils.SanitizeAPIKey(k),
				Endpoint:          u,
				ConfigSettingPath: additionalEpsKey,
			})
		}
		if hasPending {
			eps = append(eps, apicfg.Endpoint{
				Endpoint:                u,
				ConfigSettingPath:       additionalEpsKey,
				HasPendingDelegatedAuth: true,
			})
		}
	}
	return
}

// CheckAPIKeysResolved rejects endpoints whose API key is still an unresolved secret handle
// (e.g. 'ENC[...]'). Callers that do not resolve secret-backend handles themselves (system-probe)
// would otherwise send the handle verbatim as the API key.
func CheckAPIKeysResolved(eps []apicfg.Endpoint) error {
	for _, ep := range eps {
		if scrubber.IsEnc(ep.APIKey) {
			return fmt.Errorf("%s is an unresolved secret handle (%q): this process can not resolve secret handles "+
				"and expects to receive them from the core agent already resolved", ep.ConfigSettingPath, ep.APIKey)
		}
		if ep.HasPendingDelegatedAuth {
			return fmt.Errorf("%s has an unresolved delegated-auth directive: this process can not resolve "+
				"delegated-auth directives (only the process running the delegatedauth component can), "+
				"disable direct send from this process or remove the directive", ep.ConfigSettingPath)
		}
	}
	return nil
}
