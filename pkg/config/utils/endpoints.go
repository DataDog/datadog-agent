// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	infraURLPrefix = "https://app."
)

func getResolvedDDUrl(c config.ConfigReader, urlKey string) string {
	resolvedDDURL := c.GetString(urlKey)
	if c.IsSet("site") {
		log.Infof("'site' and '%s' are both set in config: setting main endpoint to '%s': \"%s\"", urlKey, urlKey, c.GetString(urlKey))
	}
	return resolvedDDURL
}

// mergeAdditionalEndpoints merges additional endpoints into keysPerDomain
func mergeAdditionalEndpoints(keysPerDomain, additionalEndpoints map[string][]string) (map[string][]string, error) {
	for domain, apiKeys := range additionalEndpoints {
		// Validating domain
		_, err := url.Parse(domain)
		if err != nil {
			return nil, fmt.Errorf("could not parse url from 'additional_endpoints' %s: %s", domain, err)
		}

		if _, ok := keysPerDomain[domain]; ok {
			for _, apiKey := range apiKeys {
				keysPerDomain[domain] = append(keysPerDomain[domain], apiKey)
			}
		} else {
			keysPerDomain[domain] = apiKeys
		}
	}

	// dedupe api keys and remove domains with no api keys (or empty ones)
	for domain, apiKeys := range keysPerDomain {
		dedupedAPIKeys := make([]string, 0, len(apiKeys))
		seen := make(map[string]bool)
		for _, apiKey := range apiKeys {
			trimmedAPIKey := strings.TrimSpace(apiKey)
			if _, ok := seen[trimmedAPIKey]; !ok && trimmedAPIKey != "" {
				seen[trimmedAPIKey] = true
				dedupedAPIKeys = append(dedupedAPIKeys, trimmedAPIKey)
			}
		}

		if len(dedupedAPIKeys) > 0 {
			keysPerDomain[domain] = dedupedAPIKeys
		} else {
			log.Infof("No API key provided for domain \"%s\", removing domain from endpoints", domain)
			delete(keysPerDomain, domain)
		}
	}

	return keysPerDomain, nil
}

// GetMainEndpointBackwardCompatible implements the logic to extract the DD URL from a config, based on `site`,ddURLKey and a backward compatible key
func GetMainEndpointBackwardCompatible(c config.ConfigReader, prefix string, ddURLKey string, backwardKey string) string {
	if c.IsSet(ddURLKey) && c.GetString(ddURLKey) != "" {
		// value under ddURLKey takes precedence over backwardKey and 'site'
		return getResolvedDDUrl(c, ddURLKey)
	} else if c.IsSet(backwardKey) && c.GetString(backwardKey) != "" {
		// value under backwardKey takes precedence over 'site'
		return getResolvedDDUrl(c, backwardKey)
	} else if c.GetString("site") != "" {
		return prefix + strings.TrimSpace(c.GetString("site"))
	}
	return prefix + config.DefaultSite
}

// GetMultipleEndpoints returns the api keys per domain specified in the main agent config
func GetMultipleEndpoints(c config.ConfigReader) (map[string][]string, error) {
	ddURL := GetInfraEndpoint(c)
	// Validating domain
	if _, err := url.Parse(ddURL); err != nil {
		return nil, fmt.Errorf("could not parse main endpoint: %s", err)
	}

	keysPerDomain := map[string][]string{
		ddURL: {
			c.GetString("api_key"),
		},
	}

	additionalEndpoints := c.GetStringMapStringSlice("additional_endpoints")
	return mergeAdditionalEndpoints(keysPerDomain, additionalEndpoints)
}

// GetMainEndpoint returns the main DD URL defined in the config, based on `site` and the prefix, or ddURLKey
func GetMainEndpoint(c config.ConfigReader, prefix string, ddURLKey string) string {
	// value under ddURLKey takes precedence over 'site'
	if c.IsSet(ddURLKey) && c.GetString(ddURLKey) != "" {
		return getResolvedDDUrl(c, ddURLKey)
	} else if c.GetString("site") != "" {
		return prefix + strings.TrimSpace(c.GetString("site"))
	}
	return prefix + config.DefaultSite
}

// GetInfraEndpoint returns the main DD Infra URL defined in config, based on the value of `site` and `dd_url`
func GetInfraEndpoint(c config.ConfigReader) string {
	return GetMainEndpoint(c, infraURLPrefix, "dd_url")
}
