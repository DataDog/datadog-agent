// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package endpoint

import (
	"errors"
	"fmt"
	"net/url"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// ErrNoConnectionsEndpoint is returned when process_config.connections_send_to_main_endpoint
// is false but no usable process_config.additional_endpoints entry is configured, which would
// leave network connections payloads with nowhere to go. The configuration is rejected instead
// of silently dropping NPM and USM data.
var ErrNoConnectionsEndpoint = errors.New("process_config.connections_send_to_main_endpoint is false but no usable process_config.additional_endpoints entry is configured: refusing to run with no destination for network connections")

// GetAPIEndpoints returns the list of api endpoints from the config
func GetAPIEndpoints(config pkgconfigmodel.Reader) (eps []apicfg.Endpoint, err error) {
	return getAPIEndpointsWithKeys(config, "https://process.", "process_config.process_dd_url", "process_config.additional_endpoints", true)
}

// GetConnectionsAPIEndpoints returns the api endpoints for network connections payloads. It is
// GetAPIEndpoints minus the main endpoint when process_config.connections_send_to_main_endpoint
// is false, so the local copy can be dropped while additional_endpoints keep receiving data.
func GetConnectionsAPIEndpoints(config pkgconfigmodel.Reader) (eps []apicfg.Endpoint, err error) {
	includeMain := config.GetBool("process_config.connections_send_to_main_endpoint")
	eps, err = getAPIEndpointsWithKeys(config, "https://process.", "process_config.process_dd_url", "process_config.additional_endpoints", includeMain)
	if err != nil {
		return nil, err
	}
	if !includeMain && !hasUsableEndpoint(eps) {
		return nil, ErrNoConnectionsEndpoint
	}
	return eps, nil
}

// hasUsableEndpoint reports whether at least one endpoint could actually receive a payload. An
// endpoint missing a host or an API key is not dropped here, it is sent and rejected by the intake.
func hasUsableEndpoint(eps []apicfg.Endpoint) bool {
	for _, e := range eps {
		if e.Endpoint != nil && e.Endpoint.Host != "" && e.APIKey != "" {
			return true
		}
	}
	return false
}

func getAPIEndpointsWithKeys(config pkgconfigmodel.Reader, prefix, defaultEpKey, additionalEpsKey string, includeMain bool) (eps []apicfg.Endpoint, err error) {
	if includeMain {
		mainEndpointURL, err := url.Parse(utils.GetMainEndpoint(config, prefix, defaultEpKey))
		if err != nil {
			return nil, fmt.Errorf("error parsing %s: %s", defaultEpKey, err)
		}
		eps = append(eps, apicfg.Endpoint{
			APIKey:            utils.SanitizeAPIKey(config.GetString("api_key")),
			Endpoint:          mainEndpointURL,
			ConfigSettingPath: "api_key",
		})
	}

	// Optional additional pairs of endpoint_url => []apiKeys to submit to other locations.
	for endpointURL, apiKeys := range config.GetStringMapStringSlice(additionalEpsKey) {
		u, err := url.Parse(endpointURL)
		if err != nil {
			return nil, fmt.Errorf("invalid %s url '%s': %s", additionalEpsKey, endpointURL, err)
		}
		for _, k := range apiKeys {
			eps = append(eps, apicfg.Endpoint{
				APIKey:            utils.SanitizeAPIKey(k),
				Endpoint:          u,
				ConfigSettingPath: additionalEpsKey,
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
	}
	return nil
}
